package email

import (
	"net/mail"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
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
