package auth

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	userpkg "github.com/konghang/ember/backend/internal/services/user"
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

	if err := s.enforceRegisterEmailDomain(req); err != nil {
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

	user, policySyncStatus, err := s.persistRegisteredUser(req, prepared, embyUser)
	if err != nil {
		s.rollbackEmbyRegistration(embyService, embyUser.ID)
		return nil, err
	}

	s.notifyNewRegistration(*user, prepared.mode)

	token, err := common.GenerateToken(user.ID, user.Username, "user", common.ComputePasswordSignature(user.Password))
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &RegisterUserResponse{
		Token:            token,
		User:             user,
		PolicySyncStatus: policySyncStatus,
	}, nil
}

func (s *AuthService) validateRegisterRequest(req *RegisterUserRequest) error {
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.EmailCode = strings.TrimSpace(req.EmailCode)
	req.Code = strings.TrimSpace(req.Code)
	if len(req.Username) < 3 || len(req.Username) > 50 {
		return userpkg.ErrUsernameLengthInvalid
	}
	if !usernamePattern.MatchString(req.Username) {
		return userpkg.ErrUsernameCharsetInvalid
	}
	return nil
}

func (s *AuthService) verifyRegisterEmailCode(req *RegisterUserRequest) error {
	if !s.emailService.IsEnabled() {
		return nil
	}
	if req.EmailCode == "" {
		return ErrRegisterEmailCodeRequired
	}
	return s.emailService.CheckCode(req.Email, req.EmailCode, models.VerificationTypeRegister)
}

// enforceRegisterEmailDomain 在用户名整理完成后、邮件码校验之前做注册邮箱域名白名单门控。
//
// 与 SendVerificationCode 共用同一份 ConfigService.IsRegistrationEmailAllowed 语义，
// 避免“验证码能发、注册被拦”的裂缝。失败路径不调用 Emby、不消耗邀请码、不写库。
//
// 当 isRegistrationEmailAllowed 未注入时（理论上不会发生，构造期已默认填充），
// 视作白名单关闭，保持当前开放注册行为，不会误把所有人挡在外面。
func (s *AuthService) enforceRegisterEmailDomain(req *RegisterUserRequest) error {
	if s.isRegistrationEmailAllowed == nil {
		return nil
	}
	if err := s.isRegistrationEmailAllowed(req.Email); err != nil {
		log.Printf("[Auth] 注册因邮箱域名白名单拦截 username=%s domain=%s reason=%v",
			req.Username, registrationEmailDomain(req.Email), err)
		return err
	}
	return nil
}

// registrationEmailDomain 仅用于注册链路日志，从原始请求邮箱中提取小写域名。
// 输入异常时返回 "unknown"，不要让日志格式化失败掩盖真实拦截原因。
func registrationEmailDomain(email string) string {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(trimmed, "@")
	if at <= 0 || at >= len(trimmed)-1 {
		return "unknown"
	}
	return trimmed[at+1:]
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
		return nil, ErrRegisterInviteCodeRequired
	}

	redemptionCode, err := s.validateInviteRegistrationCode(req.Code)
	if err != nil {
		return nil, err
	}
	prepared.redemptionCode = redemptionCode
	prepared.defaultDays = redemptionCode.DefaultDays
	registrationPlanGroup := redemptionCode.RegistrationPlanGroup
	prepared.registrationPlanGroup = &registrationPlanGroup
	log.Printf("[Auth] 邀请注册使用显式套餐分组: codeID=%s planGroup=%s", redemptionCode.ID, registrationPlanGroup)
	return prepared, nil
}

func (s *AuthService) ensureRegisterUserUnique(req *RegisterUserRequest) error {
	var existingUser models.User
	result := db.DB.Where("lower(username) = ?", strings.ToLower(req.Username)).First(&existingUser)
	if result.Error == nil {
		return userpkg.ErrUsernameAlreadyExists
	}

	var existingEmail models.User
	result = db.DB.Where("lower(email) = ?", strings.ToLower(req.Email)).First(&existingEmail)
	if result.Error == nil {
		return emailpkg.ErrEmailAlreadyRegistered
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
	s.compensation = s.newCompensation()
	return s.compensation
}
