package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	// 注意：hash 调度检查在 getChannel 函数中直接检查 X-Request-Id header

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeQuota(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil && relayInfo.FinalPreConsumedQuota != 0 {
			service.ReturnPreConsumedQuota(c, relayInfo)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}

		addUsedChannel(c, channel.Id)
		requestBody, bodyErr := common.GetRequestBody(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			return
		}

		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		// 记录状态码复写后的值，用于调试
		logger.LogInfo(c, fmt.Sprintf("[重试检查] 错误状态码: %d, 是否跳过重试: %v", newAPIError.StatusCode, types.IsSkipRetryError(newAPIError)))

		// 将当前失败的渠道ID存储到context中，用于shouldRetry检查全重试开关
		c.Set("failed_channel_id", channel.Id)

		if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
			break
		}

		// 重试时，如果 channel_id 存在但不是通过 URL 参数指定的（即 specific_channel_id 不存在），
		// 清除 channel_id 让重试时可以选择新渠道（包括兜底渠道）
		// 这样可以避免重试时一直使用失败的渠道
		if _, hasSpecificChannelId := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); !hasSpecificChannelId {
			// 清除 channel_id，让重试时重新选择渠道
			c.Set("channel_id", 0)
			common.SetContextKey(c, constant.ContextKeyChannelId, 0)
			logger.LogInfo(c, "[重试] 清除 channel_id 以允许重新选择渠道")
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		if r.MaxCompletionTokens > r.MaxTokens {
			meta.MaxTokens = int(r.MaxCompletionTokens)
		} else {
			meta.MaxTokens = int(r.MaxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(r.MaxOutputTokens)
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(r.MaxTokens)
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	// 检查是否已经通过 context 指定了渠道（比如通过 URL 参数或中间件）
	// 如果 channel_id 存在，说明已经指定了渠道，直接返回
	if channelId := c.GetInt("channel_id"); channelId > 0 {
		logger.LogInfo(c, fmt.Sprintf("[getChannel] 已指定渠道 #%d，跳过渠道选择", channelId))
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      channelId,
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}

	// 重试时检查兜底渠道
	if retryParam.GetRetry() > 0 {
		// 获取之前使用的渠道列表，最后一个就是失败的渠道
		useChannel := c.GetStringSlice("use_channel")
		if len(useChannel) > 0 {
			// 获取最后一个渠道ID（失败的渠道）
			lastChannelIdStr := useChannel[len(useChannel)-1]
			lastChannelId, err := strconv.Atoi(lastChannelIdStr)
			if err == nil && lastChannelId > 0 {
				// 查询失败渠道的兜底渠道
				failedChannel, err := model.GetChannelById(lastChannelId, false)
				if err == nil && failedChannel != nil {
					// 检查是否有兜底渠道
					if failedChannel.FallbackChannelId != nil && *failedChannel.FallbackChannelId > 0 {
						fallbackChannelId := *failedChannel.FallbackChannelId

						// 检查兜底渠道是否已经在失败列表中，避免无限循环
						fallbackAlreadyUsed := false
						for _, channelIdStr := range useChannel {
							if channelId, err := strconv.Atoi(channelIdStr); err == nil && channelId == fallbackChannelId {
								fallbackAlreadyUsed = true
								logger.LogWarn(c, fmt.Sprintf("[getChannel] 兜底渠道 #%d 已在失败列表中，跳过兜底渠道，继续正常调度", fallbackChannelId))
								break
							}
						}

						if !fallbackAlreadyUsed {
							logger.LogInfo(c, fmt.Sprintf("[getChannel] 重试流量，检测到渠道 #%d 的兜底渠道 #%d，直接使用兜底渠道", lastChannelId, fallbackChannelId))

							// 获取兜底渠道的完整信息（包括 key 字段）
							fallbackChannel, err := model.GetChannelById(fallbackChannelId, true)
							if err == nil && fallbackChannel != nil {
								// 验证兜底渠道是否可用（状态检查）
								if fallbackChannel.Status == common.ChannelStatusEnabled {
									// 验证兜底渠道是否支持该模型
									fallbackModels := fallbackChannel.GetModels()
									modelSupported := false
									for _, m := range fallbackModels {
										if m == info.OriginModelName {
											modelSupported = true
											break
										}
									}
									if modelSupported {
										logger.LogInfo(c, fmt.Sprintf("[getChannel] 使用兜底渠道 #%d (%s) 进行重试", fallbackChannelId, fallbackChannel.Name))
										// 设置兜底渠道到 context
										newAPIError := middleware.SetupContextForSelectedChannel(c, fallbackChannel, info.OriginModelName)
										if newAPIError != nil {
											return nil, newAPIError
										}
										// 验证配置是否正确设置
										channelKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
										channelBaseUrl := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
										logger.LogInfo(c, fmt.Sprintf("[getChannel] 兜底渠道 #%d 配置已设置: BaseURL=%s, Key=%s", fallbackChannelId, channelBaseUrl, common.MaskSensitiveInfo(channelKey)))
										return fallbackChannel, nil
									} else {
										logger.LogWarn(c, fmt.Sprintf("[getChannel] 兜底渠道 #%d 不支持模型 %s，继续正常调度", fallbackChannelId, info.OriginModelName))
									}
								} else {
									logger.LogWarn(c, fmt.Sprintf("[getChannel] 兜底渠道 #%d 未启用，继续正常调度", fallbackChannelId))
								}
							} else {
								logger.LogWarn(c, fmt.Sprintf("[getChannel] 兜底渠道 #%d 不存在，继续正常调度", fallbackChannelId))
							}
						}
					}
				}
			}
		}
	}

	// 需要选择渠道：检查是否携带 X-Request-Id header，如果携带则使用 hash 调度
	var channel *model.Channel
	var selectGroup string
	var err error

	// 获取已使用的渠道列表，用于排除已失败的渠道
	useChannel := c.GetStringSlice("use_channel")
	usedChannelIds := make(map[int]bool)
	for _, channelIdStr := range useChannel {
		if channelId, err := strconv.Atoi(channelIdStr); err == nil && channelId > 0 {
			usedChannelIds[channelId] = true
		}
	}

	customRequestId := c.GetHeader("X-Request-Id")
	if customRequestId == "" {
		// 也支持小写的 request_id header
		customRequestId = c.GetHeader("request_id")
	}

	// 输出调试日志，检查 header 是否存在
	logger.LogInfo(c, fmt.Sprintf("[渠道选择] 检查 X-Request-Id header，值: '%s' (空字符串表示未携带)", customRequestId))

	// 直接选择渠道，不进行重试循环
	if customRequestId != "" {
		// 使用 hash 调度
		logger.LogInfo(c, fmt.Sprintf("[Hash调度] 检测到 X-Request-Id header，使用 hash 调度: %s", customRequestId))
		channel, selectGroup, err = service.CacheGetHashSatisfiedChannel(retryParam, customRequestId)
		if err != nil {
			logger.LogWarn(c, fmt.Sprintf("[Hash调度] hash 调度失败，回退到随机调度: %s", err.Error()))
			// 如果 hash 调度失败，回退到随机调度
			channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(retryParam)
		} else if channel != nil && usedChannelIds[channel.Id] {
			// 如果 hash 调度选中的渠道已在失败列表中，回退到随机调度
			logger.LogWarn(c, fmt.Sprintf("[Hash调度] hash 调度选中的渠道 #%d 已在失败列表中，回退到随机调度", channel.Id))
			channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(retryParam)
		} else if channel != nil {
			logger.LogInfo(c, fmt.Sprintf("[Hash调度] 使用 hash 调度选择渠道 #%d (分组: %s)", channel.Id, selectGroup))
		}
	} else {
		// 使用原来的随机调度
		logger.LogInfo(c, "[渠道选择] 未检测到 X-Request-Id header，使用随机调度")
		channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(retryParam)
	}

	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	// 检查选中的渠道是否在已使用的渠道列表中（随机调度也可能选中已失败的渠道）
	if usedChannelIds[channel.Id] {
		// 如果选中的渠道已经在失败列表中，增加 retry 参数，让外层重试逻辑再次调用 getChannel
		logger.LogWarn(c, fmt.Sprintf("[渠道选择] 渠道 #%d 已在失败列表中，增加 retry 参数，由外层重试逻辑处理", channel.Id))
		retryParam.IncreaseRetry()
		return nil, types.NewError(fmt.Errorf("选中的渠道 #%d 已在失败列表中，需要重试", channel.Id), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}

	// 检查渠道的全重试开关
	// 优先从 context 中获取当前失败的渠道ID（更准确）
	failedChannelId := c.GetInt("failed_channel_id")
	if failedChannelId <= 0 {
		// 如果没有，则从 use_channel 中获取最后一个 channel_id（失败的渠道）
		useChannel := c.GetStringSlice("use_channel")
		if len(useChannel) > 0 {
			lastChannelIdStr := useChannel[len(useChannel)-1]
			lastChannelId, err := strconv.Atoi(lastChannelIdStr)
			if err == nil && lastChannelId > 0 {
				failedChannelId = lastChannelId
			}
		}
	}
	if failedChannelId > 0 {
		channel, err := model.CacheGetChannel(failedChannelId)
		if err == nil && channel != nil {
			setting := channel.GetSetting()
			if setting.AllowAllRetry {
				logger.LogInfo(c, fmt.Sprintf("[全重试] 渠道 #%d 开启了全重试开关，允许重试", failedChannelId))
				return true
			}
		}
	}
	if openaiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if openaiErr.StatusCode == 307 {
		return true
	}
	if openaiErr.StatusCode/100 == 5 {
		// 超时不重试
		if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if openaiErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if openaiErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if openaiErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(channelError.ChannelType, err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.Error())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		other["admin_info"] = adminInfo
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveError(), tokenId, 0, false, userGroup, other)
	}

}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTask(c *gin.Context) {
	retryTimes := common.RetryTimes
	channelId := c.GetInt("channel_id")
	c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return
	}
	taskErr := taskRelayHandler(c, relayInfo)
	if taskErr == nil {
		retryTimes = 0
	}
	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}
	for ; shouldRetryTaskRelay(c, channelId, taskErr, retryTimes) && retryParam.GetRetry() < retryTimes; retryParam.IncreaseRetry() {
		channel, newAPIError := getChannel(c, relayInfo, retryParam)
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
			taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
			break
		}
		channelId = channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		logger.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, retryParam.GetRetry()))
		//middleware.SetupContextForSelectedChannel(c, channel, originalModel)

		requestBody, err := common.GetRequestBody(c)
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		taskErr = taskRelayHandler(c, relayInfo)
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if taskErr != nil {
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.TaskError {
	var err *dto.TaskError
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeVideoFetchByID:
		err = relay.RelayTaskFetch(c, relayInfo.RelayMode)
	default:
		err = relay.RelayTaskSubmit(c, relayInfo)
	}
	return err
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}

	// 检查渠道的全重试开关
	if channelId > 0 {
		channel, err := model.CacheGetChannel(channelId)
		if err == nil && channel != nil {
			setting := channel.GetSetting()
			if setting.AllowAllRetry {
				logger.LogInfo(c, fmt.Sprintf("[全重试] 渠道 #%d 开启了全重试开关，允许重试", channelId))
				return true
			}
		}
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
