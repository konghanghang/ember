package redemption

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/async"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	"gorm.io/gorm"
)

type RedemptionService struct {
	embyService  *embyint.EmbyService
	compensation *accountpkg.EmbyCompensation
}

// NewRedemptionService 装配兑换码服务，复用注入的 Emby client + 补偿队列。
func NewRedemptionService() *RedemptionService {
	embyService := embyint.GetSharedService()
	return &RedemptionService{
		embyService:  embyService,
		compensation: accountpkg.NewEmbyCompensation(embyService),
	}
}

func (s *RedemptionService) embyClient() *embyint.EmbyService {
	if s.embyService != nil {
		return s.embyService
	}
	return embyint.GetSharedService()
}

func (s *RedemptionService) compensationQueue() *accountpkg.EmbyCompensation {
	if s.compensation != nil {
		return s.compensation
	}
	s.compensation = accountpkg.NewEmbyCompensation(s.embyClient())
	return s.compensation
}

func (s *RedemptionService) RedeemCode(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, ErrRedeemFailed
	}
	defer tx.Rollback()

	var code models.RedemptionCode
	err := tx.Where("code = ?", req.Code).First(&code).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRedemptionCodeNotFound
		}
		return nil, ErrRedeemFailed
	}

	if !code.IsValid() {
		return nil, ErrRedemptionCodeInvalid
	}

	var existingRedemption models.Redemption
	err = tx.Where("\"user_id\" = ? AND code = ?", userID, req.Code).First(&existingRedemption).Error
	if err == nil {
		return nil, ErrRedemptionDuplicate
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRedeemFailed
	}

	var user models.User
	if err := tx.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	now := time.Now().UTC()
	var newExpiry time.Time
	if user.ExpiresAt == nil || user.ExpiresAt.Before(now) {
		newExpiry = now.AddDate(0, 0, code.DefaultDays)
	} else {
		newExpiry = user.ExpiresAt.AddDate(0, 0, code.DefaultDays)
	}

	updates := map[string]interface{}{
		"expires_at": newExpiry,
	}

	// Emby 解封移到事务外（commit 后异步执行）：避免事务持有 Emby 网络 I/O，并在
	// Emby 暂时不可达时进入补偿队列，由 cron 重试。本地 expiresAt 立刻生效，用户感知正常。
	needsEmbyUnban := user.EmbyDisabled && user.IsActive
	if needsEmbyUnban {
		updates["emby_disabled"] = false
	}

	if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return nil, ErrRedeemFailed
	}

	redemption := models.Redemption{
		UserID: userID,
		Code:   req.Code,
		Days:   code.DefaultDays,
	}
	if err := tx.Create(&redemption).Error; err != nil {
		if isRedemptionDuplicateInsert(err) {
			return nil, ErrRedemptionDuplicate
		}
		return nil, ErrRedeemFailed
	}

	result := tx.Model(&models.RedemptionCode{}).
		Where("code = ? AND \"used_count\" < \"max_uses\" AND (\"expires_at\" IS NULL OR \"expires_at\" > ?)", req.Code, now).
		Update("used_count", gorm.Expr("\"used_count\" + 1"))
	if result.Error != nil {
		return nil, ErrRedeemFailed
	}
	if result.RowsAffected == 0 {
		return nil, ErrRedemptionCodeInvalid
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ErrRedeemFailed
	}

	if needsEmbyUnban {
		redemptionID := redemption.ID
		userIDCopy := user.ID
		embyID := strings.TrimSpace(user.EmbyID)
		async.SafeGo("redemption.unban", func() {
			if embyID == "" {
				return
			}
			if err := s.compensationQueue().EnsureUnbanned(context.Background(), models.FailedEmbyAsyncOp{
				Origin:      models.FailedEmbyOriginRedemptionUnban,
				OriginRefID: redemptionID,
				EmbyUserID:  embyID,
			}); err != nil {
				log.Printf("[Redemption] 解封 + 入队全部失败: redemptionID=%s userID=%s embyID=%s err=%v",
					redemptionID, userIDCopy, embyID, err)
			}
		})
	}

	return &RedeemCodeResponse{
		Message:   fmt.Sprintf("兑换成功，有效期已延长 %d 天", code.DefaultDays),
		Days:      code.DefaultDays,
		ExpiresAt: &newExpiry,
	}, nil
}

func isRedemptionDuplicateInsert(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505"
}

func (s *RedemptionService) GetRedemptions(userID string, req *GetRedemptionsRequest) (*GetRedemptionsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	query := db.DB.Model(&models.Redemption{}).Where("\"user_id\" = ?", userID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	var rows []models.Redemption
	offset := (page - 1) * pageSize
	if err := query.Order("\"created_at\" DESC").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	return &GetRedemptionsResponse{
		Data:       rows,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}

func (s *RedemptionService) GetAllRedemptions(req *GetAllRedemptionsRequest) (*GetAllRedemptionsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	base := db.DB.Table("redemptions r").Joins("LEFT JOIN users u ON r.\"user_id\" = u.id")
	if req.UserID != "" {
		base = base.Where("r.\"user_id\" = ?", req.UserID)
	}
	if strings.TrimSpace(req.Username) != "" {
		base = base.Where("u.username ILIKE ?", "%"+strings.TrimSpace(req.Username)+"%")
	}
	if strings.TrimSpace(req.Code) != "" {
		base = base.Where("r.code = ?", strings.TrimSpace(req.Code))
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	var rows []RedemptionWithUser
	offset := (page - 1) * pageSize
	if err := base.
		Select("r.id, r.\"user_id\", r.code, r.days, r.\"created_at\", COALESCE(u.username, '') AS username").
		Order("r.\"created_at\" DESC").
		Offset(offset).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	return &GetAllRedemptionsResponse{
		Data:       rows,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}
