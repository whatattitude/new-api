package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	// 获取支付方式
	payMethods := operation_setting.PayMethods

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "" {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	data := gin.H{
		"enable_online_topup": operation_setting.PayAddress != "" && operation_setting.EpayId != "" && operation_setting.EpayKey != "",
		"enable_stripe_topup": setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "",
		"enable_creem_topup":  setting.CreemApiKey != "" && setting.CreemProducts != "[]",
		"creem_products":      setting.CreemProducts,
		"pay_methods":         payMethods,
		"min_topup":           operation_setting.MinTopUp,
		"stripe_min_topup":    setting.StripeMinTopUp,
		"amount_options":      operation_setting.GetPaymentSetting().AmountOptions,
		"discount":            operation_setting.GetPaymentSetting().AmountDiscount,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64   `json:"amount"`         // 充值数量（美元）
	PaymentMethod string  `json:"payment_method"` // 支付方式
	TopUpCode     string  `json:"top_up_code"`    // 充值码
	ActualAmount  float64 `json:"actual_amount"`  // 实际到账金额（美元，已应用倍率）
	BonusAmount   float64 `json:"bonus_amount"`   // 赠送金额（人民币）
}

type AmountRequest struct {
	Amount    int64  `json:"amount"`
	TopUpCode string `json:"top_up_code"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(200, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	// 获取用户信息以获取充值倍率
	user, err := model.GetUserById(id, false)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户信息失败"})
		return
	}

	// 计算实际到账金额和赠送金额
	multiplier := user.TopupMultiplier
	if multiplier <= 0 {
		multiplier = 1.0
	}
	actualAmount := req.ActualAmount
	bonusAmount := req.BonusAmount
	
	// 如果前端没有传递，则后端计算
	if actualAmount <= 0 {
		actualAmount = float64(req.Amount) * multiplier
	}
	if bonusAmount <= 0 {
		bonusAmount = payMoney * (multiplier - 1)
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/log")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(200, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:        id,
		Amount:        amount,
		Money:         payMoney,
		ActualAmount:  actualAmount, // 实际到账金额（美元，已应用倍率）
		BonusAmount:   bonusAmount, // 赠送金额（人民币）
		TradeNo:       tradeNo,
		PaymentMethod: req.PaymentMethod,
		CreateTime:    time.Now().Unix(),
		Status:        "pending",
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if !ok {
		createLock.Lock()
		defer createLock.Unlock()
		lock, ok = orderLocks.Load(tradeNo)
		if !ok {
			lock = new(sync.Mutex)
			orderLocks.Store(tradeNo, lock)
		}
	}
	lock.(*sync.Mutex).Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if ok {
		lock.(*sync.Mutex).Unlock()
	}
}

func EpayNotify(c *gin.Context) {
	params := lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
		r[t] = c.Request.URL.Query().Get(t)
		return r
	}, map[string]string{})
	client := GetEpayClient()
	if client == nil {
		log.Println("易支付回调失败 未找到配置信息")
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err == nil && verifyInfo.VerifyStatus {
		_, err := c.Writer.Write([]byte("success"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
	} else {
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
		log.Println("易支付回调签名验证失败")
		return
	}

	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		log.Println(verifyInfo)
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		topUp := model.GetTopUpByTradeNo(verifyInfo.ServiceTradeNo)
		if topUp == nil {
			log.Printf("易支付回调未找到订单: %v", verifyInfo)
			return
		}
		if topUp.Status == "pending" {
			// 使用 Recharge 函数处理充值成功，它会使用订单中保存的实际到账金额和赠送金额
			err := model.Recharge(verifyInfo.ServiceTradeNo, "")
			if err != nil {
				log.Printf("易支付回调充值失败: %v, 错误: %v", topUp, err)
				return
			}
			log.Printf("易支付回调充值成功 %v", topUp)
		}
	} else {
		log.Printf("易支付异常回调: %v", verifyInfo)
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 获取用户信息以获取充值倍率
	user, err := model.GetUserById(userId, false)
	multiplier := 1.0
	if err == nil && user != nil {
		multiplier = user.TopupMultiplier
		if multiplier <= 0 {
			multiplier = 1.0
		}
	}

	// 将 topups 转换为包含 actual_money 的 map 列表
	items := make([]map[string]interface{}, len(topups))
	for i, topup := range topups {
		item := map[string]interface{}{
			"id":             topup.Id,
			"user_id":        topup.UserId,
			"amount":         topup.Amount,
			"money":          topup.Money,
			"actual_money":   topup.Money * multiplier, // 实际到账金额（已应用倍率）
			"trade_no":       topup.TradeNo,
			"payment_method": topup.PaymentMethod,
			"create_time":    topup.CreateTime,
			"complete_time":  topup.CompleteTime,
			"status":         topup.Status,
			"invoice_status": topup.InvoiceStatus,
			"invoice_id":     topup.InvoiceId,
		}
		items[i] = item
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

type UpdateInvoiceStatusRequest struct {
	Id            int    `json:"id" binding:"required"`
	InvoiceStatus string `json:"invoice_status" binding:"required"`
}

// UpdateInvoiceStatus 更新发票状态（用户申请发票）
func UpdateInvoiceStatus(c *gin.Context) {
	var req UpdateInvoiceStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 验证发票状态值
	validStatuses := []string{common.InvoiceStatusNotApplied, common.InvoiceStatusPending, common.InvoiceStatusSent}
	isValid := false
	for _, status := range validStatuses {
		if req.InvoiceStatus == status {
			isValid = true
			break
		}
	}
	if !isValid {
		common.ApiErrorMsg(c, "无效的发票状态")
		return
	}

	userId := c.GetInt("id")
	topUp := model.GetTopUpById(req.Id)
	if topUp == nil {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}

	// 用户只能修改自己的订单
	if topUp.UserId != userId {
		common.ApiErrorMsg(c, "无权操作此订单")
		return
	}

	// 只有状态为"未申请"的订单才能申请发票
	if topUp.InvoiceStatus != common.InvoiceStatusNotApplied && req.InvoiceStatus == common.InvoiceStatusPending {
		common.ApiErrorMsg(c, "该订单已申请过发票")
		return
	}

	// 更新发票状态
	topUp.InvoiceStatus = req.InvoiceStatus
	if err := topUp.Update(); err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, nil)
}

type BatchUpdateInvoiceStatusRequest struct {
	Ids           []int  `json:"ids" binding:"required"`
	InvoiceStatus string `json:"invoice_status" binding:"required"`
}

// BatchUpdateInvoiceStatus 批量更新发票状态（用户批量申请发票）
func BatchUpdateInvoiceStatus(c *gin.Context) {
	var req BatchUpdateInvoiceStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	if len(req.Ids) == 0 {
		common.ApiErrorMsg(c, "请选择至少一个订单")
		return
	}

	// 验证发票状态值
	validStatuses := []string{common.InvoiceStatusNotApplied, common.InvoiceStatusPending, common.InvoiceStatusSent}
	isValid := false
	for _, status := range validStatuses {
		if req.InvoiceStatus == status {
			isValid = true
			break
		}
	}
	if !isValid {
		common.ApiErrorMsg(c, "无效的发票状态")
		return
	}

	userId := c.GetInt("id")
	successCount := 0
	failedCount := 0
	var failedOrders []string

	for _, id := range req.Ids {
		topUp := model.GetTopUpById(id)
		if topUp == nil {
			failedCount++
			continue
		}

		// 用户只能修改自己的订单
		if topUp.UserId != userId {
			failedCount++
			failedOrders = append(failedOrders, topUp.TradeNo)
			continue
		}

		// 只有状态为"未申请"的订单才能申请发票
		if topUp.InvoiceStatus != common.InvoiceStatusNotApplied && req.InvoiceStatus == common.InvoiceStatusPending {
			failedCount++
			failedOrders = append(failedOrders, topUp.TradeNo)
			continue
		}

		// 更新发票状态
		topUp.InvoiceStatus = req.InvoiceStatus
		if err := topUp.Update(); err != nil {
			failedCount++
			failedOrders = append(failedOrders, topUp.TradeNo)
			continue
		}

		successCount++
	}

	if failedCount > 0 {
		common.ApiSuccess(c, gin.H{
			"success_count": successCount,
			"failed_count":  failedCount,
			"failed_orders": failedOrders,
			"message":       fmt.Sprintf("成功处理 %d 个订单，失败 %d 个订单", successCount, failedCount),
		})
	} else {
		common.ApiSuccess(c, gin.H{
			"success_count": successCount,
			"message":       fmt.Sprintf("成功处理 %d 个订单", successCount),
		})
	}
}

type CreateInvoiceRequest struct {
	TopUpIds []int `json:"top_up_ids" binding:"required"` // 订单ID列表
}

// CreateInvoice 管理员创建发票并关联订单
func CreateInvoice(c *gin.Context) {
	// 解析multipart form
	form, err := c.MultipartForm()
	if err != nil {
		common.ApiErrorMsg(c, "解析表单失败")
		return
	}
	defer form.RemoveAll()

	// 获取文件
	files := form.File["file"]
	if len(files) == 0 {
		common.ApiErrorMsg(c, "请上传PDF文件")
		return
	}
	if len(files) > 1 {
		common.ApiErrorMsg(c, "一次只能上传一个文件")
		return
	}

	file := files[0]
	// 验证文件类型
	if file.Header.Get("Content-Type") != "application/pdf" {
		// 也检查文件扩展名
		if len(file.Filename) < 4 || file.Filename[len(file.Filename)-4:] != ".pdf" {
			common.ApiErrorMsg(c, "只支持PDF格式文件")
			return
		}
	}

	// 获取订单ID列表
	topUpIdsStr := form.Value["top_up_ids"]
	if len(topUpIdsStr) == 0 {
		common.ApiErrorMsg(c, "请选择至少一个订单")
		return
	}

	var topUpIds []int
	for _, idStr := range topUpIdsStr {
		var ids []int
		if err := json.Unmarshal([]byte(idStr), &ids); err != nil {
			common.ApiErrorMsg(c, "订单ID格式错误")
			return
		}
		topUpIds = append(topUpIds, ids...)
	}

	if len(topUpIds) == 0 {
		common.ApiErrorMsg(c, "请选择至少一个订单")
		return
	}

	// 获取发票金额（前端计算的总和）
	var invoiceAmount float64
	amountStr := form.Value["amount"]
	if len(amountStr) > 0 {
		if amount, err := strconv.ParseFloat(amountStr[0], 64); err == nil {
			invoiceAmount = amount
		}
	}

	// 读取文件内容
	fileHandle, err := file.Open()
	if err != nil {
		common.ApiErrorMsg(c, "读取文件失败")
		return
	}
	defer fileHandle.Close()

	fileData, err := io.ReadAll(fileHandle)
	if err != nil {
		common.ApiErrorMsg(c, "读取文件内容失败")
		return
	}

	// 验证所有订单都属于同一个用户，且状态为"待开"
	var userId int
	for i, id := range topUpIds {
		topUp := model.GetTopUpById(id)
		if topUp == nil {
			common.ApiErrorMsg(c, fmt.Sprintf("订单 %d 不存在", id))
			return
		}
		if topUp.Status != common.TopUpStatusSuccess {
			common.ApiErrorMsg(c, fmt.Sprintf("订单 %s 状态不正确，只能为已完成的订单开具发票", topUp.TradeNo))
			return
		}
		if topUp.InvoiceStatus != common.InvoiceStatusPending {
			common.ApiErrorMsg(c, fmt.Sprintf("订单 %s 发票状态不正确，只能为待开状态的订单开具发票", topUp.TradeNo))
			return
		}
		if i == 0 {
			userId = topUp.UserId
		} else if topUp.UserId != userId {
			common.ApiErrorMsg(c, "所选订单必须属于同一个用户")
			return
		}
	}

	// 创建发票记录
	invoice := &model.Invoice{
		UserId:     userId,
		FileName:   file.Filename,
		FileData:   fileData,
		FileSize:   int64(len(fileData)),
		MimeType:   "application/pdf",
		Amount:     invoiceAmount,
		CreateTime: common.GetTimestamp(),
	}

	if err := invoice.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}

	// 更新所有订单的发票ID和状态
	successCount := 0
	for _, id := range topUpIds {
		topUp := model.GetTopUpById(id)
		if topUp == nil {
			continue
		}
		topUp.InvoiceId = invoice.Id
		topUp.InvoiceStatus = common.InvoiceStatusSent
		if err := topUp.Update(); err != nil {
			continue
		}
		successCount++
	}

	common.ApiSuccess(c, gin.H{
		"invoice_id":    invoice.Id,
		"success_count": successCount,
		"message":       fmt.Sprintf("发票创建成功，已关联 %d 个订单", successCount),
	})
}

// GetUserInvoices 用户获取自己的发票列表
func GetUserInvoices(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)

	invoices, total, err := model.GetInvoicesByUserId(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(invoices)
	common.ApiSuccess(c, pageInfo)
}

// GetAllInvoices 管理员获取所有发票
func GetAllInvoices(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userIdStr := c.Query("user_id")

	var (
		invoices []*model.Invoice
		total    int64
		err      error
	)

	if userIdStr != "" {
		userId, err := strconv.Atoi(userIdStr)
		if err != nil {
			common.ApiErrorMsg(c, "用户ID格式错误")
			return
		}
		invoices, total, err = model.SearchInvoicesByUserId(userId, pageInfo)
	} else {
		invoices, total, err = model.GetAllInvoices(pageInfo)
	}

	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(invoices)
	common.ApiSuccess(c, pageInfo)
}

// GetInvoiceFile 获取发票PDF文件
func GetInvoiceFile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiErrorMsg(c, "发票ID格式错误")
		return
	}

	invoice := model.GetInvoiceById(id)
	if invoice == nil {
		common.ApiErrorMsg(c, "发票不存在")
		return
	}

	// 检查权限：管理员或发票所属用户
	userId := c.GetInt("id")
	userRole := c.GetInt("role")
	if userRole != common.RoleRootUser && userRole != common.RoleAdminUser && invoice.UserId != userId {
		common.ApiErrorMsg(c, "无权访问此发票")
		return
	}

	c.Header("Content-Type", invoice.MimeType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", invoice.FileName))
	c.Data(200, invoice.MimeType, invoice.FileData)
}
