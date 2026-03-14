package services

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type telegramRedeemer interface {
	Redeem(userID, code string) (*TelegramRedeemResponse, error)
}

type telegramSubscriber interface {
	Create(userID string, req telegramSubscriptionCommand) error
}

// TelegramService Telegram 绑定与 Bot 能力
type TelegramService struct {
	redemptionService   telegramRedeemer
	subscriptionService telegramSubscriber
	newEmbyService      func() *embyint.EmbyService
}

func NewTelegramService() *TelegramService {
	return &TelegramService{
		redemptionService:   telegramRedeemerAdapter{},
		subscriptionService: telegramSubscriberAdapter{},
		newEmbyService:      embyint.NewEmbyService,
	}
}

// BindResult 绑定成功返回
type BindResult struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// AccountInfoResponse Bot 查询账号信息响应
type AccountInfoResponse struct {
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	IsExpired    bool       `json:"isExpired"`
	IsActive     bool       `json:"isActive"`
	EmbyDisabled bool       `json:"embyDisabled"`
}

// TelegramBindRequest Bot 调 Internal API 验证绑定
type TelegramBindRequest struct {
	TelegramID int64  `json:"telegramId" binding:"required"`
	Code       string `json:"code" binding:"required,len=6"`
}

// TelegramRedeemRequest Bot 调 Internal API 兑换
type TelegramRedeemRequest struct {
	TelegramID int64  `json:"telegramId" binding:"required"`
	Code       string `json:"code" binding:"required"`
}

type TelegramRedeemResponse struct {
	Message   string     `json:"message"`
	Days      int        `json:"days"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// TelegramResetPasswordRequest Bot 调 Internal API 重置密码
type TelegramResetPasswordRequest struct {
	TelegramID  int64  `json:"telegramId" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// TelegramIDRequest Bot 调 Internal API 查询账号信息
type TelegramIDRequest struct {
	TelegramID int64 `json:"telegramId" binding:"required"`
}

// TelegramSubscribeRequest Bot 调 Internal API 创建求片订阅
type TelegramSubscribeRequest struct {
	TelegramID int64  `json:"telegramId" binding:"required"`
	Type       string `json:"type" binding:"required,oneof=MOVIE TV"`
	Name       string `json:"name" binding:"required"`
	TmdbID     string `json:"tmdbId" binding:"required"`
	PosterPath string `json:"posterPath"`
	Note       string `json:"note"`
}

type telegramSubscriptionCommand struct {
	Type       models.MediaType
	Name       string
	TmdbID     string
	PosterPath *string
	Note       *string
}

// GenerateBindCode 生成绑定验证码
func (s *TelegramService) GenerateBindCode(userID string) (string, time.Time, error) {
	var user models.User
	if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return "", time.Time{}, errors.New("用户不存在")
	}
	if user.TelegramID != nil {
		return "", time.Time{}, ErrUserAlreadyBoundTelegram
	}

	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	const maxAttempts = 8
	for i := 0; i < maxAttempts; i++ {
		code := generateTelegramBindCode()

		err := db.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("\"userId\" = ?", userID).Delete(&models.TelegramBindCode{}).Error; err != nil {
				return err
			}
			return tx.Create(&models.TelegramBindCode{
				UserID:    userID,
				Code:      code,
				ExpiresAt: expiresAt,
			}).Error
		})
		if err == nil {
			return code, expiresAt, nil
		}
		if isTelegramCodeDuplicateErr(err) {
			continue
		}
		return "", time.Time{}, errors.New("生成绑定验证码失败")
	}

	return "", time.Time{}, errors.New("生成绑定验证码失败")
}

// VerifyBind 校验绑定验证码并绑定 Telegram 账号
func (s *TelegramService) VerifyBind(telegramID int64, code string) (*BindResult, error) {
	var bindCodes []models.TelegramBindCode
	if err := db.DB.
		Where("code = ? AND \"expiresAt\" > ?", code, time.Now().UTC()).
		Order("\"createdAt\" DESC").
		Limit(2).
		Find(&bindCodes).Error; err != nil {
		return nil, errors.New("绑定失败，请稍后重试")
	}
	if len(bindCodes) == 0 {
		return nil, ErrTelegramBindCodeInvalid
	}
	if len(bindCodes) > 1 {
		return nil, errors.New("绑定失败，请稍后重试")
	}
	bindCode := bindCodes[0]

	var existing models.User
	if err := db.DB.Where("\"telegramId\" = ?", telegramID).First(&existing).Error; err == nil {
		return nil, ErrTelegramAlreadyBound
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("绑定失败，请稍后重试")
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("绑定失败，请稍后重试")
	}

	var user models.User
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", bindCode.UserID).
		First(&user).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTelegramBindCodeInvalid
		}
		return nil, errors.New("绑定失败，请稍后重试")
	}

	if user.TelegramID != nil {
		tx.Rollback()
		return nil, ErrUserAlreadyBoundTelegram
	}

	user.TelegramID = &telegramID
	if err := tx.Save(&user).Error; err != nil {
		tx.Rollback()
		if strings.Contains(err.Error(), "duplicate key value") && strings.Contains(err.Error(), "telegramId") {
			return nil, ErrTelegramAlreadyBound
		}
		return nil, errors.New("绑定失败，请稍后重试")
	}

	if err := tx.Where("\"userId\" = ?", bindCode.UserID).Delete(&models.TelegramBindCode{}).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("绑定失败，请稍后重试")
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("绑定失败，请稍后重试")
	}

	return &BindResult{
		UserID:   user.ID,
		Username: user.Username,
	}, nil
}

// Unbind 解绑 Telegram
func (s *TelegramService) Unbind(userID string) error {
	var user models.User
	if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return errors.New("用户不存在")
	}
	if user.TelegramID == nil {
		return ErrTelegramNotBound
	}

	if err := db.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Update("telegramId", nil).Error; err != nil {
		return errors.New("解绑失败，请稍后重试")
	}

	return nil
}

// GetAccountInfo 按 Telegram ID 查询账号信息
func (s *TelegramService) GetAccountInfo(telegramID int64) (*AccountInfoResponse, error) {
	var user models.User
	if err := db.DB.Where("\"telegramId\" = ?", telegramID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTelegramNotBound
		}
		return nil, errors.New("查询账号信息失败，请稍后重试")
	}

	return &AccountInfoResponse{
		Username:     user.Username,
		Email:        user.Email,
		ExpiresAt:    user.ExpiresAt,
		IsExpired:    user.IsExpired(),
		IsActive:     user.IsActive,
		EmbyDisabled: user.EmbyDisabled,
	}, nil
}

// RedeemByTelegram 通过 Telegram 兑换续期码
func (s *TelegramService) RedeemByTelegram(telegramID int64, code string) (*TelegramRedeemResponse, error) {
	var user models.User
	if err := db.DB.Where("\"telegramId\" = ?", telegramID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTelegramNotBound
		}
		return nil, errors.New("兑换失败，请稍后重试")
	}

	return s.redeemForUser(user.ID, code)
}

// ResetPassword 通过 Telegram 身份重置密码
func (s *TelegramService) ResetPassword(telegramID int64, newPassword string) error {
	var user models.User
	if err := db.DB.Where("\"telegramId\" = ?", telegramID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTelegramNotBound
		}
		return errors.New("密码重置失败，请稍后重试")
	}

	if user.EmbyID != "" {
		embyService := s.newEmbyService()
		if err := embyService.UpdateUserPassword(user.EmbyID, newPassword); err != nil {
			return errors.New("密码重置失败：" + err.Error())
		}
	}

	if err := user.SetPassword(newPassword); err != nil {
		return errors.New("密码重置失败：本地密码更新失败")
	}
	if err := db.DB.Save(&user).Error; err != nil {
		return errors.New("密码重置失败：本地密码保存失败")
	}

	return nil
}

// SubscribeByTelegram 通过 Telegram 身份创建求片订阅
func (s *TelegramService) SubscribeByTelegram(req TelegramSubscribeRequest) error {
	var user models.User
	if err := db.DB.Where("\"telegramId\" = ?", req.TelegramID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTelegramNotBound
		}
		return errors.New("订阅失败，请稍后重试")
	}

	var posterPath *string
	if req.PosterPath != "" {
		posterPath = &req.PosterPath
	}
	var note *string
	if req.Note != "" {
		note = &req.Note
	}

	return s.subscribeForUser(user.ID, req, posterPath, note)
}

// CleanupExpiredBindCodes 清理过期绑定码
func (s *TelegramService) CleanupExpiredBindCodes() (int64, error) {
	result := db.DB.Where("\"expiresAt\" < ?", time.Now().UTC()).Delete(&models.TelegramBindCode{})
	return result.RowsAffected, result.Error
}

func isTelegramCodeDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") &&
		strings.Contains(msg, "telegram_bind_codes") &&
		strings.Contains(msg, "code")
}

func (s *TelegramService) redeemForUser(userID, code string) (*TelegramRedeemResponse, error) {
	return s.redemptionService.Redeem(userID, code)
}

func (s *TelegramService) subscribeForUser(
	userID string,
	req TelegramSubscribeRequest,
	posterPath *string,
	note *string,
) error {
	return s.subscriptionService.Create(userID, telegramSubscriptionCommand{
		Type:       models.MediaType(req.Type),
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		PosterPath: posterPath,
		Note:       note,
	})
}

type telegramRedeemerAdapter struct{}

func (telegramRedeemerAdapter) Redeem(userID, code string) (*TelegramRedeemResponse, error) {
	resp, err := (&RedemptionService{}).RedeemCode(userID, &RedeemCodeRequest{Code: code})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &TelegramRedeemResponse{
		Message:   resp.Message,
		Days:      resp.Days,
		ExpiresAt: resp.ExpiresAt,
	}, nil
}

type telegramSubscriberAdapter struct{}

func (telegramSubscriberAdapter) Create(userID string, req telegramSubscriptionCommand) error {
	return NewSubscriptionService().CreateSubscription(userID, CreateSubscriptionRequest{
		Type:       req.Type,
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		PosterPath: req.PosterPath,
		Note:       req.Note,
	})
}

func generateTelegramBindCode() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "000000"
	}
	num := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 1000000
	return fmt.Sprintf("%06d", num)
}
