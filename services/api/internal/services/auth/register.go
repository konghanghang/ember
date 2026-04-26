package auth

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
)

type registerPreparation struct {
	mode                  string
	defaultDays           int
	registrationPlanGroup *string
	redemptionCode        *models.RedemptionCode
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

	embyService := s.newEmbyClient()
	embyUser, err := embyService.CreateEmbyUser(req.Username, req.Password)
	if err != nil {
		return nil, errors.New("创建 Emby 用户失败：" + err.Error())
	}

	user, err := s.persistRegisteredUser(req, prepared, embyUser)
	if err != nil {
		s.rollbackEmbyRegistration(embyService, embyUser.ID)
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
	mode := s.currentRegistrationMode()

	prepared := &registerPreparation{
		mode: mode,
	}
	if mode != "invite" {
		prepared.defaultDays = s.currentDefaultTrialDays()
		return prepared, nil
	}

	if req.Code == "" {
		return nil, errors.New("当前为邀请注册模式，请提供兑换码")
	}

	redemptionCode, err := s.validateInviteRegistrationCode(req.Code)
	if err != nil {
		return nil, err
	}
	prepared.redemptionCode = redemptionCode
	prepared.defaultDays = redemptionCode.DefaultDays
	prepared.registrationPlanGroup = redemptionCode.RegistrationPlanGroup
	if prepared.registrationPlanGroup != nil {
		log.Printf("[Auth] 邀请注册使用显式套餐分组: codeID=%s planGroup=%s", redemptionCode.ID, *prepared.registrationPlanGroup)
	}
	return prepared, nil
}

func (s *AuthService) ensureRegisterUserUnique(req *RegisterUserRequest) error {
	var existingUser models.User
	result := db.DB.Where("lower(username) = ?", strings.ToLower(req.Username)).First(&existingUser)
	if result.Error == nil {
		return errors.New("用户名已存在")
	}

	var existingEmail models.User
	result = db.DB.Where("lower(email) = ?", strings.ToLower(req.Email)).First(&existingEmail)
	if result.Error == nil {
		return errors.New("邮箱已被注册")
	}

	return nil
}

// rollbackEmbyRegistration 在本地落库失败时清理 Emby 端账号；DeleteUser 失败时
// 入补偿队列，由 cron @every 10m 重试，避免 Emby 端孤儿账号永久阻塞同名注册。
//
// embyClient 接口要求带 DeleteUser 方法（Emby 端 404 已视作幂等成功，无需特殊处理）。
func (s *AuthService) rollbackEmbyRegistration(embyService authEmbyClient, embyUserID string) {
	embyUserID = strings.TrimSpace(embyUserID)
	if embyUserID == "" {
		return
	}
	if err := embyService.DeleteUser(embyUserID); err == nil {
		return
	} else {
		log.Printf("[Auth] Emby 注册回滚失败 embyUserId=%s err=%v；尝试入补偿队列", embyUserID, err)
	}

	compensation := s.compensationQueue()
	if compensation == nil {
		log.Printf("[Auth] 补偿队列未配置，Emby 账号残留 embyUserId=%s", embyUserID)
		return
	}
	if err := compensation.Enqueue(context.Background(), models.FailedEmbyAsyncOp{
		Origin:      models.FailedEmbyOriginRegisterCleanup,
		OriginRefID: embyUserID,
		EmbyUserID:  embyUserID,
		Action:      models.FailedEmbyActionDelete,
	}); err != nil {
		log.Printf("[Auth] 补偿队列入队失败 embyUserId=%s err=%v", embyUserID, err)
	}
}

func (s *AuthService) compensationQueue() *accountpkg.EmbyCompensation {
	if s.compensation != nil {
		return s.compensation
	}
	s.compensation = accountpkg.NewEmbyCompensation(embyint.GetSharedService())
	return s.compensation
}
