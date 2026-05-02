package user

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	"gorm.io/gorm"
)

// UpdateEmailRequest 修改邮箱请求
//
// 当前流程要求用户先调用 POST /api/v1/user/email/send-code 拿到发送至新邮箱的 6 位验证码，
// 再带上 newEmail + code 调 PUT /api/v1/user/email 完成确认。
type UpdateEmailRequest struct {
	NewEmail string `json:"newEmail" binding:"required,email"`
	Code     string `json:"code" binding:"required,len=6"`
}

// UpdateEmailResult 邮箱变更结果，OldEmail 用于 handler 层 fire-and-forget 通知旧邮箱。
type UpdateEmailResult struct {
	OldEmail string
	User     *models.User
}

func (s *UserService) GetProfile(userID string) (*UserView, error) {
	return s.GetUserByID(userID)
}

// UpdateEmail 通过验证码确认后才落库邮箱变更，并返回变更前的旧邮箱用于通知。
func (s *UserService) UpdateEmail(userID string, req *UpdateEmailRequest) (*UpdateEmailResult, error) {
	var user models.User
	if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, ErrUserNotFound
	}

	oldEmail := user.Email
	newEmail := strings.TrimSpace(req.NewEmail)

	// 防止前端弹窗里的 newEmail 在拿到验证码后被改成当前邮箱重新提交。
	if err := unchangedEmailCheck(oldEmail, newEmail); err != nil {
		return nil, err
	}

	// 与 SendEmailChangeCode 共用同一份注册邮箱域名白名单门控；防御 send-code 通过后
	// 管理员收紧白名单时仍能在 commit 前拦下，避免出现"换邮箱接口尚未对齐当前策略"的窗口。
	if err := s.getEmailVerifier().IsRegistrationEmailAllowed(newEmail); err != nil {
		log.Printf("[UserEmailChange] 因域名白名单拦截 userId=%s newHash=%s reason=%v",
			userID, hashEmail(newEmail), err)
		return nil, err
	}

	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// 校验即消费验证码（同事务内删除，不可重放）。
		if err := s.getEmailVerifier().ConsumeCodeTx(tx, newEmail, req.Code, models.VerificationTypeChangeEmail); err != nil {
			return err
		}

		updateErr := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Update("email", newEmail).Error
		if updateErr != nil {
			if isUserUniqueViolation(updateErr, "email") {
				return ErrEmailAlreadyExists
			}
			return ErrUserUpdateFailed
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	user.Email = newEmail

	log.Printf("[UserEmailChange] userId=%s oldHash=%s newHash=%s step=committed",
		userID, hashEmail(oldEmail), hashEmail(newEmail))

	return &UpdateEmailResult{
		OldEmail: oldEmail,
		User:     &user,
	}, nil
}

// hashEmail 对 email 做不可逆哈希，仅取前 8 位 hex 用于日志关联，避免明文落盘。
// 与 handlers.hashEmailForLog 行为一致，但为了避免跨包依赖在 service 包内独立维护一份。
func hashEmail(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return "empty"
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:8]
}

// unchangedEmailCheck 在事务前判定 newEmail 是否与当前邮箱等价。
// 大小写不敏感、忽略首尾空白；命中即拒绝并不消费验证码。
func unchangedEmailCheck(currentEmail, newEmail string) error {
	current := strings.TrimSpace(currentEmail)
	next := strings.TrimSpace(newEmail)
	if strings.EqualFold(current, next) {
		return emailpkg.ErrEmailUnchanged
	}
	return nil
}
