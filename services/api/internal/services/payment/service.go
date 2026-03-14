package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentService struct {
	embyService *embyint.EmbyService
	httpClient  *http.Client
}

func NewPaymentService() *PaymentService {
	return &PaymentService{
		embyService: embyint.NewEmbyService(),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type CreatePlanRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Days        int    `json:"days" binding:"required,min=1"`
	Price       int64  `json:"price" binding:"required,min=1"`
	SortOrder   int    `json:"sortOrder"`
}

type UpdatePlanRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Days        *int    `json:"days" binding:"omitempty,min=1"`
	Price       *int64  `json:"price" binding:"omitempty,min=1"`
	IsActive    *bool   `json:"isActive"`
	SortOrder   *int    `json:"sortOrder"`
}

type GetPlansRequest struct {
	Page     int  `form:"page" binding:"omitempty,min=1"`
	PageSize int  `form:"pageSize" binding:"omitempty,min=1"`
	ShowAll  bool `form:"showAll"`
}

type GetPlansResponse struct {
	Data       []models.Plan `json:"data"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"pageSize"`
	TotalPages int           `json:"totalPages"`
}

type CreateCheckoutRequest struct {
	PlanID string `json:"planId" binding:"required"`
}

type CreateCheckoutResponse struct {
	URL string `json:"url"`
}

type PaymentWithMeta struct {
	models.Payment
	Username string `json:"username,omitempty"`
	PlanName string `json:"planName,omitempty"`
}

type GetPaymentsRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"pageSize" binding:"omitempty,min=1"`
	UserID   string `form:"userId"`
}

type GetPaymentsResponse struct {
	Data       []PaymentWithMeta `json:"data"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}

type stripeCheckoutSessionResponse struct {
	ID    string          `json:"id"`
	URL   string          `json:"url"`
	Error *stripeAPIError `json:"error"`
}

type stripeAPIError struct {
	Message string `json:"message"`
}

type stripeWebhookEvent struct {
	Type string `json:"type"`
	Data struct {
		Object stripeCheckoutSessionObject `json:"object"`
	} `json:"data"`
}

type stripeCheckoutSessionObject struct {
	ID            string            `json:"id"`
	PaymentIntent string            `json:"payment_intent"`
	PaymentStatus string            `json:"payment_status"`
	Metadata      map[string]string `json:"metadata"`
}

const pendingCheckoutTTL = 30 * time.Minute

func (s *PaymentService) CreatePlan(req *CreatePlanRequest) (*models.Plan, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("方案名称不能为空")
	}

	plan := models.Plan{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Days:        req.Days,
		Price:       req.Price,
		Currency:    "usd",
		IsActive:    true,
		SortOrder:   req.SortOrder,
	}

	if err := db.DB.Create(&plan).Error; err != nil {
		return nil, errors.New("创建方案失败")
	}
	return &plan, nil
}

func (s *PaymentService) UpdatePlan(id string, req *UpdatePlanRequest) (*models.Plan, error) {
	var plan models.Plan
	if err := db.DB.Where("id = ?", id).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlanNotFound
		}
		return nil, errors.New("获取方案失败")
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, errors.New("方案名称不能为空")
		}
		plan.Name = name
	}
	if req.Description != nil {
		plan.Description = strings.TrimSpace(*req.Description)
	}
	if req.Days != nil {
		plan.Days = *req.Days
	}
	if req.Price != nil {
		plan.Price = *req.Price
	}
	if req.IsActive != nil {
		plan.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		plan.SortOrder = *req.SortOrder
	}
	if strings.TrimSpace(plan.Currency) == "" {
		plan.Currency = "usd"
	}

	if err := db.DB.Save(&plan).Error; err != nil {
		return nil, errors.New("更新方案失败")
	}
	return &plan, nil
}

func (s *PaymentService) DeletePlan(id string) error {
	result := db.DB.Model(&models.Plan{}).
		Where("id = ? AND \"isActive\" = ?", id, true).
		Update("isActive", false)
	if result.Error != nil {
		return errors.New("下架方案失败")
	}
	if result.RowsAffected > 0 {
		return nil
	}

	var count int64
	if err := db.DB.Model(&models.Plan{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return errors.New("下架方案失败")
	}
	if count == 0 {
		return ErrPlanNotFound
	}

	return nil
}

func (s *PaymentService) GetPlans(req *GetPlansRequest) (*GetPlansResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	query := db.DB.Model(&models.Plan{})
	if !req.ShowAll {
		query = query.Where("\"isActive\" = ?", true)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("获取方案列表失败")
	}

	var plans []models.Plan
	offset := (page - 1) * pageSize
	if err := query.Order("\"sortOrder\" ASC, \"createdAt\" DESC").Offset(offset).Limit(pageSize).Find(&plans).Error; err != nil {
		return nil, errors.New("获取方案列表失败")
	}

	return &GetPlansResponse{
		Data:       plans,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}

func (s *PaymentService) GetActivePlans() ([]models.Plan, error) {
	var plans []models.Plan
	if err := db.DB.Where("\"isActive\" = ?", true).Order("\"sortOrder\" ASC, \"createdAt\" DESC").Find(&plans).Error; err != nil {
		return nil, errors.New("获取方案列表失败")
	}
	return plans, nil
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func shouldReusePendingPayment(payment models.Payment, now time.Time) bool {
	if payment.Status != models.PaymentPending {
		return false
	}
	if strings.TrimSpace(payment.CheckoutURL) == "" {
		return false
	}
	if payment.ExpiresAt == nil {
		return false
	}
	return payment.ExpiresAt.After(now.UTC())
}

func (s *PaymentService) expirePendingPayments(userID, planID string, now time.Time) error {
	query := db.DB.Model(&models.Payment{}).
		Where("status = ?", models.PaymentPending).
		Where("\"expiresAt\" IS NOT NULL AND \"expiresAt\" <= ?", now.UTC())
	if strings.TrimSpace(userID) != "" {
		query = query.Where("\"userId\" = ?", userID)
	}
	if strings.TrimSpace(planID) != "" {
		query = query.Where("\"planId\" = ?", planID)
	}
	result := query.Update("status", models.PaymentExpired)
	if result.Error != nil {
		return errors.New("更新支付记录失败")
	}
	if result.RowsAffected > 0 {
		log.Printf("[Payment] 已标记过期订单: userID=%s planID=%s count=%d", strings.TrimSpace(userID), strings.TrimSpace(planID), result.RowsAffected)
	}
	return nil
}

func (s *PaymentService) findReusablePendingPayment(userID, planID string, now time.Time) (*models.Payment, error) {
	var payment models.Payment
	err := db.DB.
		Where("\"userId\" = ? AND \"planId\" = ? AND status = ?", userID, planID, models.PaymentPending).
		Order("\"createdAt\" DESC").
		First(&payment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("获取支付记录失败")
	}
	if !shouldReusePendingPayment(payment, now) {
		return nil, nil
	}
	return &payment, nil
}

func (s *PaymentService) CreateCheckoutSession(userID string, req *CreateCheckoutRequest) (*CreateCheckoutResponse, error) {
	log.Printf("[Payment] 开始创建支付会话: userID=%s planID=%s", userID, strings.TrimSpace(req.PlanID))

	configService := configpkg.NewConfigService()
	stripeSecret := strings.TrimSpace(configService.GetString("STRIPE_SECRET_KEY"))
	successURL := strings.TrimSpace(configService.GetString("STRIPE_SUCCESS_URL"))
	cancelURL := strings.TrimSpace(configService.GetString("STRIPE_CANCEL_URL"))
	if stripeSecret == "" || successURL == "" || cancelURL == "" {
		log.Printf("[Payment] Stripe 配置缺失，无法创建支付会话: userID=%s planID=%s hasSecret=%t hasSuccessURL=%t hasCancelURL=%t",
			userID, strings.TrimSpace(req.PlanID), stripeSecret != "", successURL != "", cancelURL != "")
		return nil, ErrStripeNotConfigured
	}

	var plan models.Plan
	if err := db.DB.Where("id = ? AND \"isActive\" = ?", req.PlanID, true).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[Payment] 方案不存在或未启用: userID=%s planID=%s", userID, strings.TrimSpace(req.PlanID))
			return nil, ErrPlanNotFound
		}
		log.Printf("[Payment] 查询方案失败: userID=%s planID=%s err=%v", userID, strings.TrimSpace(req.PlanID), err)
		return nil, errors.New("获取方案失败")
	}

	paymentMethods, err := configService.GetStripeAllowedPaymentMethods()
	if err != nil {
		log.Printf("[Payment] 读取支付方式限制失败: userID=%s planID=%s err=%v", userID, plan.ID, err)
		return nil, err
	}

	if err := s.expirePendingPayments(userID, plan.ID, time.Now().UTC()); err != nil {
		return nil, err
	}

	if existing, err := s.findReusablePendingPayment(userID, plan.ID, time.Now().UTC()); err != nil {
		log.Printf("[Payment] 查询可复用待支付订单失败: userID=%s planID=%s err=%v", userID, plan.ID, err)
		return nil, err
	} else if existing != nil {
		log.Printf("[Payment] 复用待支付订单: userID=%s planID=%s paymentID=%s sessionID=%s expiresAt=%s",
			userID, plan.ID, existing.ID, existing.StripeSessionID, existing.ExpiresAt.UTC().Format(time.RFC3339))
		return &CreateCheckoutResponse{URL: existing.CheckoutURL}, nil
	}

	sess, err := s.createStripeCheckoutSession(stripeSecret, successURL, cancelURL, userID, &plan, paymentMethods)
	if err != nil {
		log.Printf("[Payment] Stripe 会话创建失败: userID=%s planID=%s err=%v", userID, plan.ID, err)
		return nil, err
	}

	payment := models.Payment{
		UserID:          userID,
		PlanID:          plan.ID,
		StripeSessionID: sess.ID,
		CheckoutURL:     strings.TrimSpace(sess.URL),
		Amount:          plan.Price,
		Currency:        plan.Currency,
		Days:            plan.Days,
		Status:          models.PaymentPending,
		ExpiresAt:       timePtr(time.Now().UTC().Add(pendingCheckoutTTL)),
	}
	if err := db.DB.Create(&payment).Error; err != nil {
		log.Printf("[Payment] 创建支付记录失败: userID=%s planID=%s sessionID=%s err=%v", userID, plan.ID, sess.ID, err)
		return nil, errors.New("创建支付记录失败")
	}

	log.Printf("[Payment] 创建支付会话成功: userID=%s planID=%s paymentID=%s sessionID=%s amount=%d currency=%s days=%d expiresAt=%s methods=%v",
		userID, plan.ID, payment.ID, sess.ID, plan.Price, plan.Currency, plan.Days, payment.ExpiresAt.UTC().Format(time.RFC3339), paymentMethods)

	return &CreateCheckoutResponse{URL: sess.URL}, nil
}

func (s *PaymentService) createStripeCheckoutSession(secret, successURL, cancelURL, userID string, plan *models.Plan, paymentMethods []string) (*stripeCheckoutSessionResponse, error) {
	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("success_url", successURL)
	form.Set("cancel_url", cancelURL)
	form.Set("line_items[0][price_data][currency]", plan.Currency)
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(plan.Price, 10))
	form.Set("line_items[0][price_data][product_data][name]", plan.Name)
	if description := strings.TrimSpace(plan.Description); description != "" {
		form.Set("line_items[0][price_data][product_data][description]", description)
	}
	form.Set("line_items[0][quantity]", "1")
	form.Set("metadata[user_id]", userID)
	form.Set("metadata[plan_id]", plan.ID)
	form.Set("metadata[days]", strconv.Itoa(plan.Days))
	for _, method := range paymentMethods {
		form.Add("payment_method_types[]", method)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.New("创建支付会话失败")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, errors.New("创建支付会话失败")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("创建支付会话失败")
	}

	var result stripeCheckoutSessionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("创建支付会话失败")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
			return nil, fmt.Errorf("创建支付会话失败: %s", result.Error.Message)
		}
		return nil, errors.New("创建支付会话失败")
	}

	if strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.URL) == "" {
		return nil, errors.New("创建支付会话失败")
	}

	log.Printf("[Payment] Stripe 返回 checkout session: userID=%s planID=%s sessionID=%s", userID, plan.ID, result.ID)

	return &result, nil
}

func (s *PaymentService) HandleWebhook(r *http.Request) error {
	webhookSecret := strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if webhookSecret == "" {
		return ErrStripeNotConfigured
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.New("读取 webhook 请求失败")
	}

	if err := verifyStripeSignature(r.Header.Get("Stripe-Signature"), payload, webhookSecret); err != nil {
		log.Printf("[Payment] Webhook 签名验证失败: err=%v", err)
		return fmt.Errorf("webhook 签名验证失败: %w", err)
	}

	var event stripeWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return errors.New("解析 webhook 数据失败")
	}

	log.Printf("[Payment] 收到 Stripe webhook: type=%s sessionID=%s paymentStatus=%s paymentIntent=%s",
		event.Type,
		strings.TrimSpace(event.Data.Object.ID),
		strings.TrimSpace(event.Data.Object.PaymentStatus),
		strings.TrimSpace(event.Data.Object.PaymentIntent),
	)

	switch event.Type {
	case "checkout.session.completed":
		// 异步支付方式下 completed 可能先于真实到账，必须确认 paid 才履约。
		if event.Data.Object.PaymentStatus != "paid" {
			log.Printf("[Payment] 忽略未支付完成事件: type=%s sessionID=%s paymentStatus=%s",
				event.Type, strings.TrimSpace(event.Data.Object.ID), strings.TrimSpace(event.Data.Object.PaymentStatus))
			return nil
		}
		return s.fulfillPayment(event.Data.Object.ID, event.Data.Object.PaymentIntent, event.Data.Object.Metadata)
	case "checkout.session.async_payment_succeeded":
		return s.fulfillPayment(event.Data.Object.ID, event.Data.Object.PaymentIntent, event.Data.Object.Metadata)
	case "checkout.session.async_payment_failed":
		return s.markPaymentFailed(event.Data.Object.ID)
	default:
		return nil
	}
}

func verifyStripeSignature(signatureHeader string, payload []byte, secret string) error {
	timestamp, signatures, err := parseStripeSignature(signatureHeader)
	if err != nil {
		return err
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("签名时间戳无效")
	}

	if absInt64(time.Now().Unix()-ts) > 300 {
		return errors.New("签名已过期")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, signature := range signatures {
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1 {
			return nil
		}
	}

	return errors.New("签名不匹配")
}

func parseStripeSignature(signatureHeader string) (string, []string, error) {
	if strings.TrimSpace(signatureHeader) == "" {
		return "", nil, errors.New("缺少签名头")
	}

	parts := strings.Split(signatureHeader, ",")
	timestamp := ""
	signatures := make([]string, 0, 2)

	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return "", nil, errors.New("签名头格式错误")
	}

	return timestamp, signatures, nil
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (s *PaymentService) fulfillPayment(sessionID, paymentIntentID string, metadata map[string]string) error {
	if strings.TrimSpace(sessionID) == "" {
		log.Printf("[Payment] 支付履约失败：缺少 sessionID")
		return ErrPaymentFailed
	}
	log.Printf("[Payment] 开始履约支付: sessionID=%s paymentIntent=%s metadata=%v", strings.TrimSpace(sessionID), strings.TrimSpace(paymentIntentID), metadata)

	tx := db.DB.Begin()
	if tx.Error != nil {
		log.Printf("[Payment] 开启支付履约事务失败: sessionID=%s err=%v", strings.TrimSpace(sessionID), tx.Error)
		return ErrPaymentFailed
	}

	var payment models.Payment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("\"stripeSessionId\" = ?", sessionID).
		First(&payment).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约查询订单失败: sessionID=%s err=%v", strings.TrimSpace(sessionID), err)
		return ErrPaymentFailed
	}

	if payment.Status == models.PaymentCompleted {
		tx.Rollback()
		log.Printf("[Payment] 支付已履约，忽略重复 webhook: paymentID=%s sessionID=%s", payment.ID, payment.StripeSessionID)
		return nil
	}
	// 失败态是终态，禁止后续成功事件回写为 completed，避免状态穿越。
	if payment.Status == models.PaymentFailed {
		tx.Rollback()
		log.Printf("[Payment] 支付已标记失败，忽略成功回调: paymentID=%s sessionID=%s", payment.ID, payment.StripeSessionID)
		return nil
	}
	if payment.Status == models.PaymentExpired {
		tx.Rollback()
		log.Printf("[Payment] 支付订单已过期，忽略成功回调: paymentID=%s sessionID=%s", payment.ID, payment.StripeSessionID)
		return nil
	}

	var user models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", payment.UserID).First(&user).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约查询用户失败: paymentID=%s userID=%s err=%v", payment.ID, payment.UserID, err)
		return ErrPaymentFailed
	}

	now := time.Now().UTC()
	var oldExpiry *time.Time
	if user.ExpiresAt != nil {
		copied := *user.ExpiresAt
		oldExpiry = &copied
	}
	var newExpiry time.Time
	if user.ExpiresAt == nil || user.ExpiresAt.Before(now) {
		newExpiry = now.AddDate(0, 0, payment.Days)
	} else {
		newExpiry = user.ExpiresAt.AddDate(0, 0, payment.Days)
	}

	user.ExpiresAt = &newExpiry
	if user.EmbyDisabled && user.IsActive {
		if err := s.embyService.SetUserPolicy(user.EmbyID, embyint.EmbyUserPolicy{IsDisabled: false}); err != nil {
			tx.Rollback()
			log.Printf("[Payment] 支付履约解封 Emby 失败: paymentID=%s userID=%s embyID=%s err=%v", payment.ID, payment.UserID, user.EmbyID, err)
			return ErrEmbyUnbanFailed
		}
		user.EmbyDisabled = false
	}

	if err := tx.Save(&user).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约保存用户失败: paymentID=%s userID=%s err=%v", payment.ID, payment.UserID, err)
		return ErrPaymentFailed
	}

	payment.Status = models.PaymentCompleted
	if strings.TrimSpace(paymentIntentID) != "" {
		payment.StripePaymentIntentID = paymentIntentID
	}
	if err := tx.Save(&payment).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约保存订单失败: paymentID=%s sessionID=%s err=%v", payment.ID, payment.StripeSessionID, err)
		return ErrPaymentFailed
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("[Payment] 支付履约提交事务失败: paymentID=%s sessionID=%s err=%v", payment.ID, payment.StripeSessionID, err)
		return ErrPaymentFailed
	}
	log.Printf("[Payment] 支付履约成功: paymentID=%s userID=%s planID=%s sessionID=%s oldExpiresAt=%v newExpiresAt=%s days=%d",
		payment.ID, payment.UserID, payment.PlanID, payment.StripeSessionID, oldExpiry, newExpiry.Format(time.RFC3339), payment.Days)
	return nil
}

func (s *PaymentService) markPaymentFailed(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		log.Printf("[Payment] 标记支付失败时缺少 sessionID")
		return ErrPaymentFailed
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return ErrPaymentFailed
	}

	var payment models.Payment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("\"stripeSessionId\" = ?", sessionID).
		First(&payment).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 标记支付失败查询订单失败: sessionID=%s err=%v", strings.TrimSpace(sessionID), err)
		return ErrPaymentFailed
	}

	// 已履约记录不可被失败事件回滚，避免重复 webhook 破坏状态。
	if payment.Status == models.PaymentCompleted {
		tx.Rollback()
		log.Printf("[Payment] 支付已完成，忽略失败回调: paymentID=%s sessionID=%s", payment.ID, payment.StripeSessionID)
		return nil
	}
	if payment.Status == models.PaymentExpired {
		tx.Rollback()
		log.Printf("[Payment] 支付已过期，忽略失败回调: paymentID=%s sessionID=%s", payment.ID, payment.StripeSessionID)
		return nil
	}
	if payment.Status == models.PaymentFailed {
		tx.Rollback()
		log.Printf("[Payment] 支付已失败，忽略重复失败回调: paymentID=%s sessionID=%s", payment.ID, payment.StripeSessionID)
		return nil
	}

	payment.Status = models.PaymentFailed
	if err := tx.Save(&payment).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 标记支付失败保存订单失败: paymentID=%s sessionID=%s err=%v", payment.ID, payment.StripeSessionID, err)
		return ErrPaymentFailed
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("[Payment] 标记支付失败提交事务失败: paymentID=%s sessionID=%s err=%v", payment.ID, payment.StripeSessionID, err)
		return ErrPaymentFailed
	}
	log.Printf("[Payment] 已标记支付失败: paymentID=%s userID=%s planID=%s sessionID=%s", payment.ID, payment.UserID, payment.PlanID, payment.StripeSessionID)
	return nil
}

func (s *PaymentService) GetUserPayments(userID string, req *GetPaymentsRequest) (*GetPaymentsResponse, error) {
	return s.getPayments(userID, req, false)
}

func (s *PaymentService) GetAllPayments(req *GetPaymentsRequest) (*GetPaymentsResponse, error) {
	return s.getPayments("", req, true)
}

func (s *PaymentService) getPayments(userID string, req *GetPaymentsRequest, isAdmin bool) (*GetPaymentsResponse, error) {
	if !isAdmin {
		if err := s.expirePendingPayments(userID, "", time.Now().UTC()); err != nil {
			return nil, err
		}
	} else {
		if err := s.expirePendingPayments(strings.TrimSpace(req.UserID), "", time.Now().UTC()); err != nil {
			return nil, err
		}
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	query := db.DB.Model(&models.Payment{})
	if isAdmin {
		if strings.TrimSpace(req.UserID) != "" {
			query = query.Where("\"userId\" = ?", req.UserID)
		}
	} else {
		query = query.Where("\"userId\" = ?", userID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("获取支付记录失败")
	}

	var rows []models.Payment
	offset := (page - 1) * pageSize
	if err := query.Order("\"createdAt\" DESC").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errors.New("获取支付记录失败")
	}

	planIDs := make([]string, 0, len(rows))
	userIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		planIDs = append(planIDs, row.PlanID)
		userIDs = append(userIDs, row.UserID)
	}

	planNameMap := make(map[string]string, len(planIDs))
	if len(planIDs) > 0 {
		var plans []models.Plan
		if err := db.DB.Select("id", "name").Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
			return nil, errors.New("获取支付记录失败")
		}
		for i := range plans {
			planNameMap[plans[i].ID] = plans[i].Name
		}
	}

	usernameMap := make(map[string]string, len(userIDs))
	if isAdmin && len(userIDs) > 0 {
		var users []models.User
		if err := db.DB.Select("id", "username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, errors.New("获取支付记录失败")
		}
		for i := range users {
			usernameMap[users[i].ID] = users[i].Username
		}
	}

	result := make([]PaymentWithMeta, len(rows))
	for i, row := range rows {
		result[i] = PaymentWithMeta{
			Payment:  row,
			PlanName: planNameMap[row.PlanID],
		}
		if isAdmin {
			result[i].Username = usernameMap[row.UserID]
		}
	}

	return &GetPaymentsResponse{
		Data:       result,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}
