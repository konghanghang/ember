package services

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gopkg.in/gomail.v2"
)

const smtpTimeout = 10 * time.Second

// EmailService 邮件验证服务
type EmailService struct {
	host          string
	port          string
	username      string
	password      string
	from          string
	fromAddress   string
	expiryMinutes int
	dailyLimit    int
	ipDailyLimit  int
}

// NewEmailService 从环境变量初始化
func NewEmailService() *EmailService {
	service := &EmailService{}
	service.refreshConfig()
	return service
}

func (s *EmailService) refreshConfig() {
	configService := configpkg.NewConfigService()

	port := configService.GetString("SMTP_PORT")
	if port == "" {
		port = "587"
	}

	username := configService.GetString("SMTP_USERNAME")
	from := configService.GetString("SMTP_FROM")
	if from == "" {
		from = username
	}

	fromAddress := ""
	if from != "" {
		if addr, err := mail.ParseAddress(from); err == nil {
			fromAddress = addr.Address
		}
	}

	expiryMinutes := 10
	if v := configService.GetString("EMAIL_CODE_EXPIRY_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			expiryMinutes = n
		}
	}

	dailyLimit := 5
	if v := configService.GetString("EMAIL_CODE_DAILY_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dailyLimit = n
		}
	}

	ipDailyLimit := 15
	if v := configService.GetString("EMAIL_CODE_IP_DAILY_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ipDailyLimit = n
		}
	}

	s.host = strings.TrimSpace(configService.GetString("SMTP_HOST"))
	s.port = strings.TrimSpace(port)
	s.username = strings.TrimSpace(username)
	s.password = configService.GetString("SMTP_PASSWORD")
	s.from = strings.TrimSpace(from)
	s.fromAddress = fromAddress
	s.expiryMinutes = expiryMinutes
	s.dailyLimit = dailyLimit
	s.ipDailyLimit = ipDailyLimit
}

// IsConfigured 检查 SMTP 是否配置
func (s *EmailService) IsConfigured() bool {
	s.refreshConfig()
	return s.host != "" && s.username != "" && s.password != "" && s.fromAddress != ""
}

// IsEnabled 综合判断：SMTP 已配置 + 业务开关开启
func (s *EmailService) IsEnabled() bool {
	s.refreshConfig()
	if !s.IsConfigured() {
		return false
	}
	return configpkg.NewConfigService().IsEmailVerificationEnabled()
}

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

	// 先保存记录，再发送邮件（保证一致性：邮件发送失败时回滚）
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

// sendEmail 通过 SMTP 发送邮件（使用 gomail 生成 MIME，并设置全链路超时）
func (s *EmailService) sendEmail(to, subject, body string) error {
	s.refreshConfig()
	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	addr := net.JoinHostPort(s.host, s.port)
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return fmt.Errorf("connect SMTP %s: %w", addr, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return fmt.Errorf("SMTP STARTTLS: %w", err)
		}
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth: %w", err)
	}

	if err := client.Mail(s.fromAddress); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}

	if _, err := m.WriteTo(w); err != nil {
		w.Close()
		return fmt.Errorf("SMTP write body: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP close data: %w", err)
	}

	// 邮件主体发送完成即视为成功，QUIT 失败只记录日志不影响业务结果。
	if err := client.Quit(); err != nil {
		log.Printf("SMTP QUIT 失败 [%s]: %v", to, err)
	}

	return nil
}

func (s *EmailService) TestConnection() error {
	s.refreshConfig()
	if !s.IsConfigured() {
		return ErrEmailNotConfigured
	}
	return configpkg.TestSMTPDial(s.host, s.port)
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
