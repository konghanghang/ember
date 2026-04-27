package payment

import (
	"context"
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

	"github.com/konghang/ember/backend/internal/async"
	"github.com/konghang/ember/backend/internal/common/upstream"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentService struct {
	embyService  *embyint.EmbyService
	httpClient   *http.Client
	notifier     *notifierint.BotNotifier
	compensation *accountpkg.EmbyCompensation
}

func NewPaymentService() *PaymentService {
	embyService := embyint.GetSharedService()
	return &PaymentService{
		embyService: embyService,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		notifier:     notifierint.GetSharedBotNotifier(),
		compensation: accountpkg.NewEmbyCompensation(embyService),
	}
}

type CreatePlanRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Days        int    `json:"days" binding:"required,min=1"`
	Price       int64  `json:"price" binding:"required,min=1"`
	Currency    string `json:"currency"`
	PlanGroup   string `json:"planGroup"`
	SortOrder   int    `json:"sortOrder"`
}

type UpdatePlanRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Days        *int    `json:"days" binding:"omitempty,min=1"`
	Price       *int64  `json:"price" binding:"omitempty,min=1"`
	Currency    *string `json:"currency"`
	PlanGroup   *string `json:"planGroup"`
	IsActive    *bool   `json:"isActive"`
	SortOrder   *int    `json:"sortOrder"`
}

type GetPlansRequest struct {
	Page      int    `form:"page" binding:"omitempty,min=1"`
	PageSize  int    `form:"pageSize" binding:"omitempty,min=1"`
	ShowAll   bool   `form:"showAll"`
	PlanGroup string `form:"planGroup"`
}

type GetPlansResponse struct {
	Data       []PlanView `json:"data"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	TotalPages int        `json:"totalPages"`
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
	PlanID   string `form:"planId"`
	Status   string `form:"status"`
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
	ID       string `json:"id"`
	Type     string `json:"type"`
	Livemode bool   `json:"livemode"`
	Created  int64  `json:"created"`
	Data     struct {
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

func pendingStripeSessionIDsByScope(tx *gorm.DB, userID, planID string) ([]string, error) {
	if tx == nil {
		if db.DB == nil {
			return nil, nil
		}
		tx = db.DB
	}
	userID = strings.TrimSpace(userID)
	planID = strings.TrimSpace(planID)
	if userID == "" && planID == "" {
		panic("pendingStripeSessionIDsByScope requires userId or planId; misuse will read all pending payments")
	}

	query := tx.Model(&models.Payment{}).
		Where("status = ?", models.PaymentPending).
		Where(`"stripeSessionId" <> ''`)
	if userID != "" {
		query = query.Where(`"userId" = ?`, userID)
	}
	if planID != "" {
		query = query.Where(`"planId" = ?`, planID)
	}

	sessionIDs := make([]string, 0)
	if err := query.Pluck(`"stripeSessionId"`, &sessionIDs).Error; err != nil {
		return nil, errors.New("查询待失效 Stripe 会话失败")
	}
	return sessionIDs, nil
}

func PendingStripeSessionIDsForUser(tx *gorm.DB, userID string) ([]string, error) {
	return pendingStripeSessionIDsByScope(tx, userID, "")
}

func PendingStripeSessionIDsForPlan(tx *gorm.DB, planID string) ([]string, error) {
	return pendingStripeSessionIDsByScope(tx, "", planID)
}

func normalizePlanCurrency(raw string) (string, error) {
	currency := strings.ToLower(strings.TrimSpace(raw))
	if currency == "" {
		return "usd", nil
	}
	switch currency {
	case "usd", "hkd", "cny":
		return currency, nil
	default:
		return "", ErrPlanCurrencyInvalid
	}
}

func buildPlansWithGroupNameSelect(query *gorm.DB) *gorm.DB {
	return query.
		Select(`plans.*, plan_groups.name AS "planGroupName"`).
		Joins(`LEFT JOIN plan_groups ON plan_groups.key = plans."planGroup"`)
}

func (s *PaymentService) CreatePlan(req *CreatePlanRequest) (*PlanView, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("方案名称不能为空")
	}
	currency, err := normalizePlanCurrency(req.Currency)
	if err != nil {
		return nil, err
	}
	planGroup, err := NormalizePlanGroupKey(req.PlanGroup, false)
	if err != nil {
		return nil, err
	}
	if _, err := GetPlanGroupByKey(nil, planGroup); err != nil {
		return nil, err
	}

	plan := models.Plan{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Days:        req.Days,
		Price:       req.Price,
		Currency:    currency,
		PlanGroup:   planGroup,
		IsActive:    true,
		SortOrder:   req.SortOrder,
	}

	if err := db.DB.Create(&plan).Error; err != nil {
		return nil, errors.New("创建方案失败")
	}
	return s.getPlanByID(plan.ID)
}

func (s *PaymentService) UpdatePlan(id string, req *UpdatePlanRequest) (*PlanView, error) {
	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("更新方案失败")
	}

	var plan models.Plan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&plan).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlanNotFound
		}
		return nil, errors.New("获取方案失败")
	}

	planGroupChanged := false
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			tx.Rollback()
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
	if req.Currency != nil {
		currency, err := normalizePlanCurrency(*req.Currency)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		plan.Currency = currency
	}
	if req.PlanGroup != nil {
		planGroup, err := NormalizePlanGroupKey(*req.PlanGroup, false)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if _, err := GetPlanGroupByKey(tx, planGroup); err != nil {
			tx.Rollback()
			return nil, err
		}
		planGroupChanged = plan.PlanGroup != planGroup
		plan.PlanGroup = planGroup
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

	if err := tx.Model(&models.Plan{}).
		Where("id = ?", plan.ID).
		Updates(map[string]interface{}{
			"name":        plan.Name,
			"description": plan.Description,
			"days":        plan.Days,
			"price":       plan.Price,
			"currency":    plan.Currency,
			"planGroup":   plan.PlanGroup,
			"isActive":    plan.IsActive,
			"sortOrder":   plan.SortOrder,
		}).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("更新方案失败")
	}

	var expiredSessionIDs []string
	if planGroupChanged {
		sessionIDs, err := PendingStripeSessionIDsForPlan(tx, plan.ID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		expiredSessionIDs = sessionIDs
		expiredCount, err := ExpirePendingPaymentsForPlan(tx, plan.ID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		log.Printf("[Payment] 套餐分组变更，已收口待支付订单: planID=%s planGroup=%s expiredCount=%d", plan.ID, plan.PlanGroup, expiredCount)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("更新方案失败")
	}
	if len(expiredSessionIDs) > 0 {
		s.expireStripeCheckoutSessions(expiredSessionIDs)
	}
	return s.getPlanByID(plan.ID)
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
	if strings.TrimSpace(req.PlanGroup) != "" {
		planGroup, err := NormalizePlanGroupKey(req.PlanGroup, false)
		if err != nil {
			return nil, err
		}
		query = query.Where("\"planGroup\" = ?", planGroup)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("获取方案列表失败")
	}

	var plans []PlanView
	offset := (page - 1) * pageSize
	if err := buildPlansWithGroupNameSelect(query).
		Order(`plans."sortOrder" ASC, plans."createdAt" DESC`).
		Offset(offset).
		Limit(pageSize).
		Find(&plans).Error; err != nil {
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

func (s *PaymentService) GetPlansForUser(userID string) ([]PlanView, error) {
	var user models.User
	if err := db.DB.Select("id", "planGroup").Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, errors.New("获取用户信息失败")
	}

	planGroup, err := ResolveEffectivePlanGroupKey(nil, user.PlanGroup)
	if err != nil {
		return nil, err
	}

	var plans []PlanView
	if err := buildPlansWithGroupNameSelect(db.DB.Model(&models.Plan{})).
		Where(`plans."isActive" = ? AND plans."planGroup" = ?`, true, planGroup).
		Order(`plans."sortOrder" ASC, plans."createdAt" DESC`).
		Find(&plans).Error; err != nil {
		return nil, errors.New("获取方案列表失败")
	}
	return plans, nil
}

func (s *PaymentService) getPlanByID(id string) (*PlanView, error) {
	var plan PlanView
	if err := buildPlansWithGroupNameSelect(db.DB.Model(&models.Plan{})).
		Where("plans.id = ?", id).
		First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlanNotFound
		}
		return nil, errors.New("获取方案失败")
	}
	return &plan, nil
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func expirePendingPaymentsByScope(tx *gorm.DB, userID, planID string) (int64, error) {
	if tx == nil {
		tx = db.DB
	}
	userID = strings.TrimSpace(userID)
	planID = strings.TrimSpace(planID)
	if userID == "" && planID == "" {
		// 误用全表收口会一次性把所有 pending 支付吹掉；显式 panic 让 CI / 单测立刻发现，
		// 不通过返回 0 给调用方一个"好像啥都没发生"的假象。
		panic("expirePendingPaymentsByScope requires userId or planId; misuse will wipe all pending payments")
	}

	query := tx.Model(&models.Payment{}).Where("status = ?", models.PaymentPending)
	if userID != "" {
		query = query.Where("\"userId\" = ?", userID)
	}
	if planID != "" {
		query = query.Where("\"planId\" = ?", planID)
	}

	result := query.Update("status", models.PaymentExpired)
	if result.Error != nil {
		return 0, errors.New("更新支付记录失败")
	}
	return result.RowsAffected, nil
}

func ExpirePendingPaymentsForUser(tx *gorm.DB, userID string) (int64, error) {
	return expirePendingPaymentsByScope(tx, userID, "")
}

func ExpirePendingPaymentsForPlan(tx *gorm.DB, planID string) (int64, error) {
	return expirePendingPaymentsByScope(tx, "", planID)
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
	sessionIDs := make([]string, 0)
	if err := query.Session(&gorm.Session{}).Where(`"stripeSessionId" <> ''`).Pluck(`"stripeSessionId"`, &sessionIDs).Error; err != nil {
		return errors.New("查询待失效 Stripe 会话失败")
	}

	result := query.Update("status", models.PaymentExpired)
	if result.Error != nil {
		return errors.New("更新支付记录失败")
	}
	if result.RowsAffected > 0 {
		log.Printf("[Payment] 已标记过期订单: userID=%s planID=%s count=%d", strings.TrimSpace(userID), strings.TrimSpace(planID), result.RowsAffected)
		s.expireStripeCheckoutSessions(sessionIDs)
	}
	return nil
}

func (s *PaymentService) expireStripeCheckoutSessions(sessionIDs []string) {
	if len(sessionIDs) == 0 {
		return
	}
	configService := configpkg.NewConfigService()
	secret := strings.TrimSpace(configService.GetString("STRIPE_SECRET_KEY"))
	if secret == "" {
		log.Printf("[Payment] Stripe Secret 未配置，跳过失效旧 checkout sessions: count=%d", len(sessionIDs))
		return
	}

	seen := make(map[string]struct{}, len(sessionIDs))
	for _, rawID := range sessionIDs {
		sessionID := strings.TrimSpace(rawID)
		if sessionID == "" {
			continue
		}
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		if err := s.expireStripeCheckoutSession(secret, sessionID); err != nil {
			log.Printf("[Payment] 失效旧 checkout session 失败: sessionID=%s err=%v", sessionID, err)
			continue
		}
		log.Printf("[Payment] 已失效旧 checkout session: sessionID=%s", sessionID)
	}
}

func ExpireStripeCheckoutSessions(sessionIDs []string) {
	NewPaymentService().expireStripeCheckoutSessions(sessionIDs)
}

func (s *PaymentService) expireStripeCheckoutSession(secret, sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, "https://api.stripe.com/v1/checkout/sessions/"+url.PathEscape(strings.TrimSpace(sessionID))+"/expire", strings.NewReader(""))
	if err != nil {
		return errors.New("失效支付会话失败")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return upstream.SafeUpstreamError(err, "stripe")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return upstream.SafeUpstreamError(err, "stripe")
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	var apiErr stripeCheckoutSessionResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error != nil && strings.TrimSpace(apiErr.Error.Message) != "" {
		// 旧会话已失效 / 已完成时 Stripe 会返回 4xx；这种情况说明旧入口已不可继续付款，可视为收口成功。
		if strings.Contains(strings.ToLower(apiErr.Error.Message), "already expired") ||
			strings.Contains(strings.ToLower(apiErr.Error.Message), "completed") {
			return nil
		}
		return upstream.SafeUpstreamHTTPError("stripe", resp.StatusCode)
	}
	return upstream.SafeUpstreamHTTPError("stripe", resp.StatusCode)
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

	var user models.User
	if err := db.DB.Select("id", "planGroup").Where("id = ?", userID).First(&user).Error; err != nil {
		log.Printf("[Payment] 查询用户套餐分组失败: userID=%s planID=%s err=%v", userID, strings.TrimSpace(req.PlanID), err)
		return nil, errors.New("获取用户信息失败")
	}
	planGroup, err := ResolveEffectivePlanGroupKey(nil, user.PlanGroup)
	if err != nil {
		rawPlanGroup := ""
		if user.PlanGroup != nil {
			rawPlanGroup = strings.TrimSpace(*user.PlanGroup)
		}
		log.Printf("[Payment] 用户套餐分组无效: userID=%s rawPlanGroup=%s err=%v", userID, rawPlanGroup, err)
		return nil, err
	}

	var plan models.Plan
	if err := db.DB.Where("id = ? AND \"isActive\" = ? AND \"planGroup\" = ?", req.PlanID, true, planGroup).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[Payment] 方案不存在、未启用或不属于用户分组: userID=%s planID=%s planGroup=%s", userID, strings.TrimSpace(req.PlanID), planGroup)
			return nil, ErrPlanNotFound
		}
		log.Printf("[Payment] 查询方案失败: userID=%s planID=%s planGroup=%s err=%v", userID, strings.TrimSpace(req.PlanID), planGroup, err)
		return nil, errors.New("获取方案失败")
	}

	paymentMethods, err := configService.GetStripeAllowedPaymentMethods()
	if err != nil {
		log.Printf("[Payment] 读取支付方式限制失败: userID=%s planID=%s err=%v", userID, plan.ID, err)
		return nil, err
	}

	now := time.Now().UTC()
	if err := s.expirePendingPayments(userID, plan.ID, now); err != nil {
		return nil, err
	}

	payment, err := s.reservePendingPayment(userID, &plan, now)
	if err != nil {
		log.Printf("[Payment] 占位 pending 订单失败: userID=%s planID=%s err=%v", userID, plan.ID, err)
		return nil, err
	}

	// 已经回填了 checkoutUrl 的 pending：第二个 tab 直接复用，不再调 Stripe。
	if strings.TrimSpace(payment.CheckoutURL) != "" {
		log.Printf("[Payment] 复用待支付订单: userID=%s planID=%s paymentID=%s sessionID=%s",
			userID, plan.ID, payment.ID, payment.StripeSessionID)
		return &CreateCheckoutResponse{URL: payment.CheckoutURL}, nil
	}

	// 否则调 Stripe；并发的两个请求会复用同一 paymentId，Idempotency-Key 保证 Stripe 端只产生一条 Session。
	sess, err := s.createStripeCheckoutSession(stripeSecret, successURL, cancelURL, userID, &plan, paymentMethods, payment.ID)
	if err != nil {
		log.Printf("[Payment] Stripe 会话创建失败: userID=%s planID=%s paymentID=%s err=%v", userID, plan.ID, payment.ID, err)
		return nil, err
	}

	if err := db.DB.Model(&models.Payment{}).
		Where("id = ?", payment.ID).
		Updates(map[string]interface{}{
			"stripeSessionId": sess.ID,
			"checkoutUrl":     strings.TrimSpace(sess.URL),
		}).Error; err != nil {
		log.Printf("[Payment] 回填 Stripe sessionId 失败: userID=%s planID=%s paymentID=%s sessionID=%s err=%v",
			userID, plan.ID, payment.ID, sess.ID, err)
		return nil, errors.New("创建支付记录失败")
	}

	expiresAtStr := ""
	if payment.ExpiresAt != nil {
		expiresAtStr = payment.ExpiresAt.UTC().Format(time.RFC3339)
	}
	log.Printf("[Payment] 创建支付会话成功: userID=%s planID=%s paymentID=%s sessionID=%s amount=%d currency=%s days=%d expiresAt=%s methods=%v",
		userID, plan.ID, payment.ID, sess.ID, plan.Price, plan.Currency, plan.Days, expiresAtStr, paymentMethods)

	return &CreateCheckoutResponse{URL: sess.URL}, nil
}

// reservePendingPayment 在事务里 INSERT 一条 pending 占位行（status='pending', stripeSessionId=”）。
//
// 同 (userId, planId) 唯一冲突时，回查现有 pending 行返回。partial unique
// uq_payments_pending_user_plan WHERE status='pending' 保证全表只能存在一条 pending。
func (s *PaymentService) reservePendingPayment(userID string, plan *models.Plan, now time.Time) (*models.Payment, error) {
	expiresAt := now.Add(pendingCheckoutTTL)
	payment := models.Payment{
		UserID:      userID,
		PlanID:      plan.ID,
		Amount:      plan.Price,
		Currency:    plan.Currency,
		Days:        plan.Days,
		Status:      models.PaymentPending,
		ExpiresAt:   &expiresAt,
		CheckoutURL: "",
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("创建支付记录失败")
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// 锁住 user 与 plan 行避免并发履约穿越（fulfillPayment 也按同样顺序加锁）。
	var lockedUser models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userID).First(&lockedUser).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("获取用户信息失败")
	}
	var lockedPlan models.Plan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", plan.ID).First(&lockedPlan).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("获取方案失败")
	}

	result := tx.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "userId"}, {Name: "planId"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Eq{Column: "status", Value: string(models.PaymentPending)}}},
		DoNothing:   true,
	}).Create(&payment)
	if result.Error != nil {
		tx.Rollback()
		return nil, errors.New("创建支付记录失败")
	}

	if result.RowsAffected == 0 {
		// 冲突：回查已存在的 pending 行复用。
		var existing models.Payment
		if err := tx.Where(`"userId" = ? AND "planId" = ? AND status = ?`, userID, plan.ID, models.PaymentPending).
			Order(`"createdAt" ASC`).
			First(&existing).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("创建支付记录失败")
		}
		if err := tx.Commit().Error; err != nil {
			return nil, errors.New("创建支付记录失败")
		}
		return &existing, nil
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("创建支付记录失败")
	}
	return &payment, nil
}

func (s *PaymentService) createStripeCheckoutSession(secret, successURL, cancelURL, userID string, plan *models.Plan, paymentMethods []string, paymentID string) (*stripeCheckoutSessionResponse, error) {
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
	form.Set("metadata[payment_id]", paymentID)
	for _, method := range paymentMethods {
		form.Add("payment_method_types[]", method)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.New("创建支付会话失败")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Idempotency-Key 绑定在 paymentId 上：并发占位拿到同一 paymentId 的两个请求，Stripe 会返回同一 Session。
	if strings.TrimSpace(paymentID) != "" {
		req.Header.Set("Idempotency-Key", "checkout:"+paymentID)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "stripe")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, upstream.SafeUpstreamError(err, "stripe")
	}

	var result stripeCheckoutSessionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, upstream.SafeUpstreamError(err, "stripe")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, upstream.SafeUpstreamHTTPError("stripe", resp.StatusCode)
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
		return fmt.Errorf("%w: %v", ErrStripeWebhookInvalid, err)
	}

	var event stripeWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("%w: %v", ErrStripeWebhookParseFailed, err)
	}

	eventID := strings.TrimSpace(event.ID)
	if eventID == "" {
		log.Printf("[Payment] Stripe webhook 缺少 event.id，忽略: type=%s", event.Type)
		return nil
	}

	log.Printf("[Payment] 收到 Stripe webhook: eventId=%s type=%s sessionID=%s paymentStatus=%s paymentIntent=%s",
		eventID, event.Type,
		strings.TrimSpace(event.Data.Object.ID),
		strings.TrimSpace(event.Data.Object.PaymentStatus),
		strings.TrimSpace(event.Data.Object.PaymentIntent),
	)

	// event.id 级别去重 + 失败重试：
	//   - 首次写入：INSERT ON CONFLICT DO NOTHING，进入业务分发
	//   - 命中冲突：回查 status
	//       processed / skipped → 真正幂等，直接 200 不再分发
	//       received / failed   → 上次未完成或失败，允许重新分发；fulfillPayment / markPaymentFailed
	//                              内部自带状态机幂等检查，并发或重复分发不会破坏数据
	//   这样 Stripe 的自动重试在初次失败 / 进程崩溃后仍能驱动履约。
	dedup := models.StripeWebhookEvent{
		EventID:    eventID,
		EventType:  event.Type,
		Livemode:   event.Livemode,
		ReceivedAt: time.Now().UTC(),
		Status:     models.StripeWebhookEventReceived,
	}
	insertResult := db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "eventId"}},
		DoNothing: true,
	}).Create(&dedup)
	if insertResult.Error != nil {
		log.Printf("[Payment] webhook 去重落库失败: eventId=%s err=%v", eventID, insertResult.Error)
		return ErrPaymentFailed
	}
	if insertResult.RowsAffected == 0 {
		var existing models.StripeWebhookEvent
		if err := db.DB.Where(`"eventId" = ?`, eventID).First(&existing).Error; err != nil {
			log.Printf("[Payment] webhook 回查去重记录失败: eventId=%s err=%v", eventID, err)
			return ErrPaymentFailed
		}
		if !shouldRedispatchWebhook(existing.Status) {
			log.Printf("[Payment] webhook 已处理过的事件，幂等返回: eventId=%s status=%s", eventID, existing.Status)
			return nil
		}
		// received / failed：允许重新分发，对应 Stripe 自动重试 / 进程崩溃恢复 / 上次失败的幂等重放。
		log.Printf("[Payment] webhook 命中未完成事件，允许重新分发: eventId=%s status=%s", eventID, existing.Status)
		// 把 receivedAt 刷新为本次重投时间，errorMessage 等终态由分发完成后的 UPDATE 覆盖。
		if err := db.DB.Model(&models.StripeWebhookEvent{}).
			Where(`"eventId" = ?`, eventID).
			Updates(map[string]interface{}{
				"status":     models.StripeWebhookEventReceived,
				"receivedAt": time.Now().UTC(),
			}).Error; err != nil {
			log.Printf("[Payment] webhook 重置 received 状态失败: eventId=%s err=%v", eventID, err)
		}
	}

	eventCreated := time.Time{}
	if event.Created > 0 {
		eventCreated = time.Unix(event.Created, 0).UTC()
	}

	dispatchErr := s.dispatchWebhook(&event, eventCreated)

	finalStatus := models.StripeWebhookEventProcessed
	var errMessage *string
	switch event.Type {
	case "checkout.session.completed",
		"checkout.session.async_payment_succeeded",
		"checkout.session.async_payment_failed",
		"checkout.session.expired":
		// 上述事件类型走 processed
	default:
		finalStatus = models.StripeWebhookEventSkipped
	}
	if dispatchErr != nil {
		finalStatus = models.StripeWebhookEventFailed
		msg := truncateString(dispatchErr.Error(), 500)
		errMessage = &msg
	}

	processedAt := time.Now().UTC()
	if err := db.DB.Model(&models.StripeWebhookEvent{}).
		Where(`"eventId" = ?`, eventID).
		Updates(map[string]interface{}{
			"processedAt":  processedAt,
			"status":       finalStatus,
			"errorMessage": errMessage,
		}).Error; err != nil {
		log.Printf("[Payment] webhook 状态回写失败: eventId=%s status=%s err=%v", eventID, finalStatus, err)
	}

	return dispatchErr
}

func (s *PaymentService) dispatchWebhook(event *stripeWebhookEvent, eventCreated time.Time) error {
	switch event.Type {
	case "checkout.session.completed":
		// 异步支付方式下 completed 可能先于真实到账，必须确认 paid 才履约。
		if event.Data.Object.PaymentStatus != "paid" {
			log.Printf("[Payment] 忽略未支付完成事件: type=%s sessionID=%s paymentStatus=%s",
				event.Type, strings.TrimSpace(event.Data.Object.ID), strings.TrimSpace(event.Data.Object.PaymentStatus))
			return nil
		}
		return s.fulfillPayment(event.Data.Object.ID, event.Data.Object.PaymentIntent, eventCreated, event.Data.Object.Metadata)
	case "checkout.session.async_payment_succeeded":
		return s.fulfillPayment(event.Data.Object.ID, event.Data.Object.PaymentIntent, eventCreated, event.Data.Object.Metadata)
	case "checkout.session.async_payment_failed":
		return s.markPaymentFailed(event.Data.Object.ID, eventCreated)
	case "checkout.session.expired":
		return s.MarkPaymentExpired(event.Data.Object.ID)
	default:
		return nil
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// shouldRedispatchWebhook 决定命中已有 stripe_webhook_events 行时是否允许重新分发。
//
// 真正幂等结束态（processed / skipped）直接返回 200，不重复履约；
// received / failed 表示上次未完成（崩溃中断 / DB 错误 / 业务返回 5xx），允许 Stripe 重试驱动履约。
//
// 拆出独立函数便于单测覆盖去重状态机，避免回退到"首次失败永不重试"的资金硬故障。
func shouldRedispatchWebhook(status models.StripeWebhookEventStatus) bool {
	switch status {
	case models.StripeWebhookEventProcessed, models.StripeWebhookEventSkipped:
		return false
	default:
		return true
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

func formatNotifyTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.UTC().Format(time.RFC3339)
	return &formatted
}

func planGroupsMatchForFulfillment(userPlanGroup *string, planPlanGroup string) (bool, error) {
	normalizedUserPlanGroup, err := ResolveEffectivePlanGroupKey(nil, userPlanGroup)
	if err != nil {
		return false, err
	}
	normalizedPlanPlanGroup, err := NormalizePlanGroupKey(planPlanGroup, false)
	if err != nil {
		return false, err
	}
	return normalizedUserPlanGroup == normalizedPlanPlanGroup, nil
}

func (s *PaymentService) fulfillPayment(sessionID, paymentIntentID string, eventCreated time.Time, metadata map[string]string) error {
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
	// 乱序保护：若该 webhook 早于 payment.updatedAt，说明本地已被更新事件之后的状态收口，跳过。
	if !eventCreated.IsZero() && eventCreated.Before(payment.UpdatedAt) {
		tx.Rollback()
		log.Printf("[Payment] 忽略乱序支付成功事件: paymentID=%s sessionID=%s eventCreated=%s paymentUpdatedAt=%s",
			payment.ID, payment.StripeSessionID, eventCreated.Format(time.RFC3339), payment.UpdatedAt.Format(time.RFC3339))
		return nil
	}

	var user models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", payment.UserID).First(&user).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约查询用户失败: paymentID=%s userID=%s err=%v", payment.ID, payment.UserID, err)
		return ErrPaymentFailed
	}

	var plan models.Plan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "name", "planGroup").
		Where("id = ?", payment.PlanID).
		First(&plan).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约查询套餐失败: paymentID=%s planID=%s err=%v", payment.ID, payment.PlanID, err)
		return ErrPaymentFailed
	}
	planName := plan.Name

	planGroupMatched, err := planGroupsMatchForFulfillment(user.PlanGroup, plan.PlanGroup)
	if err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约校验套餐分组失败: paymentID=%s userID=%s planID=%s err=%v", payment.ID, payment.UserID, payment.PlanID, err)
		return ErrPaymentFailed
	}
	if !planGroupMatched {
		payment.Status = models.PaymentExpired
		if strings.TrimSpace(paymentIntentID) != "" {
			payment.StripePaymentIntentID = paymentIntentID
		}
		if err := tx.Model(&models.Payment{}).
			Where("id = ?", payment.ID).
			Updates(map[string]interface{}{
				"status":                payment.Status,
				"stripePaymentIntentId": payment.StripePaymentIntentID,
			}).Error; err != nil {
			tx.Rollback()
			log.Printf("[Payment] 支付履约拒绝后保存订单失败: paymentID=%s sessionID=%s err=%v", payment.ID, payment.StripeSessionID, err)
			return ErrPaymentFailed
		}
		if err := tx.Commit().Error; err != nil {
			log.Printf("[Payment] 支付履约拒绝后提交事务失败: paymentID=%s sessionID=%s err=%v", payment.ID, payment.StripeSessionID, err)
			return ErrPaymentFailed
		}
		rawUserPlanGroup := ""
		if user.PlanGroup != nil {
			rawUserPlanGroup = strings.TrimSpace(*user.PlanGroup)
		}
		log.Printf("[Payment] 支付履约拒绝：套餐分组已变更: paymentID=%s userID=%s planID=%s sessionID=%s userPlanGroup=%s planPlanGroup=%s",
			payment.ID, payment.UserID, payment.PlanID, payment.StripeSessionID, rawUserPlanGroup, strings.TrimSpace(plan.PlanGroup))
		return nil
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
	// Emby 解封移到事务外（commit 后异步），避免事务中持有 Emby 网络 I/O，并解耦事务边界。
	needsEmbyUnban := user.EmbyDisabled && user.IsActive
	if needsEmbyUnban {
		user.EmbyDisabled = false
	}

	if err := tx.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]interface{}{
			"expiresAt":    user.ExpiresAt,
			"embyDisabled": user.EmbyDisabled,
		}).Error; err != nil {
		tx.Rollback()
		log.Printf("[Payment] 支付履约保存用户失败: paymentID=%s userID=%s err=%v", payment.ID, payment.UserID, err)
		return ErrPaymentFailed
	}

	payment.Status = models.PaymentCompleted
	if strings.TrimSpace(paymentIntentID) != "" {
		payment.StripePaymentIntentID = paymentIntentID
	}
	if err := tx.Model(&models.Payment{}).
		Where("id = ?", payment.ID).
		Updates(map[string]interface{}{
			"status":                payment.Status,
			"stripePaymentIntentId": payment.StripePaymentIntentID,
		}).Error; err != nil {
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

	if needsEmbyUnban {
		paymentID := payment.ID
		userIDCopy := user.ID
		embyID := user.EmbyID
		async.SafeGo("payment.unban", func() {
			if err := s.compensation.EnsureUnbanned(context.Background(), models.FailedEmbyAsyncOp{
				Origin:      models.FailedEmbyOriginPaymentUnban,
				OriginRefID: paymentID,
				EmbyUserID:  embyID,
			}); err != nil {
				log.Printf("[Payment] 解封 + 入队全部失败: paymentID=%s userID=%s embyID=%s err=%v",
					paymentID, userIDCopy, embyID, err)
			}
		})
	}

	paymentPayload := notifierint.PaymentSuccessNotification{
		PaymentID:    payment.ID,
		UserID:       user.ID,
		UserName:     user.Username,
		PlanID:       payment.PlanID,
		PlanName:     planName,
		Amount:       payment.Amount,
		Currency:     payment.Currency,
		Days:         payment.Days,
		OldExpiresAt: formatNotifyTime(oldExpiry),
		NewExpiresAt: newExpiry.Format(time.RFC3339),
	}
	async.SafeGo("payment.notifySuccess", func() {
		if s.notifier == nil || !s.notifier.IsConfigured() {
			return
		}
		s.notifier.NotifyPaymentSuccess(paymentPayload)
	})

	return nil
}

func (s *PaymentService) markPaymentFailed(sessionID string, eventCreated time.Time) error {
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
	if !eventCreated.IsZero() && eventCreated.Before(payment.UpdatedAt) {
		tx.Rollback()
		log.Printf("[Payment] 忽略乱序支付失败事件: paymentID=%s sessionID=%s eventCreated=%s paymentUpdatedAt=%s",
			payment.ID, payment.StripeSessionID, eventCreated.Format(time.RFC3339), payment.UpdatedAt.Format(time.RFC3339))
		return nil
	}

	payment.Status = models.PaymentFailed
	if err := tx.Model(&models.Payment{}).
		Where("id = ?", payment.ID).
		Update("status", payment.Status).Error; err != nil {
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

// MarkPaymentExpired 处理 checkout.session.expired：把本地仍在 pending 的订单收口为 expired。
//
// RowsAffected=0 视为已收口（noop），可能是被 expirePendingPayments 提前收口或被并发 webhook 抢先处理。
func (s *PaymentService) MarkPaymentExpired(sessionID string) error {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return ErrPaymentFailed
	}

	result := db.DB.Model(&models.Payment{}).
		Where(`"stripeSessionId" = ? AND status = ?`, sid, models.PaymentPending).
		Updates(map[string]interface{}{"status": models.PaymentExpired})
	if result.Error != nil {
		log.Printf("[Payment] 标记 checkout 过期失败: sessionID=%s err=%v", sid, result.Error)
		return ErrPaymentFailed
	}
	if result.RowsAffected == 0 {
		log.Printf("[Payment] 收到 checkout.session.expired 但本地无 pending 订单可收口: sessionID=%s", sid)
		return nil
	}
	log.Printf("[Payment] 已按 webhook 标记支付过期: sessionID=%s rowsAffected=%d", sid, result.RowsAffected)
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
		if err := s.expirePendingPayments(strings.TrimSpace(req.UserID), strings.TrimSpace(req.PlanID), time.Now().UTC()); err != nil {
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
	if strings.TrimSpace(req.PlanID) != "" {
		query = query.Where("\"planId\" = ?", strings.TrimSpace(req.PlanID))
	}
	if status := normalizePaymentStatusFilter(req.Status); status != "" {
		query = query.Where("status = ?", status)
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

func normalizePaymentStatusFilter(raw string) models.PaymentStatus {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(models.PaymentPending):
		return models.PaymentPending
	case string(models.PaymentCompleted):
		return models.PaymentCompleted
	case string(models.PaymentExpired):
		return models.PaymentExpired
	case string(models.PaymentFailed):
		return models.PaymentFailed
	default:
		return ""
	}
}
