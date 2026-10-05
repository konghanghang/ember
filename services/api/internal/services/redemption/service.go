package redemption

import (
	"errors"
	"fmt"
	configpkg "github.com/konghang/ember/backend/internal/config"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
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
	policypkg "github.com/konghang/ember/backend/internal/services/policy"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RedemptionService struct {
	embyService  *embyint.EmbyService
	compensation *accountpkg.EmbyCompensation

	redeemCodeStore func(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error)
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

// RedeemCode 兑换用户提交的兑换码，并在进入存储层前统一收口兑换码空白字符。
func (s *RedemptionService) RedeemCode(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
	normalizedReq := *req
	normalizedReq.Code = strings.TrimSpace(normalizedReq.Code)
	if s.redeemCodeStore != nil {
		return s.redeemCodeStore(userID, &normalizedReq)
	}
	return s.redeemCodeWithDB(userID, &normalizedReq)
}

// redeemCodeWithDB 在事务内按兑换码类型授予分组权益，并保存不可变的兑换有效期快照。
func (s *RedemptionService) redeemCodeWithDB(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
	location := configpkg.LoadConfiguredTimezone()
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

	user, err := lockUserForRedemptionRenewal(tx, userID)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	benefit, err := CodeBenefit(&code)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := entitlementpkg.GrantLocked(tx, user, []entitlementpkg.Benefit{benefit}, "redemption:"+user.ID+":"+code.ID, "system:redemption", now, location); err != nil {
		if errors.Is(err, entitlementpkg.ErrAlreadyOwned) || errors.Is(err, entitlementpkg.ErrGroupsNotReady) {
			return nil, err
		}
		log.Printf("[Redemption] 权益发放失败 userId=%s codeId=%s errorType=%T", user.ID, code.ID, err)
		return nil, ErrRedeemFailed
	}
	needsPolicySync := strings.TrimSpace(user.EmbyID) != ""
	holdings, err := entitlementpkg.Load(tx, user.ID)
	if err != nil {
		return nil, err
	}
	var newExpiry *time.Time
	for _, holding := range holdings {
		if holding.PlanGroup == code.RegistrationPlanGroup {
			newExpiry = holding.ExpiresAt
		}
	}

	redemption := models.Redemption{
		ValidityType: benefit.ValidityType,
		UserID:       userID,
		Code:         req.Code,
		Days:         code.DefaultDays,
	}
	if err := tx.Create(&redemption).Error; err != nil {
		if isRedemptionDuplicateInsert(err) {
			return nil, ErrRedemptionDuplicate
		}
		return nil, ErrRedeemFailed
	}

	result := tx.Model(&models.RedemptionCode{}).
		Where("code = ? AND NOT legacy_invalidated AND \"used_count\" < \"max_uses\" AND (\"expires_at\" IS NULL OR \"expires_at\" > ?)", req.Code, now).
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

	if needsPolicySync {
		redemptionID := redemption.ID
		userIDCopy := user.ID
		async.SafeGo("redemption.applyEffectivePolicy", func() {
			if err := policypkg.NewService(s.embyClient()).ApplyEffectiveUserPolicyOrRecordFailure(userIDCopy, "redemption_renewal"); err != nil {
				log.Printf("[Redemption] 兑换续期后同步 Emby Policy 失败: redemptionID=%s userID=%s err=%v",
					redemptionID, userIDCopy, err)
			}
		})
	}

	message := fmt.Sprintf("兑换成功，有效期已延长 %d 天", code.DefaultDays)
	if benefit.ValidityType == entitlementpkg.Permanent {
		message = "兑换成功，已获得对应分组的永久权益"
	}
	log.Printf("[Redemption] 权益兑换完成 userId=%s codeId=%s planGroup=%s validityType=%s", user.ID, code.ID, benefit.PlanGroup, benefit.ValidityType)
	return &RedeemCodeResponse{
		ValidityType: benefit.ValidityType,
		Message:      message,
		Days:         code.DefaultDays,
		ExpiresAt:    newExpiry,
	}, nil
}

// lockUserForRedemptionRenewal 读取并锁定续期目标用户，保证并发兑换在最新到期日上累加。
func lockUserForRedemptionRenewal(tx *gorm.DB, userID string) (*models.User, error) {
	var user models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// calculateRedeemedExpiry 计算兑换后的用户到期日；仍有效的账号从原到期日续期，空或已过期账号从当前时间重新起算。
func calculateRedeemedExpiry(now time.Time, currentExpiry *time.Time, days int) time.Time {
	if currentExpiry == nil || currentExpiry.Before(now) {
		return now.AddDate(0, 0, days)
	}
	return currentExpiry.AddDate(0, 0, days)
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
		Select("r.id, r.\"user_id\", r.code, r.validity_type, r.days, r.\"created_at\", COALESCE(u.username, '') AS username").
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
