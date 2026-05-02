package email

import (
	"net/mail"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
)

const smtpTimeout = 10 * time.Second

type emailConfigReader interface {
	GetString(key string) string
	IsEmailVerificationEnabled() bool
	IsRegistrationEmailAllowed(email string) error
}

// EmailService 邮件验证服务
type EmailService struct {
	configReader  emailConfigReader
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

func (s *EmailService) hasSMTPConfig() bool {
	return s.host != "" && s.username != "" && s.password != "" && s.fromAddress != ""
}

// NewEmailService 从环境变量初始化
func NewEmailService() *EmailService {
	return NewEmailServiceWithConfig(configpkg.NewConfigService())
}

func NewEmailServiceWithConfig(reader emailConfigReader) *EmailService {
	if reader == nil {
		reader = configpkg.NewConfigService()
	}
	service := &EmailService{
		configReader: reader,
	}
	service.refreshConfig()
	return service
}

func (s *EmailService) refreshConfig() {
	configService := s.configReader
	if configService == nil {
		configService = configpkg.NewConfigService()
		s.configReader = configService
	}

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
	return s.hasSMTPConfig()
}

// IsEnabled 综合判断：SMTP 已配置 + 业务开关开启
func (s *EmailService) IsEnabled() bool {
	s.refreshConfig()
	if !s.IsConfigured() {
		return false
	}
	return s.configReader.IsEmailVerificationEnabled()
}

// IsRegistrationEmailAllowed 暴露给上层（如 UserService）复用的注册邮箱域名白名单门控。
// 与 SendVerificationCode(register) / SendEmailChangeCode 共用 ConfigService 同一份语义，
// 让"邮箱业务策略"在 EmailService 这一层收口，避免上层直接依赖 ConfigService。
func (s *EmailService) IsRegistrationEmailAllowed(email string) error {
	return s.configReader.IsRegistrationEmailAllowed(email)
}
