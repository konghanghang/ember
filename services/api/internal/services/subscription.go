package services

import (
	"errors"
	"fmt"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	moviepilot *MoviePilotClient
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService() *SubscriptionService {
	return &SubscriptionService{
		moviepilot: NewMoviePilotClient(),
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
		return fmt.Errorf("创建订阅失败: %w", err)
	}

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

// DeleteSubscription 删除订阅（仅允许删除 PENDING 状态）
func (s *SubscriptionService) DeleteSubscription(subscriptionID, userID string) error {
	// 查询订阅
	var subscription models.Subscription
	if err := db.DB.Where("id = ?", subscriptionID).First(&subscription).Error; err != nil {
		return errors.New("订阅不存在")
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

	// 查询订阅列表（包含用户信息）
	var subscriptions []models.Subscription
	err := query.
		Preload("User").
		Order("\"createdAt\" DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&subscriptions).Error

	if err != nil {
		return nil, fmt.Errorf("查询订阅列表失败: %w", err)
	}

	// 构建响应（包含用户名和邮箱）
	result := make([]SubscriptionWithUser, len(subscriptions))
	for i, sub := range subscriptions {
		result[i].Subscription = sub
		if sub.User != nil {
			result[i].User.Username = sub.User.Username
			result[i].User.Email = sub.User.Email
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
		return errors.New("订阅不存在")
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

		err := s.moviepilot.CreateSubscription(SubscribeRequest{
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
		return errors.New("订阅不存在")
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
