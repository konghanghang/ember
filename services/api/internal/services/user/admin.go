package user

import (
	"errors"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdminUpdateUserRequest 管理员更新用户请求
type AdminUpdateUserRequest struct {
	Email          *string `json:"email"`
	IsActive       *bool   `json:"isActive"`
	PlanGroup      *string `json:"planGroup"`
	ExpiresAt      *string `json:"expiresAt"`
	ClearExpiresAt bool    `json:"clearExpiresAt"`
}

// GetUsersRequest 获取用户列表请求
type GetUsersRequest struct {
	Page         int    `form:"page" binding:"omitempty,min=1"`
	PageSize     int    `form:"pageSize" binding:"omitempty,min=1"`
	Search       string `form:"search"`
	IsActive     *bool  `form:"isActive"`
	ExpiresAfter string `form:"expiresAfter"`
	EmbyStatus   string `form:"embyStatus"`
	PlanGroup    string `form:"planGroup"`
}

// GetUsersResponse 获取用户列表响应
type GetUsersResponse struct {
	Data       []models.User `json:"data"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"pageSize"`
	TotalPages int           `json:"totalPages"`
}

// ExtendExpiryRequest 延长到期时间请求
type ExtendExpiryRequest struct {
	Days int `json:"days" binding:"required,min=1"`
}

func buildUsersWithPlanGroupSelect(query *gorm.DB) *gorm.DB {
	return query.
		Select(`users.*,
			explicit_pg.name AS "planGroupName",
			COALESCE(users."planGroup", default_pg.key) AS "effectivePlanGroup",
			CASE
				WHEN users."planGroup" IS NULL THEN default_pg.name
				WHEN explicit_pg.key IS NULL THEN ''
				ELSE explicit_pg.name
			END AS "effectivePlanGroupName",
			CASE
				WHEN users."planGroup" IS NOT NULL AND explicit_pg.key IS NULL THEN true
				ELSE false
			END AS "isPlanGroupMissing"`).
		Joins(`LEFT JOIN plan_groups explicit_pg ON explicit_pg.key = users."planGroup"`).
		Joins(`LEFT JOIN plan_groups default_pg ON default_pg."isDefault" = ?`, true)
}

func markUsersUsingDefaultPlanGroup(users []models.User) {
	for i := range users {
		users[i].IsUsingDefaultPlanGroup = users[i].PlanGroup == nil && !users[i].IsPlanGroupMissing
	}
}

func (s *UserService) GetUsers(req *GetUsersRequest) (*GetUsersResponse, error) {
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	if _, err := paymentpkg.GetDefaultPlanGroup(nil); err != nil {
		return nil, err
	}

	query := db.DB.Model(&models.User{})
	if req.Search != "" {
		query = query.Where("username LIKE ? OR email LIKE ?", "%"+req.Search+"%", "%"+req.Search+"%")
	}
	if req.IsActive != nil {
		query = query.Where("\"isActive\" = ?", *req.IsActive)
	}
	if req.ExpiresAfter != "" {
		expiresAfter, err := time.Parse("2006-01-02", req.ExpiresAfter)
		if err != nil {
			return nil, ErrInvalidExpiresAfter
		}
		query = query.Where("\"expiresAt\" IS NOT NULL AND \"expiresAt\" > ?", expiresAfter.UTC())
	}

	switch strings.TrimSpace(req.EmbyStatus) {
	case "":
	case "available":
		query = query.Where("COALESCE(\"embyId\", '') <> '' AND \"embyDisabled\" = ?", false)
	case "disabled":
		query = query.Where("COALESCE(\"embyId\", '') <> '' AND \"embyDisabled\" = ?", true)
	case "unlinked":
		query = query.Where("COALESCE(\"embyId\", '') = ''")
	default:
		return nil, ErrInvalidEmbyStatus
	}

	if strings.TrimSpace(req.PlanGroup) != "" {
		defaultGroup, err := paymentpkg.GetDefaultPlanGroup(nil)
		if err != nil {
			return nil, err
		}
		planGroup, err := normalizePlanGroupStrict(req.PlanGroup)
		if err != nil {
			return nil, err
		}
		if defaultGroup.Key == planGroup {
			query = query.Where(`("planGroup" = ? OR "planGroup" IS NULL)`, planGroup)
		} else {
			query = query.Where(`"planGroup" = ?`, planGroup)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var users []models.User
	offset := (req.Page - 1) * req.PageSize
	if err := buildUsersWithPlanGroupSelect(query).
		Offset(offset).
		Limit(req.PageSize).
		Order(`users."createdAt" DESC`).
		Find(&users).Error; err != nil {
		return nil, err
	}
	markUsersUsingDefaultPlanGroup(users)

	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPages++
	}

	return &GetUsersResponse{
		Data:       users,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *UserService) GetUserByID(userID string) (*models.User, error) {
	if _, err := paymentpkg.GetDefaultPlanGroup(nil); err != nil {
		return nil, err
	}
	var user models.User
	result := buildUsersWithPlanGroupSelect(db.DB.Model(&models.User{})).
		Where("users.id = ?", userID).
		First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}
	user.IsUsingDefaultPlanGroup = user.PlanGroup == nil
	return &user, nil
}

func (s *UserService) UpdateUserByAdmin(userID string, req *AdminUpdateUserRequest) (*models.User, error) {
	if req == nil {
		return nil, errors.New("请求参数错误")
	}
	if req.Email == nil && req.IsActive == nil && req.PlanGroup == nil && req.ExpiresAt == nil && !req.ClearExpiresAt {
		return nil, errors.New("至少提供一个可更新字段")
	}
	if req.ClearExpiresAt && req.ExpiresAt != nil {
		return nil, errors.New("clearExpiresAt 和 expiresAt 不能同时设置")
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("更新失败")
	}

	var user models.User
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user)
	if result.Error != nil {
		tx.Rollback()
		return nil, errors.New("用户不存在")
	}

	needSyncEmbyPolicy := false
	oldEffectivePlanGroup, err := paymentpkg.ResolveEffectivePlanGroupKey(tx, user.PlanGroup)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			tx.Rollback()
			return nil, errors.New("邮箱不能为空")
		}
		if _, err := mail.ParseAddress(email); err != nil {
			tx.Rollback()
			return nil, errors.New("邮箱格式错误")
		}
		user.Email = email
	}

	if req.IsActive != nil {
		user.IsActive = *req.IsActive
		needSyncEmbyPolicy = true
	}

	if req.PlanGroup != nil {
		planGroup, err := paymentpkg.NormalizePlanGroupKey(*req.PlanGroup, true)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if planGroup == "" {
			user.PlanGroup = nil
		} else {
			if _, err := paymentpkg.GetPlanGroupByKey(tx, planGroup); err != nil {
				tx.Rollback()
				return nil, err
			}
			user.PlanGroup = &planGroup
		}
	}

	if req.ClearExpiresAt {
		user.ExpiresAt = nil
		needSyncEmbyPolicy = true
	} else if req.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			tx.Rollback()
			return nil, errors.New("expiresAt 必须是 RFC3339 格式")
		}
		expiresAtUTC := expiresAt.UTC()
		user.ExpiresAt = &expiresAtUTC
		needSyncEmbyPolicy = true
	}

	if needSyncEmbyPolicy {
		if err := s.syncEmbyPolicy(&user); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Save(&user).Error; err != nil {
		tx.Rollback()
		if strings.Contains(err.Error(), "duplicate key value") && strings.Contains(err.Error(), "email") {
			return nil, errors.New("邮箱已存在")
		}
		return nil, errors.New("更新失败")
	}

	newEffectivePlanGroup, err := paymentpkg.ResolveEffectivePlanGroupKey(tx, user.PlanGroup)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if oldEffectivePlanGroup != newEffectivePlanGroup {
		expiredCount, err := paymentpkg.ExpirePendingPaymentsForUser(tx, user.ID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		log.Printf("[User] 用户有效套餐分组变更，已收口待支付订单: userID=%s oldPlanGroup=%s newPlanGroup=%s expiredCount=%d",
			user.ID, oldEffectivePlanGroup, newEffectivePlanGroup, expiredCount)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("更新失败")
	}

	return s.GetUserByID(userID)
}

func (s *UserService) ExtendExpiry(userID string, days int) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	var newExpiry time.Time
	now := time.Now().UTC()
	if user.ExpiresAt == nil || user.ExpiresAt.Before(now) {
		newExpiry = now.AddDate(0, 0, days)
	} else {
		newExpiry = user.ExpiresAt.AddDate(0, 0, days)
	}

	user.ExpiresAt = &newExpiry
	if err := s.syncEmbyPolicy(&user); err != nil {
		return nil, err
	}
	if err := db.DB.Save(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *UserService) ToggleUserStatus(userID string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	user.IsActive = !user.IsActive
	if err := s.syncEmbyPolicy(&user); err != nil {
		return nil, err
	}
	if err := db.DB.Save(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *UserService) DeleteUser(userID string) error {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return errors.New("用户不存在")
	}

	if user.EmbyID != "" {
		embyService := embyint.NewEmbyService()
		if err := embyService.DeleteUser(user.EmbyID); err != nil {
			return errors.New("删除用户失败：" + err.Error())
		}
	}

	if err := db.DB.Delete(&user).Error; err != nil {
		return err
	}

	return nil
}
