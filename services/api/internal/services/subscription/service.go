package subscription

import (
	"errors"
	"fmt"

	"github.com/konghang/ember/backend/internal/db"
	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	moviepilot *moviepilotint.MoviePilotClient
	notifier   *notifierint.BotNotifier
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService() *SubscriptionService {
	return &SubscriptionService{
		moviepilot: moviepilotint.NewMoviePilotClient(),
		notifier:   notifierint.NewBotNotifier(),
	}
}

// CreateSubscriptionRequest 创建订阅请求
type CreateSubscriptionRequest struct {
	Type       models.MediaType `json:"type" binding:"required"`
	Name       string           `json:"name" binding:"required"`
	TmdbID     string           `json:"tmdbId" binding:"required"`
	PosterPath *string          `json:"posterPath"`
	Note       *string          `json:"note"`
}

// CreateSubscription 用户创建订阅
func (s *SubscriptionService) CreateSubscription(userID string, req CreateSubscriptionRequest) error {
	// 验证必填字段
	if req.Name == "" || req.TmdbID == "" {
		return errors.New("影视名称和 TMDB ID 为必填项")
	}

	// 全局去重：同一 type + tmdbId 只允许提交一次
	var count int64
	if err := db.DB.Model(&models.Subscription{}).
		Where("type = ? AND \"tmdbId\" = ?", req.Type, req.TmdbID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("创建订阅失败: %w", err)
	}
	if count > 0 {
		return ErrSubscriptionDuplicated
	}

	// 创建订阅记录
	subscription := &models.Subscription{
		UserID:     userID,
		Type:       req.Type,
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		PosterPath: req.PosterPath,
		Note:       req.Note,
		Status:     models.SubscriptionPending,
	}

	if err := db.DB.Create(subscription).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrSubscriptionDuplicated
		}
		return fmt.Errorf("创建订阅失败: %w", err)
	}

	// 通知 Bot（异步 fire-and-forget，失败不影响订阅创建）
	go func(subscriptionID, userID string, req CreateSubscriptionRequest) {
		if s.notifier == nil || !s.notifier.IsConfigured() {
			return
		}

		var username string
		var user models.User
		if err := db.DB.Select("username").Where("id = ?", userID).First(&user).Error; err == nil {
			username = user.Username
		}

		s.notifier.NotifyNewSubscription(notifierint.SubscriptionNotification{
			ID:         subscriptionID,
			UserName:   username,
			Type:       string(req.Type),
			Name:       req.Name,
			TmdbID:     req.TmdbID,
			PosterPath: req.PosterPath,
			Note:       req.Note,
		})
	}(subscription.ID, userID, req)

	return nil
}

// GetUserSubscriptions 获取用户的订阅列表
func (s *SubscriptionService) GetUserSubscriptions(userID string) ([]models.Subscription, error) {
	var subscriptions []models.Subscription

	err := db.DB.
		Where("\"userId\" = ?", userID).
		Order("\"createdAt\" DESC").
		Find(&subscriptions).Error

	if err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}

	return subscriptions, nil
}

// GetUserSubscriptionsPaginated 用户订阅分页查询
func (s *SubscriptionService) GetUserSubscriptionsPaginated(userID string, status *models.SubscriptionStatus, page, pageSize int) (*GetAllSubscriptionsResponse, error) {
	offset := (page - 1) * pageSize

	query := db.DB.Model(&models.Subscription{}).Where("\"userId\" = ?", userID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询订阅总数失败: %w", err)
	}

	var subscriptions []models.Subscription
	if err := query.
		Order("\"createdAt\" DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}

	result := make([]SubscriptionWithUser, len(subscriptions))
	for i, sub := range subscriptions {
		result[i] = SubscriptionWithUser{Subscription: sub}
	}

	return &GetAllSubscriptionsResponse{
		Data:  result,
		Total: total,
	}, nil
}

// DeleteSubscription 删除订阅（仅允许删除 PENDING 状态）
func (s *SubscriptionService) DeleteSubscription(subscriptionID, userID string) error {
	// 查询订阅
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}

	// 验证所有权
	if subscription.UserID != userID {
		return errors.New("无权删除此订阅")
	}

	// 验证状态（只能删除 PENDING）
	if subscription.Status != models.SubscriptionPending {
		return errors.New("只能删除待审核的订阅")
	}

	// 删除订阅
	if err := db.DB.Delete(&subscription).Error; err != nil {
		return fmt.Errorf("删除订阅失败: %w", err)
	}

	return nil
}

// DeleteSubscriptionAsAdmin 管理员删除订阅（不限制状态和所有权）
func (s *SubscriptionService) DeleteSubscriptionAsAdmin(subscriptionID string) error {
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}

	if err := db.DB.Delete(&subscription).Error; err != nil {
		return fmt.Errorf("删除订阅失败: %w", err)
	}

	return nil
}

// SubscriptionWithUser 订阅记录（包含用户信息）
type SubscriptionWithUser struct {
	models.Subscription
	User struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

// GetAllSubscriptionsResponse 管理员查询订阅列表响应
type GetAllSubscriptionsResponse struct {
	Data  []SubscriptionWithUser `json:"data"`
	Total int64                  `json:"total"`
}

// GetAllSubscriptions 管理员获取所有订阅（分页）
func (s *SubscriptionService) GetAllSubscriptions(status *models.SubscriptionStatus, page, pageSize int) (*GetAllSubscriptionsResponse, error) {
	// 计算偏移量
	offset := (page - 1) * pageSize

	// 构建查询
	query := db.DB.Model(&models.Subscription{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 查询总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询订阅总数失败: %w", err)
	}

	// 查询订阅列表
	var subscriptions []models.Subscription
	err := query.
		Order("\"createdAt\" DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&subscriptions).Error

	if err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}

	// 收集所有 UserID
	userIDs := make([]string, len(subscriptions))
	for i, sub := range subscriptions {
		userIDs[i] = sub.UserID
	}

	// 批量查询用户信息
	var users []models.User
	if len(userIDs) > 0 {
		if err := db.DB.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, fmt.Errorf("查询用户信息失败: %w", err)
		}
	}

	// 构建 UserID → User 映射
	userMap := make(map[string]*models.User)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	// 构建响应（包含用户名和邮箱）
	result := make([]SubscriptionWithUser, len(subscriptions))
	for i, sub := range subscriptions {
		result[i].Subscription = sub
		if user, ok := userMap[sub.UserID]; ok {
			result[i].User.Username = user.Username
			result[i].User.Email = user.Email
		}
	}

	return &GetAllSubscriptionsResponse{
		Data:  result,
		Total: total,
	}, nil
}

// ApproveSubscription 批准订阅
func (s *SubscriptionService) ApproveSubscription(subscriptionID string) error {
	// 查询订阅
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}

	// 验证状态（只能审核 PENDING）
	if subscription.Status != models.SubscriptionPending {
		return errors.New("订阅已被处理")
	}

	// 调用 MoviePilot API（失败时记录错误但不回滚状态）
	var mpError *string
	if s.moviepilot.IsConfigured() {
		// 转换 MediaType 为 MoviePilot 格式
		mpType := "movie"
		if subscription.Type == models.MediaTV {
			mpType = "tv"
		}

		err := s.moviepilot.CreateSubscription(moviepilotint.SubscribeRequest{
			Type:   mpType,
			Name:   subscription.Name,
			TmdbID: subscription.TmdbID,
		})

		if err != nil {
			// MP API 失败时保存错误信息，但仍将订阅状态设为 APPROVED
			errMsg := err.Error()
			mpError = &errMsg
			fmt.Printf("MoviePilot API 调用失败: %v\n", err)
		}
	} else {
		// 未配置 MoviePilot 时跳过 API 调用
		errMsg := "MoviePilot 未配置"
		mpError = &errMsg
	}

	// 更新订阅状态为 APPROVED（无论 MP API 是否成功）
	subscription.Status = models.SubscriptionApproved
	subscription.MpError = mpError

	if err := db.DB.Save(&subscription).Error; err != nil {
		return fmt.Errorf("更新订阅状态失败: %w", err)
	}

	return nil
}

// RejectSubscription 拒绝订阅
func (s *SubscriptionService) RejectSubscription(subscriptionID string) error {
	// 查询订阅
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return ErrSubscriptionNotFound
	}

	// 验证状态（只能审核 PENDING）
	if subscription.Status != models.SubscriptionPending {
		return errors.New("订阅已被处理")
	}

	// 更新状态为 REJECTED
	subscription.Status = models.SubscriptionRejected

	if err := db.DB.Save(&subscription).Error; err != nil {
		return fmt.Errorf("更新订阅状态失败: %w", err)
	}

	return nil
}
