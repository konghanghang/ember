package email

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockVerificationRateLimitScope(tx *gorm.DB, scope string) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", scope).Error
}

func validateVerificationRateLimits(emailCount, ipCount int64, dailyLimit, ipDailyLimit int) error {
	if emailCount >= int64(dailyLimit) {
		return ErrEmailCodeRateLimit
	}
	if ipCount >= int64(ipDailyLimit) {
		return ErrEmailCodeIPRateLimit
	}
	return nil
}

func normalizeVerificationEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// evaluateVerificationCode 纯函数：判断已锁定的 verification 记录是否能与提交的 code 匹配
func evaluateVerificationCode(verification *models.EmailVerification, code string) error {
	if verification == nil {
		return ErrEmailCodeInvalid
	}
	if verification.IsExpired() {
		return ErrEmailCodeInvalid
	}
	if verification.Code != code {
		return ErrEmailCodeInvalid
	}
	return nil
}

func (s *EmailService) findLatestVerification(tx *gorm.DB, email, codeType string, lock bool) (*models.EmailVerification, error) {
	normalizedEmail := normalizeVerificationEmail(email)
	query := tx.Where("lower(email) = ? AND \"type\" = ?", normalizedEmail, codeType).
		Order("\"createdAt\" DESC")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	var verification models.EmailVerification
	if err := query.First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEmailCodeInvalid
		}
		return nil, err
	}
	return &verification, nil
}

// SendVerificationCode 发送验证码
// ip 参数由 handler 层通过 c.ClientIP() 传入
// codeType 取值：models.VerificationTypeRegister 或 models.VerificationTypeReset
//
// 反账号枚举约束：reset 路径无论邮箱是否已注册都先消耗同一套 IP / email 限流配额，
// 未注册邮箱仍写入限流计数行（不发送邮件）并返回 ErrEmailNotRegistered，由 handler 收口
// 为统一 200 响应；攻击者无法借限流差异（200 vs 429）或响应体差异枚举注册状态。
func (s *EmailService) SendVerificationCode(email, ip, codeType string) error {
	s.refreshConfig()
	if !s.IsConfigured() {
		return ErrEmailNotConfigured
	}
	normalizedEmail := normalizeVerificationEmail(email)

	var existingUserCount int64
	db.DB.Model(&models.User{}).Where("lower(email) = ?", normalizedEmail).Count(&existingUserCount)

	// 注册场景：邮箱已存在直接拒绝（业务约束，前端注册页可见）。reset 不在此处提前返回。
	if codeType == models.VerificationTypeRegister && existingUserCount > 0 {
		return ErrEmailAlreadyRegistered
	}

	code := generateVerificationCode()

	verification := models.EmailVerification{
		Email:     normalizedEmail,
		Code:      code,
		Type:      codeType,
		IP:        ip,
		ExpiresAt: time.Now().UTC().Add(time.Duration(s.expiryMinutes) * time.Minute),
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		log.Printf("发送验证码开启事务失败 [%s]: %v", normalizedEmail, tx.Error)
		return ErrEmailSendFailed
	}

	if err := lockVerificationRateLimitScope(tx, fmt.Sprintf("email-verification:%s:%s", normalizedEmail, codeType)); err != nil {
		tx.Rollback()
		log.Printf("发送验证码锁定邮箱限流失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailSendFailed
	}
	if err := lockVerificationRateLimitScope(tx, fmt.Sprintf("email-verification-ip:%s:%s", ip, codeType)); err != nil {
		tx.Rollback()
		log.Printf("发送验证码锁定 IP 限流失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailSendFailed
	}

	since := time.Now().UTC().Add(-24 * time.Hour)

	var emailCount int64
	if err := tx.Model(&models.EmailVerification{}).
		Where("lower(email) = ? AND \"type\" = ? AND \"createdAt\" > ?", normalizedEmail, codeType, since).
		Count(&emailCount).Error; err != nil {
		tx.Rollback()
		log.Printf("发送验证码统计邮箱次数失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailSendFailed
	}

	var ipCount int64
	if err := tx.Model(&models.EmailVerification{}).
		Where("ip = ? AND \"type\" = ? AND \"createdAt\" > ?", ip, codeType, since).
		Count(&ipCount).Error; err != nil {
		tx.Rollback()
		log.Printf("发送验证码统计 IP 次数失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailSendFailed
	}
	if err := validateVerificationRateLimits(emailCount, ipCount, s.dailyLimit, s.ipDailyLimit); err != nil {
		tx.Rollback()
		return err
	}

	// 重置 + 邮箱未注册：写入限流计数行（与已注册路径计数一致），不发送邮件，
	// 返回 ErrEmailNotRegistered；handler 层将其与限流 / 发送失败一起折叠为 200 + 统一文案。
	if codeType == models.VerificationTypeReset && existingUserCount == 0 {
		if err := tx.Create(&verification).Error; err != nil {
			tx.Rollback()
			log.Printf("[Email] 记录未注册重置请求失败 ip=%s err=%v", ip, err)
			return ErrEmailSendFailed
		}
		if err := tx.Commit().Error; err != nil {
			log.Printf("发送验证码提交事务失败 [%s]: %v", normalizedEmail, err)
			return ErrEmailSendFailed
		}
		return ErrEmailNotRegistered
	}

	subject := "Ember 注册验证码"
	action := "注册"
	if codeType == models.VerificationTypeReset {
		subject = "Ember 密码重置验证码"
		action = "密码重置"
	}
	body := fmt.Sprintf("你的 Ember %s验证码是：%s\n有效期 %d 分钟，请勿泄露给他人。", action, code, s.expiryMinutes)

	if err := tx.Create(&verification).Error; err != nil {
		tx.Rollback()
		log.Printf("发送验证码保存记录失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailSendFailed
	}

	if err := s.sendEmail(normalizedEmail, subject, body); err != nil {
		tx.Rollback()
		log.Printf("发送验证码邮件失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailSendFailed
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("发送验证码提交事务失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailSendFailed
	}

	return nil
}

// VerifyCode 校验验证码：在事务中 SELECT ... FOR UPDATE 锁行 → 校验有效性 → 立即 DELETE。
// 校验通过即消费，同一码不可重放用于注册或密码重置。
func (s *EmailService) VerifyCode(email, code, codeType string) error {
	return db.DB.Transaction(func(tx *gorm.DB) error {
		return s.ConsumeCodeTx(tx, email, code, codeType)
	})
}

// CheckCode 只校验验证码，不消费，供需要把“消费”并入业务事务的链路使用。
func (s *EmailService) CheckCode(email, code, codeType string) error {
	normalizedEmail := normalizeVerificationEmail(email)
	verification, err := s.findLatestVerification(db.DB, normalizedEmail, codeType, false)
	if err != nil {
		if errors.Is(err, ErrEmailCodeInvalid) {
			return ErrEmailCodeInvalid
		}
		log.Printf("[Email] 校验验证码事务失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailCodeInvalid
	}
	return evaluateVerificationCode(verification, code)
}

// ConsumeCodeTx 在调用方事务内校验并消费验证码。
func (s *EmailService) ConsumeCodeTx(tx *gorm.DB, email, code, codeType string) error {
	normalizedEmail := normalizeVerificationEmail(email)
	verification, err := s.findLatestVerification(tx, normalizedEmail, codeType, true)
	if err != nil {
		if errors.Is(err, ErrEmailCodeInvalid) {
			return ErrEmailCodeInvalid
		}
		log.Printf("[Email] 校验验证码查询失败 [%s]: %v", normalizedEmail, err)
		return ErrEmailCodeInvalid
	}
	if err := evaluateVerificationCode(verification, code); err != nil {
		return err
	}
	if err := tx.Where("id = ?", verification.ID).Delete(&models.EmailVerification{}).Error; err != nil {
		log.Printf("[Email] 校验验证码消费失败 [%s id=%s]: %v", normalizedEmail, verification.ID, err)
		return ErrEmailCodeInvalid
	}
	return nil
}

// CleanupExpired 清理过期验证码（供 cron 调用）
func (s *EmailService) CleanupExpired() (int64, error) {
	result := db.DB.Where("\"expiresAt\" < ?", time.Now().UTC()).
		Delete(&models.EmailVerification{})
	return result.RowsAffected, result.Error
}

// generateVerificationCode 生成 6 位随机数字验证码
func generateVerificationCode() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "000000"
	}
	num := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 1000000
	return fmt.Sprintf("%06d", num)
}
