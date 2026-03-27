package auth

import (
	"errors"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	"regexp"
)

type authEmailVerifier interface {
	IsEnabled() bool
	VerifyCode(email, code, codeType string) error
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

// AuthService 认证服务
type AuthService struct {
	notifier      authRegistrationNotifier
	emailService  authEmailVerifier
	newEmbyClient func() authEmbyClient
	saveUser      func(user *models.User) error
}

// NewAuthService 创建认证服务
func NewAuthService() *AuthService {
	return &AuthService{
		notifier:      notifierint.NewBotNotifier(),
		emailService:  emailpkg.NewEmailService(),
		newEmbyClient: func() authEmbyClient { return embyint.NewEmbyService() },
		saveUser: func(user *models.User) error {
			return db.DB.Save(user).Error
		},
	}
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

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

	whitelistFields := []string{
		"EnableAllFolders",
		"EnabledFolders",
		"ExcludedSubFolders",
		"EnableContentDownloading",
		"EnableSyncTranscoding",
		"EnableVideoPlaybackTranscoding",
		"EnablePlaybackRemuxing",
		"EnableAudioPlaybackTranscoding",
		"MaxParentalRating",
	}
	if err := embyService.PatchUserPolicyFields(newEmbyID, sourcePolicy, whitelistFields); err != nil {
		return errors.New("应用模板用户权限失败")
	}

	return nil
}
