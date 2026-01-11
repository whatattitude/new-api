package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func validUserInfo(username string, role int) bool {
	// check username is empty
	if strings.TrimSpace(username) == "" {
		return false
	}
	if !common.IsValidateRole(role) {
		return false
	}
	return true
}

func authHelper(c *gin.Context, minRole int) {
	session := sessions.Default(c)
	username := session.Get("username")
	role := session.Get("role")
	id := session.Get("id")
	status := session.Get("status")
	useAccessToken := false
	if username == nil {
		// Check access token or sk-xxx token
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无权进行此操作，未登录且未提供 access token",
			})
			c.Abort()
			return
		}
		
		// 检查是否是 sk-xxx 格式的 token
		authHeader = strings.TrimPrefix(authHeader, "Bearer ")
		if strings.HasPrefix(authHeader, "sk-") {
			// 这是 sk-xxx 格式的 token，应该查询 tokens 表
			key := strings.TrimPrefix(authHeader, "sk-")
			parts := strings.Split(key, "-")
			key = parts[0]
			
			token, err := model.ValidateUserToken(key)
			if err != nil || token == nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "无权进行此操作，token 无效: " + err.Error(),
				})
				c.Abort()
				return
			}
			
			// 获取完整的用户信息（包含 Role）
			user, err := model.GetUserById(token.UserId, false)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "无权进行此操作，获取用户信息失败: " + err.Error(),
				})
				c.Abort()
				return
			}
			
			if user.Status != common.UserStatusEnabled {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "用户已被封禁",
				})
				c.Abort()
				return
			}
			
			if !validUserInfo(user.Username, user.Role) {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "无权进行此操作，用户信息无效",
				})
				c.Abort()
				return
			}
			
			// Token is valid, set user info
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
			useAccessToken = true
			// 使用 token 鉴权时，不需要 New-Api-User header，直接跳过检查
		} else {
			// 这是 access token 格式，查询 users 表
			user := model.ValidateAccessToken(authHeader)
			if user != nil && user.Username != "" {
				if !validUserInfo(user.Username, user.Role) {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": "无权进行此操作，用户信息无效",
					})
					c.Abort()
					return
				}
			// Token is valid
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
			useAccessToken = true
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，access token 无效",
			})
			c.Abort()
			return
		}
	}
	}
	
	// 只有在使用 session 鉴权时，才需要检查 New-Api-User header
	// 使用 token 鉴权（sk-xxx 或 access token）时，已经通过 token 获取了用户ID，不需要 New-Api-User
	if !useAccessToken {
	// get header New-Api-User
	apiUserIdStr := c.Request.Header.Get("New-Api-User")
	if apiUserIdStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，未提供 New-Api-User",
		})
		c.Abort()
		return
	}
	apiUserId, err := strconv.Atoi(apiUserIdStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，New-Api-User 格式错误",
		})
		c.Abort()
		return

	}
	if id != apiUserId {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，New-Api-User 与登录用户不匹配",
		})
		c.Abort()
		return
		}
	}
	if status.(int) == common.UserStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已被封禁",
		})
		c.Abort()
		return
	}
	if role.(int) < minRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，权限不足",
		})
		c.Abort()
		return
	}
	if !validUserInfo(username.(string), role.(int)) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，用户信息无效",
		})
		c.Abort()
		return
	}
	c.Set("username", username)
	c.Set("role", role)
	c.Set("id", id)
	c.Set("group", session.Get("group"))
	c.Set("user_group", session.Get("group"))
	c.Set("use_access_token", useAccessToken)

	//userCache, err := model.GetUserCache(id.(int))
	//if err != nil {
	//	c.JSON(http.StatusOK, gin.H{
	//		"success": false,
	//		"message": err.Error(),
	//	})
	//	c.Abort()
	//	return
	//}
	//userCache.WriteContext(c)

	c.Next()
}

func TryUserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id := session.Get("id")
		if id != nil {
			c.Set("id", id)
		}
		c.Next()
	}
}

func UserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleCommonUser)
	}
}

func AdminAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleAdminUser)
	}
}

func RootAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleRootUser)
	}
}

func WssAuth(c *gin.Context) {

}

func TokenAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 先检测是否为ws
		if c.Request.Header.Get("Sec-WebSocket-Protocol") != "" {
			// Sec-WebSocket-Protocol: realtime, openai-insecure-api-key.sk-xxx, openai-beta.realtime-v1
			// read sk from Sec-WebSocket-Protocol
			key := c.Request.Header.Get("Sec-WebSocket-Protocol")
			parts := strings.Split(key, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "openai-insecure-api-key") {
					key = strings.TrimPrefix(part, "openai-insecure-api-key.")
					break
				}
			}
			c.Request.Header.Set("Authorization", "Bearer "+key)
		}
		// 检查path包含/v1/messages
		if strings.Contains(c.Request.URL.Path, "/v1/messages") {
			anthropicKey := c.Request.Header.Get("x-api-key")
			if anthropicKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+anthropicKey)
			}
		}
		// gemini api 从query中获取key
		if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1beta/openai/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
			skKey := c.Query("key")
			if skKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+skKey)
			}
			// 从x-goog-api-key header中获取key
			xGoogKey := c.Request.Header.Get("x-goog-api-key")
			if xGoogKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+xGoogKey)
			}
		}
		key := c.Request.Header.Get("Authorization")
		parts := make([]string, 0)
		key = strings.TrimPrefix(key, "Bearer ")
		if key == "" || key == "midjourney-proxy" {
			key = c.Request.Header.Get("mj-api-secret")
			key = strings.TrimPrefix(key, "Bearer ")
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		} else {
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		}
		token, err := model.ValidateUserToken(key)
		if token != nil {
			id := c.GetInt("id")
			if id == 0 {
				c.Set("id", token.UserId)
			}
		}
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, err.Error())
			return
		}

		allowIps := token.GetIpLimits()
		if len(allowIps) > 0 {
			clientIp := c.ClientIP()
			logger.LogDebug(c, "Token has IP restrictions, checking client IP %s", clientIp)
			ip := net.ParseIP(clientIp)
			if ip == nil {
				abortWithOpenAiMessage(c, http.StatusForbidden, "无法解析客户端 IP 地址")
				return
			}
			if common.IsIpInCIDRList(ip, allowIps) == false {
				abortWithOpenAiMessage(c, http.StatusForbidden, "您的 IP 不在令牌允许访问的列表中")
				return
			}
			logger.LogDebug(c, "Client IP %s passed the token IP restrictions check", clientIp)
		}

		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, err.Error())
			return
		}
		userEnabled := userCache.Status == common.UserStatusEnabled
		if !userEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, "用户已被封禁")
			return
		}

		userCache.WriteContext(c)

		userGroup := userCache.Group
		tokenGroup := token.Group
		if tokenGroup != "" {
			// check common.UserUsableGroups[userGroup]
			if _, ok := service.GetUserUsableGroups(userGroup)[tokenGroup]; !ok {
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("无权访问 %s 分组", tokenGroup))
				return
			}
			// check group in common.GroupRatio
			if !ratio_setting.ContainsGroupRatio(tokenGroup) {
				if tokenGroup != "auto" {
					abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("分组 %s 已被弃用", tokenGroup))
					return
				}
			}
			userGroup = tokenGroup
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, userGroup)

		err = SetupContextForToken(c, token, parts...)
		if err != nil {
			return
		}
		c.Next()
	}
}

func SetupContextForToken(c *gin.Context, token *model.Token, parts ...string) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}
	c.Set("id", token.UserId)
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	c.Set("token_name", token.Name)
	c.Set("token_unlimited_quota", token.UnlimitedQuota)
	if !token.UnlimitedQuota {
		c.Set("token_quota", token.RemainQuota)
	}
	if token.ModelLimitsEnabled {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", token.GetModelLimitsMap())
	} else {
		c.Set("token_model_limit_enabled", false)
	}
	common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, token.CrossGroupRetry)
	if len(parts) > 1 {
		if model.IsAdmin(token.UserId) {
			c.Set("specific_channel_id", parts[1])
		} else {
			abortWithOpenAiMessage(c, http.StatusForbidden, "普通用户不支持指定渠道")
			return fmt.Errorf("普通用户不支持指定渠道")
		}
	}
	return nil
}

// TokenAuthForAPI 支持 token 鉴权的中间件，用于 API 接口（返回标准 JSON 响应）
// 复用 TokenAuth 的逻辑，但返回标准 JSON 响应而不是 OpenAI 格式
func TokenAuthForAPI() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 复用 TokenAuth 的 token 解析逻辑
		key := c.Request.Header.Get("Authorization")
		parts := make([]string, 0)
		key = strings.TrimPrefix(key, "Bearer ")
		if key == "" || key == "midjourney-proxy" {
			key = c.Request.Header.Get("mj-api-secret")
			key = strings.TrimPrefix(key, "Bearer ")
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		} else {
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		}

		// 使用 ValidateUserToken 查询 token 表（不是 users 表）
		token, err := model.ValidateUserToken(key)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "token 无效: " + err.Error(),
			})
			c.Abort()
			return
		}

		if token == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "token 无效",
			})
			c.Abort()
			return
		}

		// 检查 IP 限制
		allowIps := token.GetIpLimits()
		if len(allowIps) > 0 {
			clientIp := c.ClientIP()
			logger.LogDebug(c, "Token has IP restrictions, checking client IP %s", clientIp)
			ip := net.ParseIP(clientIp)
			if ip == nil {
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"message": "无法解析客户端 IP 地址",
				})
				c.Abort()
				return
			}
			if common.IsIpInCIDRList(ip, allowIps) == false {
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"message": "您的 IP 不在令牌允许访问的列表中",
				})
				c.Abort()
				return
			}
			logger.LogDebug(c, "Client IP %s passed the token IP restrictions check", clientIp)
		}

		// 检查用户状态
		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "获取用户信息失败: " + err.Error(),
			})
			c.Abort()
			return
		}

		userEnabled := userCache.Status == common.UserStatusEnabled
		if !userEnabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "用户已被封禁",
			})
			c.Abort()
			return
		}

		userCache.WriteContext(c)

		// 设置 token 相关信息到 context（复用 TokenAuth 的逻辑）
		c.Set("id", token.UserId)
		c.Set("token_id", token.Id)
		c.Set("token_key", token.Key)
		c.Set("token_name", token.Name)
		c.Set("token_unlimited_quota", token.UnlimitedQuota)
		if !token.UnlimitedQuota {
			c.Set("token_quota", token.RemainQuota)
		}
		if token.ModelLimitsEnabled {
			c.Set("token_model_limit_enabled", true)
			c.Set("token_model_limit", token.GetModelLimitsMap())
		} else {
			c.Set("token_model_limit_enabled", false)
		}
		common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
		common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, token.CrossGroupRetry)

		// 处理分组逻辑（复用 TokenAuth 的逻辑）
		userGroup := userCache.Group
		tokenGroup := token.Group
		if tokenGroup != "" {
			if _, ok := service.GetUserUsableGroups(userGroup)[tokenGroup]; !ok {
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"message": fmt.Sprintf("无权访问 %s 分组", tokenGroup),
				})
				c.Abort()
				return
			}
			if !ratio_setting.ContainsGroupRatio(tokenGroup) {
				if tokenGroup != "auto" {
					c.JSON(http.StatusForbidden, gin.H{
						"success": false,
						"message": fmt.Sprintf("分组 %s 已被弃用", tokenGroup),
					})
					c.Abort()
					return
				}
			}
			userGroup = tokenGroup
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, userGroup)

		c.Next()
	}
}
