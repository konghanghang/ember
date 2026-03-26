package services

import (
	"errors"
	"strings"

	"github.com/konghang/ember/backend/internal/common"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
)

type registerPreparation struct {
	mode           string
	defaultDays    int
	redemptionCode *models.RedemptionCode
}

func (s *AuthService) RegisterUser(req *RegisterUserRequest) (*RegisterUserResponse, error) {
	if err := s.validateRegisterRequest(req); err != nil {
		return nil, err
	}

	if err := s.verifyRegisterEmailCode(req); err != nil {
		return nil, err
	}

	prepared, err := s.prepareRegister(req)
	if err != nil {
		return nil, err
	}

	if err := s.ensureRegisterUserUnique(req); err != nil {
		return nil, err
	}

	embyService := embyint.NewEmbyService()
	embyUser, err := embyService.CreateEmbyUser(req.Username, req.Password)
	if err != nil {
		return nil, errors.New("创建 Emby 用户失败：" + err.Error())
	}

	user, err := s.persistRegisteredUser(req, prepared, embyUser)
	if err != nil {
		_ = embyService.DeleteUser(embyUser.ID)
		return nil, err
	}

	s.notifyNewRegistration(*user, prepared.mode)

	token, err := common.GenerateToken(user.ID, user.Username, "user")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &RegisterUserResponse{
		Token: token,
		User:  user,
	}, nil
}

func (s *AuthService) validateRegisterRequest(req *RegisterUserRequest) error {
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Username) > 50 {
		return errors.New("用户名长度必须为 3-50 位")
	}
	if !usernamePattern.MatchString(req.Username) {
		return errors.New("用户名只能包含字母和数字")
	}
	return nil
}

func (s *AuthService) verifyRegisterEmailCode(req *RegisterUserRequest) error {
	if !s.emailService.IsEnabled() {
		return nil
	}
	if req.EmailCode == "" {
		return errors.New("请先获取邮箱验证码")
	}
	return s.emailService.VerifyCode(req.Email, req.EmailCode, models.VerificationTypeRegister)
}

func (s *AuthService) prepareRegister(req *RegisterUserRequest) (*registerPreparation, error) {
	configService := configpkg.NewConfigService()
	mode := configService.GetRegistrationMode()

	prepared := &registerPreparation{
		mode: mode,
	}
	if mode != "invite" {
		prepared.defaultDays = configService.GetDefaultTrialDays()
		return prepared, nil
	}

	if req.Code == "" {
		return nil, errors.New("当前为邀请注册模式，请提供兑换码")
	}

	codeService := &redemptionpkg.RedemptionCodeService{}
	redemptionCode, err := codeService.ValidateCode(req.Code)
	if err != nil {
		return nil, err
	}
	prepared.redemptionCode = redemptionCode
	prepared.defaultDays = redemptionCode.DefaultDays
	return prepared, nil
}

func (s *AuthService) ensureRegisterUserUnique(req *RegisterUserRequest) error {
	var existingUser models.User
	result := db.DB.Where("username = ?", req.Username).First(&existingUser)
	if result.Error == nil {
		return errors.New("用户名已存在")
	}

	var existingEmail models.User
	result = db.DB.Where("email = ?", req.Email).First(&existingEmail)
	if result.Error == nil {
		return errors.New("邮箱已被注册")
	}

	return nil
}
