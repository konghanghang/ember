package services

import (
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// SendVerificationCode 发送验证码
// ip 参数由 handler 层通过 c.ClientIP() 传入
// codeType 取值：models.VerificationTypeRegister 或 models.VerificationTypeReset
func (s *EmailService) SendVerificationCode(email, ip, codeType string) error {
	s.refreshConfig()
	if !s.IsConfigured() {
		return ErrEmailNotConfigured
	}

	var existingUserCount int64
	db.DB.Model(&models.User{}).Where("email = ?", email).Count(&existingUserCount)
	if codeType == models.VerificationTypeRegister && existingUserCount > 0 {
		return ErrEmailAlreadyRegistered
	}
	if codeType == models.VerificationTypeReset && existingUserCount == 0 {
		return ErrEmailNotRegistered
	}

	since := time.Now().UTC().Add(-24 * time.Hour)

	var emailCount int64
	db.DB.Model(&models.EmailVerification{}).
		Where("email = ? AND \"type\" = ? AND \"createdAt\" > ?", email, codeType, since).
		Count(&emailCount)
	if emailCount >= int64(s.dailyLimit) {
		return ErrEmailCodeRateLimit
	}

	var ipCount int64
	db.DB.Model(&models.EmailVerification{}).
		Where("ip = ? AND \"createdAt\" > ?", ip, since).
		Count(&ipCount)
	if ipCount >= int64(s.ipDailyLimit) {
		return ErrEmailCodeIPRateLimit
	}

	code := generateVerificationCode()

	verification := models.EmailVerification{
		Email:     email,
		Code:      code,
		Type:      codeType,
		IP:        ip,
		ExpiresAt: time.Now().UTC().Add(time.Duration(s.expiryMinutes) * time.Minute),
	}

	subject := "Ember 注册验证码"
	action := "注册"
	if codeType == models.VerificationTypeReset {
		subject = "Ember 密码重置验证码"
		action = "密码重置"
	}
	body := fmt.Sprintf("你的 Ember %s验证码是：%s\n有效期 %d 分钟，请勿泄露给他人。", action, code, s.expiryMinutes)

	tx := db.DB.Begin()
	if tx.Error != nil {
		log.Printf("发送验证码开启事务失败 [%s]: %v", email, tx.Error)
		return ErrEmailSendFailed
	}

	if err := tx.Create(&verification).Error; err != nil {
		tx.Rollback()
		log.Printf("发送验证码保存记录失败 [%s]: %v", email, err)
		return ErrEmailSendFailed
	}

	if err := s.sendEmail(email, subject, body); err != nil {
		tx.Rollback()
		log.Printf("发送验证码邮件失败 [%s]: %v", email, err)
		return ErrEmailSendFailed
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("发送验证码提交事务失败 [%s]: %v", email, err)
		return ErrEmailSendFailed
	}

	return nil
}

// VerifyCode 校验验证码
func (s *EmailService) VerifyCode(email, code, codeType string) error {
	var verification models.EmailVerification
	result := db.DB.Where("email = ? AND \"type\" = ?", email, codeType).
		Order("\"createdAt\" DESC").
		First(&verification)
	if result.Error != nil {
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
