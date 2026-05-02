package redemption

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/gorm"
)

type RedemptionCodeService struct{}

const (
	maxCreateRedemptionCodesCount = 100
	maxCodeInsertRetry            = 5
)

var (
	redemptionFindCodeByValue = func(code string) (*models.RedemptionCode, error) {
		var redemptionCode models.RedemptionCode
		if err := db.DB.Where("code = ?", code).First(&redemptionCode).Error; err != nil {
			return nil, err
		}
		return &redemptionCode, nil
	}
	redemptionNormalizePlanGroupKey = paymentpkg.NormalizePlanGroupKey
	redemptionGetPlanGroupByKey     = paymentpkg.GetPlanGroupByKey
	redemptionGetPlanGroupForUpdate = paymentpkg.GetPlanGroupByKeyForUpdate
)

func (s *RedemptionCodeService) CreateRedemptionCode(req *CreateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	codes, err := s.createRedemptionCodes(req.RedemptionCodeCreateOptions, 1)
	if err != nil {
		return nil, err
	}

	return &codes[0], nil
}

func (s *RedemptionCodeService) CreateRedemptionCodesBatch(req *CreateRedemptionCodesBatchRequest) (*CreateRedemptionCodesBatchResponse, error) {
	codes, err := s.createRedemptionCodes(req.RedemptionCodeCreateOptions, req.Count)
	if err != nil {
		return nil, err
	}

	return &CreateRedemptionCodesBatchResponse{
		Data:  codes,
		Count: len(codes),
	}, nil
}

func (s *RedemptionCodeService) createRedemptionCodes(options RedemptionCodeCreateOptions, count int) ([]models.RedemptionCode, error) {
	if count < 1 || count > maxCreateRedemptionCodesCount {
		return nil, ErrRedemptionCodeBatchCountInvalid
	}

	templateUserID, err := s.validateTemplateUserID(options.TemplateUserID)
	if err != nil {
		return nil, err
	}
	registrationPlanGroup, err := s.validateRegistrationPlanGroup(options.RegistrationPlanGroup)
	if err != nil {
		return nil, err
	}

	baseCode := models.RedemptionCode{
		MaxUses:               options.MaxUses,
		DefaultDays:           options.DefaultDays,
		ExpiresAt:             options.ExpiresAt,
		TemplateUserID:        templateUserID,
		RegistrationPlanGroup: registrationPlanGroup,
		Notes:                 options.Notes,
	}

	codes := make([]models.RedemptionCode, 0, count)
	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		if err := s.ensureRegistrationPlanGroupExists(tx, registrationPlanGroup, true); err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			code, err := s.createSingleRedemptionCode(tx, baseCode)
			if err != nil {
				return err
			}
			codes = append(codes, code)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if err := s.fillDisplayFields(codes); err != nil {
		return nil, errors.New("创建兑换码失败")
	}

	return codes, nil
}

func (s *RedemptionCodeService) GetRedemptionCodes(req *GetRedemptionCodesRequest) (*GetRedemptionCodesResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	var total int64
	var codes []models.RedemptionCode

	query := db.DB.Model(&models.RedemptionCode{})
	now := time.Now().UTC()

	if code := strings.TrimSpace(req.Code); code != "" {
		query = query.Where("code ILIKE ?", "%"+code+"%")
	}

	if templateUserID := strings.TrimSpace(req.TemplateUserID); templateUserID != "" {
		query = query.Where("\"template_user_id\" = ?", templateUserID)
	}
	if registrationPlanGroup := strings.TrimSpace(req.RegistrationPlanGroup); registrationPlanGroup != "" {
		normalizedRegistrationPlanGroup, err := redemptionNormalizePlanGroupKey(registrationPlanGroup, false)
		if err != nil {
			return nil, err
		}
		query = query.Where("\"registration_plan_group\" = ?", normalizedRegistrationPlanGroup)
	}

	status := RedemptionCodeStatus(strings.TrimSpace(req.Status))
	if status != "" {
		var err error
		query, err = applyRedemptionCodeStatusFilter(query, status, now)
		if err != nil {
			return nil, err
		}
	} else if !req.ShowAll {
		query = query.Where("\"used_count\" < \"max_uses\" AND (\"expires_at\" IS NULL OR \"expires_at\" > ?)", now)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("获取兑换码数量失败")
	}

	offset := (page - 1) * pageSize
	if err := query.Order("\"created_at\" DESC").Offset(offset).Limit(pageSize).Find(&codes).Error; err != nil {
		return nil, errors.New("获取兑换码列表失败")
	}

	if err := s.fillDisplayFields(codes); err != nil {
		return nil, errors.New("获取兑换码列表失败")
	}

	return &GetRedemptionCodesResponse{
		Data:       codes,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}

func (s *RedemptionCodeService) DeleteRedemptionCode(id string) error {
	result := db.DB.Delete(&models.RedemptionCode{}, "id = ?", id)
	if result.Error != nil {
		return errors.New("删除兑换码失败")
	}
	if result.RowsAffected == 0 {
		return ErrRedemptionCodeNotFound
	}
	return nil
}

func (s *RedemptionCodeService) UpdateRedemptionCode(id string, req *UpdateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	templateUserID, err := s.validateTemplateUserID(req.TemplateUserID)
	if err != nil {
		return nil, err
	}
	registrationPlanGroup, err := s.validateRegistrationPlanGroup(req.RegistrationPlanGroup)
	if err != nil {
		return nil, err
	}

	var redemptionCode models.RedemptionCode
	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&redemptionCode).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRedemptionCodeNotFound
			}
			return errors.New("更新兑换码失败")
		}

		if err := s.ensureRegistrationPlanGroupExists(tx, registrationPlanGroup, true); err != nil {
			return err
		}

		if req.MaxUses < redemptionCode.UsedCount {
			return ErrRedemptionCodeUsedOver
		}

		redemptionCode.MaxUses = req.MaxUses
		redemptionCode.DefaultDays = req.DefaultDays
		redemptionCode.ExpiresAt = req.ExpiresAt
		redemptionCode.TemplateUserID = templateUserID
		redemptionCode.RegistrationPlanGroup = registrationPlanGroup
		redemptionCode.Notes = req.Notes

		if err := tx.Model(&models.RedemptionCode{}).
			Where("id = ?", redemptionCode.ID).
			Updates(map[string]interface{}{
				"max_uses":                redemptionCode.MaxUses,
				"default_days":            redemptionCode.DefaultDays,
				"expires_at":              redemptionCode.ExpiresAt,
				"template_user_id":        redemptionCode.TemplateUserID,
				"registration_plan_group": redemptionCode.RegistrationPlanGroup,
				"notes":                   redemptionCode.Notes,
			}).Error; err != nil {
			return errors.New("更新兑换码失败")
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if err := s.enrichCode(&redemptionCode); err != nil {
		return nil, errors.New("更新兑换码失败")
	}

	return &redemptionCode, nil
}

func (s *RedemptionCodeService) GetUserTemplates() (*GetUserTemplatesResponse, error) {
	now := time.Now().UTC()
	var users []models.User
	if err := db.DB.
		Model(&models.User{}).
		Where("role = ? AND \"is_active\" = true AND (\"expires_at\" IS NULL OR \"expires_at\" > ?)", "user", now).
		Order("username ASC").
		Find(&users).Error; err != nil {
		return nil, errors.New("获取模板用户失败")
	}

	templates := make([]UserTemplate, 0, len(users))
	for _, user := range users {
		templates = append(templates, UserTemplate{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			ExpiresAt: user.ExpiresAt,
		})
	}

	return &GetUserTemplatesResponse{
		Data: templates,
	}, nil
}

func (s *RedemptionCodeService) ValidateRegistrationCode(code string) (*models.RedemptionCode, error) {
	redemptionCode, err := s.validateUsableCode(code)
	if err != nil {
		return nil, err
	}
	if err := s.ensureRegistrationPlanGroupAvailable(redemptionCode); err != nil {
		return nil, err
	}
	if err := s.enrichCode(redemptionCode); err != nil {
		return nil, errors.New("校验兑换码失败")
	}
	return redemptionCode, nil
}

func (s *RedemptionCodeService) ValidateRenewalCode(code string) (*models.RedemptionCode, error) {
	redemptionCode, err := s.validateUsableCode(code)
	if err != nil {
		return nil, err
	}
	if err := s.enrichCode(redemptionCode); err != nil {
		return nil, errors.New("校验兑换码失败")
	}
	return redemptionCode, nil
}

func (s *RedemptionCodeService) UseCode(code string) error {
	result := db.DB.Model(&models.RedemptionCode{}).
		Where("code = ? AND \"used_count\" < \"max_uses\"", code).
		Update("used_count", gorm.Expr("\"used_count\" + 1"))

	if result.Error != nil {
		return errors.New("使用兑换码失败")
	}
	if result.RowsAffected == 0 {
		return ErrRedemptionCodeInvalid
	}
	return nil
}

func (s *RedemptionCodeService) generateCode(length int) (string, error) {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

func (s *RedemptionCodeService) validateTemplateUserID(templateUserID *string) (*string, error) {
	if templateUserID == nil || *templateUserID == "" {
		return nil, nil
	}

	var user models.User
	if err := db.DB.Where("id = ?", *templateUserID).First(&user).Error; err != nil {
		return nil, ErrTemplateUserNotFound
	}

	if user.Role != "user" {
		return nil, ErrTemplateUserMustBeUser
	}

	if user.EmbyID == "" {
		return nil, ErrTemplateUserEmbyRequired
	}

	validID := user.ID
	return &validID, nil
}

func (s *RedemptionCodeService) validateRegistrationPlanGroup(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}

	normalizedPlanGroup, err := redemptionNormalizePlanGroupKey(*raw, true)
	if err != nil {
		return nil, err
	}
	if normalizedPlanGroup == "" {
		return nil, nil
	}

	validPlanGroup := normalizedPlanGroup
	return &validPlanGroup, nil
}

func (s *RedemptionCodeService) ensureRegistrationPlanGroupExists(tx *gorm.DB, registrationPlanGroup *string, lockForUpdate bool) error {
	if registrationPlanGroup == nil || *registrationPlanGroup == "" {
		return nil
	}

	lookup := redemptionGetPlanGroupByKey
	if lockForUpdate {
		lookup = redemptionGetPlanGroupForUpdate
	}
	if _, err := lookup(tx, *registrationPlanGroup); err != nil {
		return err
	}
	return nil
}

func (s *RedemptionCodeService) fillDisplayFields(codes []models.RedemptionCode) error {
	if err := s.fillTemplateUserNames(codes); err != nil {
		return err
	}
	if err := s.fillRegistrationPlanGroupNames(codes); err != nil {
		return err
	}
	return nil
}

func (s *RedemptionCodeService) fillTemplateUserNames(codes []models.RedemptionCode) error {
	if db.DB == nil {
		return nil
	}

	userIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range codes {
		if codes[i].TemplateUserID == nil || *codes[i].TemplateUserID == "" {
			continue
		}
		id := *codes[i].TemplateUserID
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		userIDs = append(userIDs, id)
	}

	if len(userIDs) == 0 {
		return nil
	}

	var users []models.User
	if err := db.DB.Model(&models.User{}).Select("id", "username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return err
	}

	nameMap := make(map[string]string, len(users))
	for _, user := range users {
		nameMap[user.ID] = user.Username
	}

	for i := range codes {
		if codes[i].TemplateUserID == nil || *codes[i].TemplateUserID == "" {
			continue
		}
		if name, ok := nameMap[*codes[i].TemplateUserID]; ok {
			codes[i].TemplateUserName = &name
		}
	}

	return nil
}

func (s *RedemptionCodeService) fillRegistrationPlanGroupNames(codes []models.RedemptionCode) error {
	if db.DB == nil {
		return nil
	}

	planGroups := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range codes {
		if codes[i].RegistrationPlanGroup == nil || *codes[i].RegistrationPlanGroup == "" {
			continue
		}
		key := *codes[i].RegistrationPlanGroup
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		planGroups = append(planGroups, key)
	}

	if len(planGroups) == 0 {
		return nil
	}

	var groups []models.PlanGroup
	if err := db.DB.Model(&models.PlanGroup{}).Select("key", "name").Where("key IN ?", planGroups).Find(&groups).Error; err != nil {
		return err
	}

	nameMap := make(map[string]string, len(groups))
	for _, group := range groups {
		nameMap[group.Key] = group.Name
	}

	for i := range codes {
		if codes[i].RegistrationPlanGroup == nil || *codes[i].RegistrationPlanGroup == "" {
			continue
		}
		if name, ok := nameMap[*codes[i].RegistrationPlanGroup]; ok {
			codes[i].RegistrationPlanGroupName = &name
		}
	}

	return nil
}

func (s *RedemptionCodeService) validateUsableCode(code string) (*models.RedemptionCode, error) {
	trimmedCode := strings.TrimSpace(code)
	redemptionCode, err := redemptionFindCodeByValue(trimmedCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRedemptionCodeNotFound
		}
		return nil, errors.New("校验兑换码失败")
	}
	if !redemptionCode.IsValid() {
		return nil, ErrRedemptionCodeInvalid
	}
	return redemptionCode, nil
}

func (s *RedemptionCodeService) ensureRegistrationPlanGroupAvailable(redemptionCode *models.RedemptionCode) error {
	if redemptionCode == nil || redemptionCode.RegistrationPlanGroup == nil || *redemptionCode.RegistrationPlanGroup == "" {
		return nil
	}

	if _, err := redemptionGetPlanGroupByKey(nil, *redemptionCode.RegistrationPlanGroup); err != nil {
		if errors.Is(err, paymentpkg.ErrPlanGroupNotFound) {
			log.Printf("[RedemptionCode] 注册兑换码绑定的套餐分组已失效: codeID=%s planGroup=%s", redemptionCode.ID, *redemptionCode.RegistrationPlanGroup)
			return ErrRegistrationPlanGroupNotFound
		}
		return err
	}
	return nil
}

func (s *RedemptionCodeService) enrichCode(code *models.RedemptionCode) error {
	if code == nil {
		return nil
	}

	items := []models.RedemptionCode{*code}
	if err := s.fillDisplayFields(items); err != nil {
		return err
	}
	*code = items[0]
	return nil
}

func (s *RedemptionCodeService) createSingleRedemptionCode(tx *gorm.DB, baseCode models.RedemptionCode) (models.RedemptionCode, error) {
	for attempt := 0; attempt < maxCodeInsertRetry; attempt++ {
		code, err := s.generateCode(16)
		if err != nil {
			return models.RedemptionCode{}, errors.New("生成兑换码失败")
		}

		redemptionCode := baseCode
		redemptionCode.Code = code

		if err := tx.Create(&redemptionCode).Error; err != nil {
			if isRedemptionCodeConflict(err) {
				continue
			}
			return models.RedemptionCode{}, errors.New("创建兑换码失败")
		}

		return redemptionCode, nil
	}

	return models.RedemptionCode{}, errors.New("生成兑换码失败")
}

func isRedemptionCodeConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505"
}

func applyRedemptionCodeStatusFilter(query *gorm.DB, status RedemptionCodeStatus, now time.Time) (*gorm.DB, error) {
	switch status {
	case RedemptionCodeStatusActive:
		return query.Where("\"used_count\" < \"max_uses\" AND (\"expires_at\" IS NULL OR \"expires_at\" > ?)", now), nil
	case RedemptionCodeStatusExpired:
		return query.Where("\"used_count\" < \"max_uses\" AND \"expires_at\" IS NOT NULL AND \"expires_at\" <= ?", now), nil
	case RedemptionCodeStatusExhausted:
		return query.Where("\"used_count\" >= \"max_uses\""), nil
	case "":
		return query, nil
	default:
		return nil, ErrRedemptionCodeStatusInvalid
	}
}
