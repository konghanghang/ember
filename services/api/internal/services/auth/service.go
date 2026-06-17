package auth

import (
	"context"
	"errors"
	"regexp"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	"gorm.io/gorm"
)

type authEmailVerifier interface {
	IsEnabled() bool
	CheckCode(email, code, codeType string) error
	ConsumeCodeTx(tx *gorm.DB, email, code, codeType string) error
}

type authRegistrationNotifier interface {
	IsConfigured() bool
	NotifyNewRegistration(data notifierint.RegistrationNotification)
}

type authEmbyClient interface {
	AuthenticateUser(username, password string) (*embyint.EmbyUser, error)
	UpdateUserPassword(embyUserID, newPassword string) error
	CreateEmbyUser(username, password string) (*embyint.EmbyUser, error)
	CreateEmbyUserWithInitialDisabled(username, password string, initialDisabled bool) (*embyint.EmbyUser, error)
	DeleteUser(embyUserID string) error
	GetUsers() ([]embyint.EmbyUser, error)
	GetUserByID(embyUserID string) (*embyint.EmbyUser, error)
	GetUserPolicyRaw(embyUserID string) (map[string]any, error)
	PatchUserPolicyFields(targetUserID string, sourcePolicy map[string]any, fields []string) error
}

type authConfigReader interface {
	GetString(key string) string
}

type authTurnstileVerifier interface {
	VerifyLogin(ctx context.Context, payload TurnstileVerifyPayload) error
}

type AuthServiceDeps struct {
	Notifier                   authRegistrationNotifier
	EmailService               authEmailVerifier
	ConfigReader               authConfigReader
	TurnstileVerifier          authTurnstileVerifier
	NewEmbyClient              func() authEmbyClient
	SaveUser                   func(user *models.User) error
	GetRegistrationMode        func() string
	GetDefaultTrialDays        func() int
	GetDefaultPlanGroup        func() (*models.PlanGroup, error)
	IsRegistrationEmailAllowed func(email string) error
	ValidateRegistrationCode   func(code string) (*models.RedemptionCode, error)
	FindLoginUser              func(username string) (*models.User, error)
	Compensation               *accountpkg.EmbyCompensation
	NewCompensation            func() *accountpkg.EmbyCompensation
}

// AuthService 认证服务
type AuthService struct {
	notifier                   authRegistrationNotifier
	emailService               authEmailVerifier
	configReader               authConfigReader
	turnstileVerifier          authTurnstileVerifier
	newEmbyClient              func() authEmbyClient
	saveUser                   func(user *models.User) error
	getRegistrationMode        func() string
	getDefaultTrialDays        func() int
	getDefaultPlanGroup        func() (*models.PlanGroup, error)
	isRegistrationEmailAllowed func(email string) error
	validateRegistrationCode   func(code string) (*models.RedemptionCode, error)
	findLoginUserFn            func(username string) (*models.User, error)
	compensation               *accountpkg.EmbyCompensation
	newCompensation            func() *accountpkg.EmbyCompensation

	// Emby 绑定 hook：默认走 db.DB，测试时由 newAuthServiceForBinding 注入。
	findUserByIDForBindingFn func(userID string) (*models.User, error)
	findOccupyingUserFn      func(embyID, excludeUserID string) (*models.User, error)
	findUsersByEmbyIDsFn     func(embyIDs []string) ([]models.User, error)
	updateUserEmbyIDFn       func(userID, embyID string) error
}

var (
	ErrAuthInvalidCredentials     = errors.New("用户名或密码错误")
	ErrRegisterEmailCodeRequired  = errors.New("请先获取邮箱验证码")
	ErrRegisterInviteCodeRequired = errors.New("当前为邀请注册模式，请提供兑换码")
)

// NewAuthService 创建认证服务
func NewAuthService() *AuthService {
	return NewAuthServiceWithDeps(AuthServiceDeps{})
}

func NewAuthServiceWithDeps(deps AuthServiceDeps) *AuthService {
	defaultConfigReader := configpkg.NewConfigService()
	redemptionCodeService := &redemptionpkg.RedemptionCodeService{}

	service := &AuthService{
		notifier:                   deps.Notifier,
		emailService:               deps.EmailService,
		configReader:               deps.ConfigReader,
		turnstileVerifier:          deps.TurnstileVerifier,
		newEmbyClient:              deps.NewEmbyClient,
		saveUser:                   deps.SaveUser,
		getRegistrationMode:        deps.GetRegistrationMode,
		getDefaultTrialDays:        deps.GetDefaultTrialDays,
		getDefaultPlanGroup:        deps.GetDefaultPlanGroup,
		isRegistrationEmailAllowed: deps.IsRegistrationEmailAllowed,
		validateRegistrationCode:   deps.ValidateRegistrationCode,
		findLoginUserFn:            deps.FindLoginUser,
		compensation:               deps.Compensation,
		newCompensation:            deps.NewCompensation,
	}

	if service.notifier == nil {
		service.notifier = notifierint.GetSharedBotNotifier()
	}
	if service.emailService == nil {
		service.emailService = emailpkg.NewEmailService()
	}
	if service.configReader == nil {
		service.configReader = defaultConfigReader
	}
	if service.turnstileVerifier == nil {
		service.turnstileVerifier = NewCloudflareTurnstileVerifier()
	}
	if service.newEmbyClient == nil {
		service.newEmbyClient = func() authEmbyClient { return embyint.GetSharedService() }
	}
	if service.saveUser == nil {
		service.saveUser = func(user *models.User) error {
			return db.DB.Model(&models.User{}).
				Where("id = ?", user.ID).
				Select("password", "updated_at").
				Updates(map[string]any{
					"password":   user.Password,
					"updated_at": time.Now(),
				}).Error
		}
	}
	if service.getRegistrationMode == nil {
		service.getRegistrationMode = func() string {
			return defaultConfigReader.GetRegistrationMode()
		}
	}
	if service.getDefaultTrialDays == nil {
		service.getDefaultTrialDays = func() int {
			return defaultConfigReader.GetDefaultTrialDays()
		}
	}
	if service.getDefaultPlanGroup == nil {
		service.getDefaultPlanGroup = func() (*models.PlanGroup, error) {
			return paymentpkg.GetDefaultPlanGroup(nil)
		}
	}
	if service.isRegistrationEmailAllowed == nil {
		service.isRegistrationEmailAllowed = func(email string) error {
			return defaultConfigReader.IsRegistrationEmailAllowed(email)
		}
	}
	if service.validateRegistrationCode == nil {
		service.validateRegistrationCode = func(code string) (*models.RedemptionCode, error) {
			return redemptionCodeService.ValidateRegistrationCode(code)
		}
	}
	if service.findLoginUserFn == nil {
		service.findLoginUserFn = service.findLoginUserFromDB
	}
	if service.newCompensation == nil {
		service.newCompensation = func() *accountpkg.EmbyCompensation {
			return accountpkg.NewEmbyCompensation(embyint.GetSharedService())
		}
	}
	return service
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func (s *AuthService) currentRegistrationMode() string {
	return s.getRegistrationMode()
}

func (s *AuthService) currentDefaultTrialDays() int {
	return s.getDefaultTrialDays()
}

func (s *AuthService) validateInviteRegistrationCode(code string) (*models.RedemptionCode, error) {
	return s.validateRegistrationCode(code)
}

// RegisterUserRequest 用户注册请求
type RegisterUserRequest struct {
	Username  string `json:"username" binding:"required,min=3,max=50"`
	Password  string `json:"password" binding:"required,min=6"`
	Email     string `json:"email" binding:"required,email"`
	Code      string `json:"code"`      // 兑换码（invite 模式必填，open 模式忽略）
	EmailCode string `json:"emailCode"` // 邮箱验证码（启用时必填）
}

// RegisterUserResponse 用户注册响应
type RegisterUserResponse struct {
	Token            string       `json:"token"`
	User             *models.User `json:"user"`
	PolicySyncStatus string       `json:"policySyncStatus,omitempty"`
}
