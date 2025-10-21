package gemini

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
)

// // calculateMultimodalTokens 根据 Gemini API 官方规则计算多模态内容的 token 数量
// func calculateMultimodalTokens(inlineData *GeminiInlineData) int {
// 	if inlineData == nil {
// 		return 0
// 	}

// 	mimeType := strings.ToLower(inlineData.MimeType)

// 	// 图片处理
// 	if strings.HasPrefix(mimeType, "image/") {
// 		// 如果没有尺寸信息，使用默认值 258 tokens
// 		if inlineData.Width == nil || inlineData.Height == nil {
// 			return 258
// 		}

// 		width, height := *inlineData.Width, *inlineData.Height

// 		// 如果两个维度都小于等于 384 像素，计为 258 个 token
// 		if width <= 384 && height <= 384 {
// 			return 258
// 		}

// 		// 如果图片较大，按 768x768 图块计算
// 		// 计算需要的图块数量
// 		tilesX := int(math.Ceil(float64(width) / 768.0))
// 		tilesY := int(math.Ceil(float64(height) / 768.0))
// 		totalTiles := tilesX * tilesY

// 		return totalTiles * 258
// 	}

// 	// 视频处理：每秒 263 个 token
// 	if strings.HasPrefix(mimeType, "video/") {
// 		if inlineData.Duration == nil {
// 			// 如果没有时长信息，使用默认值
// 			return 263
// 		}
// 		return int(math.Ceil(*inlineData.Duration * 263))
// 	}

// 	// 音频处理：每秒 32 个 token
// 	if strings.HasPrefix(mimeType, "audio/") {
// 		if inlineData.Duration == nil {
// 			// 如果没有时长信息，使用默认值
// 			return 32
// 		}
// 		return int(math.Ceil(*inlineData.Duration * 32))
// 	}

// 	// 其他类型，使用默认值
// 	return 258
// }

func GeminiTextGenerationHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	// 解析为 Gemini 原生响应格式
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 计算使用量（基于 UsageMetadata）
	usage := dto.Usage{
		PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
		CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount + geminiResponse.UsageMetadata.ThoughtsTokenCount,
		TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
	}

	usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount

	for _, detail := range geminiResponse.UsageMetadata.PromptTokensDetails {
		if detail.Modality == "AUDIO" {
			usage.PromptTokensDetails.AudioTokens = detail.TokenCount
		} else if detail.Modality == "TEXT" {
			usage.PromptTokensDetails.TextTokens = detail.TokenCount
		}
	}

	// // 统计响应中的多模态内容（仅用于调试信息）
	// var imageCount, videoCount, audioCount int
	// for _, candidate := range geminiResponse.Candidates {
	// 	for _, part := range candidate.Content.Parts {
	// 		if part.InlineData != nil && part.InlineData.MimeType != "" {
	// 			mimeType := strings.ToLower(part.InlineData.MimeType)
	// 			if strings.HasPrefix(mimeType, "video/") {
	// 				videoCount++
	// 			} else if strings.HasPrefix(mimeType, "audio/") {
	// 				audioCount++
	// 			} else if strings.HasPrefix(mimeType, "image/") {
	// 				imageCount++
	// 			}
	// 		}
	// 	}
	// }

	// if common.DebugEnabled && (imageCount > 0 || videoCount > 0 || audioCount > 0) {
	// 	println("Generated content contains:", imageCount, "images,", videoCount, "videos,", audioCount, "audio files")
	// 	println("Official token count - Prompt:", usage.PromptTokens, "Completion:", usage.CompletionTokens, "Total:", usage.TotalTokens)
	// }

	// 直接返回 Gemini 原生格式的 JSON 响应
	jsonResponse, err := common.Marshal(geminiResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return &usage, nil
}

func NativeGeminiEmbeddingHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	usage := &dto.Usage{
		PromptTokens: info.PromptTokens,
		TotalTokens:  info.PromptTokens,
	}

	if info.IsGeminiBatchEmbedding {
		var geminiResponse dto.GeminiBatchEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	} else {
		var geminiResponse dto.GeminiEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return usage, nil
}

func GeminiTextGenerationStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var usage = &dto.Usage{}
	var imageCount int
	// var totalMultimodalTokens int
	// var imageCount, videoCount, audioCount int

	helper.SetEventStreamHeaders(c)

	responseText := strings.Builder{}

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		var geminiResponse dto.GeminiChatResponse
		err := common.UnmarshalJsonStr(data, &geminiResponse)
		if err != nil {
			logger.LogError(c, "error unmarshalling stream response: "+err.Error())
			return false
		}

		// 统计多模态内容
		for _, candidate := range geminiResponse.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.InlineData != nil && part.InlineData.MimeType != "" {
					imageCount++
					// tokens := calculateMultimodalTokens(part.InlineData)
					// totalMultimodalTokens += tokens

					// mimeType := strings.ToLower(part.InlineData.MimeType)
					// if strings.HasPrefix(mimeType, "video/") {
					// 	videoCount++
					// } else if strings.HasPrefix(mimeType, "audio/") {
					// 	audioCount++
					// } else if strings.HasPrefix(mimeType, "image/") {
					// 	imageCount++
					// }
				}
				if part.Text != "" {
					responseText.WriteString(part.Text)
				}
			}
		}

		// 更新使用量统计
		if geminiResponse.UsageMetadata.TotalTokenCount != 0 {
			usage.PromptTokens = geminiResponse.UsageMetadata.PromptTokenCount
			usage.CompletionTokens = geminiResponse.UsageMetadata.CandidatesTokenCount + geminiResponse.UsageMetadata.ThoughtsTokenCount
			usage.TotalTokens = geminiResponse.UsageMetadata.TotalTokenCount
			usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount
			for _, detail := range geminiResponse.UsageMetadata.PromptTokensDetails {
				if detail.Modality == "AUDIO" {
					usage.PromptTokensDetails.AudioTokens = detail.TokenCount
				} else if detail.Modality == "TEXT" {
					usage.PromptTokensDetails.TextTokens = detail.TokenCount
				}
			}
		}

		// 直接发送 GeminiChatResponse 响应
		err = helper.StringData(c, data)
		if err != nil {
			logger.LogError(c, err.Error())
		}
		info.SendResponseCount++
		return true
	})

	if info.SendResponseCount == 0 {
		return nil, types.NewOpenAIError(errors.New("no response received from Gemini API"), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
	}

	if imageCount != 0 {
		if usage.CompletionTokens == 0 {
			usage.CompletionTokens = imageCount * 258
			// // 只有在官方 token 数据不可用时才使用自定义计算
			// if usage.CompletionTokens == 0 && totalMultimodalTokens > 0 {
			// 	usage.CompletionTokens = totalMultimodalTokens
			// 	if common.DebugEnabled {
			// 		println("Using custom token calculation:", totalMultimodalTokens, "tokens")
			// 	}
			// }

			// if common.DebugEnabled && (imageCount > 0 || videoCount > 0 || audioCount > 0) {
			// 	println("Stream response contains:", imageCount, "images,", videoCount, "videos,", audioCount, "audio files")
			// 	if usage.CompletionTokens > 0 {
			// 		println("Official token count - Completion:", usage.CompletionTokens)
			// 	} else {
			// 		println("Custom calculated tokens:", totalMultimodalTokens)
		}
	}

	// 如果usage.CompletionTokens为0，则使用本地统计的completion tokens
	if usage.CompletionTokens == 0 {
		str := responseText.String()
		if len(str) > 0 {
			usage = service.ResponseText2Usage(responseText.String(), info.UpstreamModelName, info.PromptTokens)
		} else {
			// 空补全，不需要使用量
			usage = &dto.Usage{}
		}
	}

	// 移除流式响应结尾的[Done]，因为Gemini API没有发送Done的行为
	//helper.Done(c)

	return usage, nil
}
