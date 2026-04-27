package auth

import (
	"context"
	"errors"
	"regexp"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
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
	DeleteUser(embyUserID string) error
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
	Notifier                 authRegistrationNotifier
	EmailService             authEmailVerifier
	ConfigReader             authConfigReader
	TurnstileVerifier        authTurnstileVerifier
	NewEmbyClient            func() authEmbyClient
	SaveUser                 func(user *models.User) error
	GetRegistrationMode      func() string
	GetDefaultTrialDays      func() int
	ValidateRegistrationCode func(code string) (*models.RedemptionCode, error)
	Compensation             *accountpkg.EmbyCompensation
	NewCompensation          func() *accountpkg.EmbyCompensation
}

// AuthService 认证服务
type AuthService struct {
	notifier                 authRegistrationNotifier
	emailService             authEmailVerifier
	configReader             authConfigReader
	turnstileVerifier        authTurnstileVerifier
	newEmbyClient            func() authEmbyClient
	saveUser                 func(user *models.User) error
	getRegistrationMode      func() string
	getDefaultTrialDays      func() int
	validateRegistrationCode func(code string) (*models.RedemptionCode, error)
	compensation             *accountpkg.EmbyCompensation
	newCompensation          func() *accountpkg.EmbyCompensation
}

// NewAuthService 创建认证服务
func NewAuthService() *AuthService {
	return NewAuthServiceWithDeps(AuthServiceDeps{})
}

func NewAuthServiceWithDeps(deps AuthServiceDeps) *AuthService {
	defaultConfigReader := configpkg.NewConfigService()
	redemptionCodeService := &redemptionpkg.RedemptionCodeService{}

	service := &AuthService{
		notifier:                 deps.Notifier,
		emailService:             deps.EmailService,
		configReader:             deps.ConfigReader,
		turnstileVerifier:        deps.TurnstileVerifier,
		newEmbyClient:            deps.NewEmbyClient,
		saveUser:                 deps.SaveUser,
		getRegistrationMode:      deps.GetRegistrationMode,
		getDefaultTrialDays:      deps.GetDefaultTrialDays,
		validateRegistrationCode: deps.ValidateRegistrationCode,
		compensation:             deps.Compensation,
		newCompensation:          deps.NewCompensation,
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
			return db.DB.Save(user).Error
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
	if service.validateRegistrationCode == nil {
		service.validateRegistrationCode = func(code string) (*models.RedemptionCode, error) {
			return redemptionCodeService.ValidateRegistrationCode(code)
		}
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
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

func (s *AuthService) applyTemplatePolicyIfNeeded(newEmbyID string, templateUserID *string) error {
	if templateUserID == nil || *templateUserID == "" {
		return nil
	}

	var templateUser models.User
	if err := db.DB.Where("id = ?", *templateUserID).First(&templateUser).Error; err != nil {
		return errors.New("模板用户不存在")
	}
	if templateUser.EmbyID == "" {
		return errors.New("模板用户未关联 Emby 账号")
	}

	embyService := s.newEmbyClient()
	sourcePolicy, err := embyService.GetUserPolicyRaw(templateUser.EmbyID)
	if err != nil {
		return errors.New("读取模板用户权限失败")
	}

	// 模板用户 Policy 复制白名单（同时也是禁止复制清单的反义）。
	//
	// 显式禁止复制（即使模板用户上有也不带给新用户）：
	//   IsAdministrator / EnableUserPreferenceAccess / EnableContentDeletion /
	//   EnableContentDownloading / EnableLiveTvManagement / EnableLiveTvAccess /
	//   EnableMediaConversion / EnableSubtitleManagement / MaxParentalRating /
	//   BlockedTags / AllowedTags
	//
	// 这些字段如果允许复制：
	//   - IsAdministrator / 各类 Management：会让新用户拿到本不应该具有的管理面板能力
	//   - EnableContentDownloading：放开后用户可整剧打包下载，绕过 Ember 默认禁下载策略
	//   - MaxParentalRating / BlockedTags / AllowedTags：会按管理员预设把新用户限制 / 解锁到非预期评级范围
	whitelistFields := []string{
		"EnableAllFolders",
		"EnabledFolders",
		"ExcludedSubFolders",
		"EnableSyncTranscoding",
		"EnableVideoPlaybackTranscoding",
		"EnablePlaybackRemuxing",
		"EnableAudioPlaybackTranscoding",
	}
	if err := embyService.PatchUserPolicyFields(newEmbyID, sourcePolicy, whitelistFields); err != nil {
		return errors.New("应用模板用户权限失败")
	}

	return nil
}
