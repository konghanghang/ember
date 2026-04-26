package user

import (
	"context"
	"errors"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/gorm"
)

type AdminCreateUserRequest struct {
	Username    string  `json:"username" binding:"required"`
	Email       string  `json:"email" binding:"required"`
	Password    string  `json:"password" binding:"required"`
	PlanGroup   string  `json:"planGroup" binding:"required"`
	ExpiresAt   *string `json:"expiresAt"`
	NeverExpire bool    `json:"neverExpire"`
}

func (s *UserService) CreateUserByAdmin(req *AdminCreateUserRequest) (*UserView, error) {
	s.setDefaults()

	if req == nil {
		return nil, errors.New("请求参数错误")
	}

	username, err := validateAdminCreateUsername(req.Username)
	if err != nil {
		return nil, err
	}
	email, err := validateAdminCreateEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if len(req.Password) < 6 {
		return nil, errors.New("密码长度不能小于 6 位")
	}

	planGroupKey, err := normalizePlanGroupStrict(req.PlanGroup)
	if err != nil {
		return nil, err
	}
	planGroup, err := s.getPlanGroupByKey(planGroupKey)
	if err != nil {
		return nil, err
	}

	expiresAt, err := parseAdminCreateExpiresAt(req)
	if err != nil {
		return nil, err
	}

	if err := s.ensureAdminCreatedUserUnique(username, email); err != nil {
		return nil, err
	}

	log.Printf("[User] 后台创建用户开始: username=%s email=%s planGroup=%s neverExpire=%t expiresAt=%s",
		username, email, planGroup.Key, req.NeverExpire, formatAdminCreateExpiryForLog(expiresAt))

	embyService := s.newEmbyClient()
	embyUser, err := embyService.CreateEmbyUser(username, req.Password)
	if err != nil {
		return nil, errors.New("创建 Emby 用户失败：" + err.Error())
	}

	log.Printf("[User] 后台创建 Emby 用户成功: username=%s embyId=%s", username, embyUser.ID)

	user := &models.User{
		Username:     username,
		Role:         "user",
		Email:        email,
		EmbyID:       embyUser.ID,
		EmbyDisabled: false,
		PlanGroup:    &planGroup.Key,
		ExpiresAt:    expiresAt,
		IsActive:     true,
	}
	if err := user.SetPassword(req.Password); err != nil {
		s.cleanupAdminCreatedEmbyUser(embyService, embyUser.ID, username)
		return nil, errors.New("创建用户失败")
	}

	if err := s.syncEmbyPolicy(user); err != nil {
		s.cleanupAdminCreatedEmbyUser(embyService, embyUser.ID, username)
		return nil, err
	}

	if err := s.createUser(user); err != nil {
		s.cleanupAdminCreatedEmbyUser(embyService, embyUser.ID, username)
		return nil, mapAdminCreatePersistenceError(err)
	}

	log.Printf("[User] 后台创建用户成功: userID=%s username=%s embyId=%s planGroup=%s neverExpire=%t expiresAt=%s",
		user.ID, user.Username, user.EmbyID, planGroup.Key, req.NeverExpire, formatAdminCreateExpiryForLog(user.ExpiresAt))

	planGroupName := planGroup.Name
	return &UserView{
		User:                   *user,
		PlanGroupName:          &planGroupName,
		EffectivePlanGroup:     planGroup.Key,
		EffectivePlanGroupName: planGroup.Name,
		IsPlanGroupMissing:     false,
	}, nil
}

func validateAdminCreateUsername(raw string) (string, error) {
	username := strings.TrimSpace(raw)
	if len(username) < 3 || len(username) > 50 {
		return "", errors.New("用户名长度必须为 3-50 位")
	}
	for _, r := range username {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return "", errors.New("用户名只能包含字母和数字")
		}
	}
	return username, nil
}

func validateAdminCreateEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", errors.New("邮箱不能为空")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return "", errors.New("邮箱格式错误")
	}
	return parsed.Address, nil
}

func parseAdminCreateExpiresAt(req *AdminCreateUserRequest) (*time.Time, error) {
	if req.NeverExpire {
		if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
			return nil, errors.New("neverExpire=true 时不能再传 expiresAt")
		}
		return nil, nil
	}

	if req.ExpiresAt == nil || strings.TrimSpace(*req.ExpiresAt) == "" {
		return nil, errors.New("neverExpire=false 时必须传 expiresAt")
	}

	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresAt))
	if err != nil {
		return nil, errors.New("expiresAt 必须是 RFC3339 格式")
	}
	expiresAtUTC := expiresAt.UTC()
	return &expiresAtUTC, nil
}

func (s *UserService) ensureAdminCreatedUserUnique(username, email string) error {
	if _, err := s.findUserByUsername(username); err == nil {
		return errors.New("用户名已存在")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("创建用户失败")
	}

	if _, err := s.findUserByEmail(email); err == nil {
		return errors.New("邮箱已存在")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("创建用户失败")
	}

	return nil
}

func (s *UserService) cleanupAdminCreatedEmbyUser(client embyClient, embyUserID, username string) {
	if client == nil || embyUserID == "" {
		return
	}
	if err := client.DeleteUser(embyUserID); err == nil {
		log.Printf("[User] 后台创建用户回滚完成: username=%s embyId=%s", username, embyUserID)
		return
	} else {
		log.Printf("[User] 后台创建用户回滚失败: username=%s embyId=%s error=%v；尝试入补偿队列", username, embyUserID, err)
	}

	compensation := s.compensationQueue()
	if compensation == nil {
		log.Printf("[User] 补偿队列未配置，Emby 账号残留: username=%s embyId=%s", username, embyUserID)
		return
	}
	if err := compensation.Enqueue(context.Background(), models.FailedEmbyAsyncOp{
		Origin:      models.FailedEmbyOriginRegisterCleanup,
		OriginRefID: embyUserID,
		EmbyUserID:  embyUserID,
		Action:      models.FailedEmbyActionDelete,
	}); err != nil {
		log.Printf("[User] 补偿队列入队失败 username=%s embyId=%s err=%v", username, embyUserID, err)
	}
}

func (s *UserService) compensationQueue() *accountpkg.EmbyCompensation {
	if s.compensation != nil {
		return s.compensation
	}
	s.compensation = accountpkg.NewEmbyCompensation(nil)
	return s.compensation
}

func mapAdminCreatePersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "duplicate key value") {
		switch {
		case strings.Contains(err.Error(), "username"):
			return errors.New("用户名已存在")
		case strings.Contains(err.Error(), "email"):
			return errors.New("邮箱已存在")
		}
	}
	if errors.Is(err, paymentpkg.ErrPlanGroupNotFound) || errors.Is(err, paymentpkg.ErrPlanGroupInvalid) {
		return err
	}
	return errors.New("创建用户失败")
}

func formatAdminCreateExpiryForLog(expiresAt *time.Time) string {
	if expiresAt == nil {
		return "never"
	}
	return expiresAt.UTC().Format(time.RFC3339)
}
