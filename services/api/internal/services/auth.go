package services

import (
	"errors"
	"log"
	"regexp"
	"strings"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// AuthService 认证服务
type AuthService struct {
	notifier     *BotNotifier
	emailService *EmailService
}

// NewAuthService 创建认证服务
func NewAuthService() *AuthService {
	return &AuthService{
		notifier:     NewBotNotifier(),
		emailService: NewEmailService(),
	}
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// LoginRequest 统一登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 统一登录响应
type LoginResponse struct {
	Token     string       `json:"token"`
	User      *models.User `json:"user"`
	IsExpired bool         `json:"isExpired"`
}

func (s *AuthService) Login(req *LoginRequest) (*LoginResponse, error) {
	var user models.User
	result := db.DB.Where("username = ?", req.Username).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户名或密码错误")
	}

	var authenticated bool

	if user.IsAdmin() {
		authenticated = user.CheckPassword(req.Password)
	} else {
		embyService := NewEmbyService()
		embyUser, err := embyService.AuthenticateUser(user.Username, req.Password)
		if err == nil && embyUser.ID == user.EmbyID {
			authenticated = true
		} else if user.Password != "" && user.CheckPassword(req.Password) {
			// 兜底：允许历史本地密码登录，并尝试同步回 Emby
			authenticated = true
			if user.EmbyID != "" {
				if syncErr := embyService.UpdateUserPassword(user.EmbyID, req.Password); syncErr != nil {
					log.Printf("⚠️  登录时同步 Emby 密码失败：userID=%s, err=%v", user.ID, syncErr)
				}
			}
		} else {
			return nil, errors.New("用户名或密码错误")
		}

		// 登录成功后，确保本地哈希与当前密码一致
		if !user.CheckPassword(req.Password) {
			if err := user.SetPassword(req.Password); err != nil {
				log.Printf("⚠️  登录时更新本地密码哈希失败（不影响登录）：userID=%s, err=%v", user.ID, err)
			} else if err := db.DB.Save(&user).Error; err != nil {
				log.Printf("⚠️  登录时保存本地密码哈希失败（不影响登录）：userID=%s, err=%v", user.ID, err)
			}
		}
	}

	if !authenticated {
		return nil, errors.New("用户名或密码错误")
	}

	token, err := common.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &LoginResponse{
		Token:     token,
		User:      &user,
		IsExpired: user.IsExpired(),
	}, nil
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

func (s *AuthService) RegisterUser(req *RegisterUserRequest) (*RegisterUserResponse, error) {
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Username) > 50 {
		return nil, errors.New("用户名长度必须为 3-50 位")
	}
	if !usernamePattern.MatchString(req.Username) {
		return nil, errors.New("用户名只能包含字母和数字")
	}

	// 邮箱验证码校验（仅在功能启用时生效）
	if s.emailService.IsEnabled() {
		if req.EmailCode == "" {
			return nil, errors.New("请先获取邮箱验证码")
		}
		if err := s.emailService.VerifyCode(req.Email, req.EmailCode); err != nil {
			return nil, err
		}
	}

	settingService := &SettingService{}
	mode := settingService.GetRegistrationMode()

	var defaultDays int
	var redemptionCode *models.RedemptionCode

	if mode == "invite" {
		if req.Code == "" {
			return nil, errors.New("当前为邀请注册模式，请提供兑换码")
		}
		codeService := &RedemptionCodeService{}
		var err error
		redemptionCode, err = codeService.ValidateCode(req.Code)
		if err != nil {
			return nil, err
		}
		defaultDays = redemptionCode.DefaultDays
	} else {
		defaultDays = settingService.GetDefaultTrialDays()
	}

	var existingUser models.User
	result := db.DB.Where("username = ?", req.Username).First(&existingUser)
	if result.Error == nil {
		return nil, errors.New("用户名已存在")
	}

	var existingEmail models.User
	result = db.DB.Where("email = ?", req.Email).First(&existingEmail)
	if result.Error == nil {
		return nil, errors.New("邮箱已被注册")
	}

	embyService := NewEmbyService()
	embyUser, err := embyService.CreateEmbyUser(req.Username, req.Password)
	if err != nil {
		return nil, errors.New("创建 Emby 用户失败：" + err.Error())
	}

	expiresAt := common.CalculateExpiryDate(defaultDays)

	user := models.User{
		Username:  req.Username,
		Role:      "user",
		Email:     req.Email,
		EmbyID:    embyUser.ID,
		ExpiresAt: &expiresAt,
		IsActive:  true,
	}
	if err := user.SetPassword(req.Password); err != nil {
		return nil, errors.New("创建用户失败")
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("创建用户失败")
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("创建用户失败")
	}

	if mode == "invite" && redemptionCode != nil {
		result := tx.Model(&models.RedemptionCode{}).
			Where("code = ? AND \"usedCount\" < \"maxUses\"", req.Code).
			Update("usedCount", gorm.Expr("\"usedCount\" + 1"))
		if result.Error != nil {
			tx.Rollback()
			return nil, errors.New("创建用户失败")
		}
		if result.RowsAffected == 0 {
			tx.Rollback()
			return nil, ErrRedemptionCodeInvalid
		}

		redemption := models.Redemption{
			UserID: user.ID,
			Code:   req.Code,
			Days:   defaultDays,
		}
		if err := tx.Create(&redemption).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("创建用户失败")
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("创建用户失败")
	}

	// 通知 Bot（异步 fire-and-forget，失败不影响注册成功）
	go func(user models.User, mode string) {
		if s.notifier == nil || !s.notifier.IsConfigured() {
			return
		}

		var expiresAt *string
		if user.ExpiresAt != nil {
			formatted := user.ExpiresAt.UTC().Format("2006-01-02 15:04:05 MST")
			expiresAt = &formatted
		}

		s.notifier.NotifyNewRegistration(RegistrationNotification{
			ID:               user.ID,
			UserName:         user.Username,
			Email:            user.Email,
			EmbyID:           user.EmbyID,
			RegistrationMode: mode,
			ExpiresAt:        expiresAt,
		})
	}(user, mode)

	token, err := common.GenerateToken(user.ID, user.Username, "user")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &RegisterUserResponse{
		Token: token,
		User:  &user,
	}, nil
}

// GetCurrentUser 获取当前用户信息
func (s *AuthService) GetCurrentUser(userID string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}
	return &user, nil
}
