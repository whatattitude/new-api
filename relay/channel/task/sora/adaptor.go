package sora

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type responseTask struct {
	ID                 string `json:"id"`
	TaskID             string `json:"task_id,omitempty"` //兼容旧接口
	Object             string `json:"object"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	CreatedAt          int64  `json:"created_at"`
	CompletedAt        int64  `json:"completed_at,omitempty"`
	ExpiresAt          int64  `json:"expires_at,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	return nil
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	// 检查是否从 JSON 转换为 multipart/form-data
	if multipartContentType, exists := c.Get("multipart_content_type"); exists {
		if contentType, ok := multipartContentType.(string); ok {
			req.Header.Set("Content-Type", contentType)
			return nil
		}
	}

	// 如果已经是 multipart/form-data，直接使用原始 Content-Type
	contentType := c.Request.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		req.Header.Set("Content-Type", contentType)
	} else {
		// 如果既不是 multipart 也没有转换后的 Content-Type，使用原始 Content-Type
		// 这种情况不应该发生，但为了安全起见还是设置一下
		req.Header.Set("Content-Type", contentType)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	contentType := c.Request.Header.Get("Content-Type")

	// 如果已经是 multipart/form-data，直接使用原始请求体
	if strings.HasPrefix(contentType, "multipart/form-data") {
		cachedBody, err := common.GetRequestBody(c)
		if err != nil {
			return nil, errors.Wrap(err, "get_request_body_failed")
		}
		return bytes.NewReader(cachedBody), nil
	}

	// 如果是 JSON 格式，需要转换为 multipart/form-data
	// 从原始 JSON 请求体中解析所有字段（包括 aspect_ratio 等）
	cachedBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}

	var jsonReq map[string]interface{}
	if err := common.Unmarshal(cachedBody, &jsonReq); err != nil {
		return nil, errors.Wrap(err, "unmarshal_json_request_failed")
	}

	// 创建 multipart/form-data 请求体
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// 写入所有字段（包括 model, prompt, seconds, size, mode, aspect_ratio 等）
	// 排除需要特殊处理的字段
	skipFields := map[string]bool{
		"input_reference": true, // 文件字段，需要特殊处理
	}

	for key, value := range jsonReq {
		if skipFields[key] {
			// input_reference 如果是文件，需要特殊处理（见下面的文件处理逻辑）
			continue
		}
		// 写入所有其他字段
		if strValue, ok := value.(string); ok && strValue != "" {
			writer.WriteField(key, strValue)
		} else if numValue, ok := value.(float64); ok {
			writer.WriteField(key, fmt.Sprintf("%.0f", numValue))
		} else if boolValue, ok := value.(bool); ok {
			writer.WriteField(key, fmt.Sprintf("%v", boolValue))
		} else if value != nil {
			writer.WriteField(key, fmt.Sprintf("%v", value))
		}
	}

	// 处理 input_reference（图片文件）
	// 检查是否有实际上传的文件
	if mf, err := c.MultipartForm(); err == nil {
		if files, exists := mf.File["input_reference"]; exists && len(files) > 0 {
			// 使用实际上传的文件
			for _, fileHeader := range files {
				file, err := fileHeader.Open()
				if err != nil {
					continue
				}
				part, err := writer.CreateFormFile("input_reference", fileHeader.Filename)
				if err != nil {
					file.Close()
					continue
				}
				io.Copy(part, file)
				file.Close()
			}
		}
	}
	// 注意：如果 input_reference 在 JSON 中是字符串（如 base64 或 URL），
	// OpenAI Sora API 通常期望文件上传，所以这里不处理字符串形式的 input_reference

	// 关闭 writer 以完成 multipart 数据
	if err := writer.Close(); err != nil {
		return nil, errors.Wrap(err, "close_multipart_writer_failed")
	}

	// 设置正确的 Content-Type header（需要在调用方设置）
	// 这里返回一个包含 Content-Type 的 reader wrapper
	multipartBody := &multipartBodyReader{
		reader:      bytes.NewReader(requestBody.Bytes()),
		contentType: writer.FormDataContentType(),
	}

	// 将 Content-Type 存储到 context 中，以便在 BuildRequestHeader 之后设置
	c.Set("multipart_content_type", writer.FormDataContentType())

	return multipartBody, nil
}

// multipartBodyReader 是一个包装器，用于在读取时设置 Content-Type
type multipartBodyReader struct {
	reader      io.Reader
	contentType string
}

func (m *multipartBodyReader) Read(p []byte) (n int, err error) {
	return m.reader.Read(p)
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		if dResp.TaskID == "" {
			taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
			return
		}
		dResp.ID = dResp.TaskID
		dResp.TaskID = ""
	}

	c.JSON(http.StatusOK, dResp)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Url = fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, resTask.ID)
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	return task.Data, nil
}
