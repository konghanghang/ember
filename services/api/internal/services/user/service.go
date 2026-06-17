package user

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	policypkg "github.com/konghang/ember/backend/internal/services/policy"
	"gorm.io/gorm"
)

type emailVerifier interface {
	VerifyCode(email, code, codeType string) error
	CheckCode(email, code, codeType string) error
	ConsumeCodeTx(tx *gorm.DB, email, code, codeType string) error
	IsRegistrationEmailAllowed(email string) error
}

type embyClient interface {
	AuthenticateUser(username, password string) (*embyint.EmbyUser, error)
	UpdateUserPassword(embyUserID, newPassword string) error
	CreateEmbyUser(username, password string) (*embyint.EmbyUser, error)
	GetUserPolicyRaw(embyUserID string) (map[string]any, error)
	PatchUserPolicyFields(targetUserID string, sourcePolicy map[string]any, fields []string) error
	DeleteUser(embyUserID string) error
}

type UserServiceDeps struct {
	EmailVerifier       emailVerifier
	NewEmbyClient       func() embyClient
	FindUserByID        func(userID string) (*models.User, error)
	FindUserByUsername  func(username string) (*models.User, error)
	FindUserByEmail     func(email string) (*models.User, error)
	CreateUser          func(user *models.User) error
	GetPlanGroupByKey   func(key string) (*models.PlanGroup, error)
	SaveUser            func(user *models.User) error
	DeleteUserRecord    func(user *models.User) error
	UpdateUserActive    func(userID string, isActive bool) error
	GetUserViewByID     func(userID string) (*UserView, error)
	UpdateEmailWithCode func(userID, newEmail, code string) error
	Compensation        *accountpkg.EmbyCompensation
	NewCompensation     func() *accountpkg.EmbyCompensation
	ApplyPolicy         func(userID, reason string) error
	RecordPolicyFailure func(userID, reason string, cause error) error
}

// UserService 用户服务
type UserService struct {
	emailVerifier       emailVerifier
	newEmbyClient       func() embyClient
	findUserByID        func(userID string) (*models.User, error)
	findUserByUsername  func(username string) (*models.User, error)
	findUserByEmail     func(email string) (*models.User, error)
	createUser          func(user *models.User) error
	getPlanGroupByKey   func(key string) (*models.PlanGroup, error)
	saveUser            func(user *models.User) error
	deleteUserRecord    func(user *models.User) error
	updateUserActive    func(userID string, isActive bool) error
	getUserViewByID     func(userID string) (*UserView, error)
	updateEmailWithCode func(userID, newEmail, code string) error
	compensation        *accountpkg.EmbyCompensation
	newCompensation     func() *accountpkg.EmbyCompensation
	applyPolicy         func(userID, reason string) error
	recordPolicyFailure func(userID, reason string, cause error) error
}

func NewUserService() *UserService {
	return NewUserServiceWithDeps(UserServiceDeps{})
}

func NewUserServiceWithEmailVerifier(verifier emailVerifier) *UserService {
	return NewUserServiceWithDeps(UserServiceDeps{EmailVerifier: verifier})
}

func NewUserServiceWithDeps(deps UserServiceDeps) *UserService {
	service := &UserService{
		emailVerifier:       deps.EmailVerifier,
		newEmbyClient:       deps.NewEmbyClient,
		findUserByID:        deps.FindUserByID,
		findUserByUsername:  deps.FindUserByUsername,
		findUserByEmail:     deps.FindUserByEmail,
		createUser:          deps.CreateUser,
		getPlanGroupByKey:   deps.GetPlanGroupByKey,
		saveUser:            deps.SaveUser,
		deleteUserRecord:    deps.DeleteUserRecord,
		updateUserActive:    deps.UpdateUserActive,
		getUserViewByID:     deps.GetUserViewByID,
		updateEmailWithCode: deps.UpdateEmailWithCode,
		compensation:        deps.Compensation,
		newCompensation:     deps.NewCompensation,
		applyPolicy:         deps.ApplyPolicy,
		recordPolicyFailure: deps.RecordPolicyFailure,
	}

	if service.emailVerifier == nil {
		service.emailVerifier = emailpkg.NewEmailService()
	}
	if service.newEmbyClient == nil {
		service.newEmbyClient = func() embyClient { return embyint.GetSharedService() }
	}
	if service.findUserByID == nil {
		service.findUserByID = func(userID string) (*models.User, error) {
			var user models.User
			if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
				return nil, err
			}
			return &user, nil
		}
	}
	if service.findUserByUsername == nil {
		service.findUserByUsername = func(username string) (*models.User, error) {
			var user models.User
			if err := db.DB.Where("lower(username) = ?", strings.ToLower(username)).First(&user).Error; err != nil {
				return nil, err
			}
			return &user, nil
		}
	}
	if service.findUserByEmail == nil {
		service.findUserByEmail = func(email string) (*models.User, error) {
			var user models.User
			if err := db.DB.Where("lower(email) = ?", strings.ToLower(email)).First(&user).Error; err != nil {
				return nil, err
			}
			return &user, nil
		}
	}
	if service.createUser == nil {
		service.createUser = func(user *models.User) error {
			return db.DB.Create(user).Error
		}
	}
	if service.getPlanGroupByKey == nil {
		service.getPlanGroupByKey = func(key string) (*models.PlanGroup, error) {
			return paymentpkg.GetPlanGroupByKey(nil, key)
		}
	}
	if service.saveUser == nil {
		service.saveUser = func(user *models.User) error {
			return db.DB.Model(&models.User{}).
				Where("id = ?", user.ID).
				Select("password", "password_reset_required", "updated_at").
				Updates(map[string]any{
					"password":                user.Password,
					"password_reset_required": user.PasswordResetRequired,
					"updated_at":              time.Now(),
				}).Error
		}
	}
	if service.deleteUserRecord == nil {
		service.deleteUserRecord = func(user *models.User) error {
			return db.DB.Delete(user).Error
		}
	}
	if service.updateUserActive == nil {
		service.updateUserActive = func(userID string, isActive bool) error {
			return db.DB.Model(&models.User{}).
				Where("id = ?", userID).
				Updates(map[string]interface{}{
					"is_active": isActive,
				}).Error
		}
	}
	if service.getUserViewByID == nil {
		service.getUserViewByID = service.GetUserByID
	}
	if service.updateEmailWithCode == nil {
		service.updateEmailWithCode = service.updateEmailWithCodeTx
	}
	if service.newCompensation == nil {
		service.newCompensation = func() *accountpkg.EmbyCompensation {
			return accountpkg.NewEmbyCompensation(nil)
		}
	}
	if service.applyPolicy == nil {
		service.applyPolicy = func(userID, reason string) error {
			return policypkg.NewService(service.newEmbyClient()).ApplyEffectiveUserPolicy(userID, reason)
		}
	}
	if service.recordPolicyFailure == nil {
		service.recordPolicyFailure = func(userID, reason string, cause error) error {
			return policypkg.NewService(service.newEmbyClient()).RecordUserPolicySyncFailure(userID, reason, cause)
		}
	}
	return service
}

func (s *UserService) getEmailVerifier() emailVerifier {
	return s.emailVerifier
}

// embyClient 返回用户服务当前配置的 Emby 客户端；未完整装配的测试服务回退到共享客户端。
func (s *UserService) embyClient() embyClient {
	if s.newEmbyClient != nil {
		return s.newEmbyClient()
	}
	return embyint.GetSharedService()
}

var ErrInvalidExpiresAfter = errors.New("expiresAfter 必须是 YYYY-MM-DD 格式")
var ErrInvalidEmbyStatus = errors.New("embyStatus 仅支持 available/disabled/unlinked")
var ErrInvalidPlanGroup = paymentpkg.ErrPlanGroupInvalid

func normalizePlanGroupStrict(raw string) (string, error) {
	return paymentpkg.NormalizePlanGroupKey(raw, false)
}

// normalizePlanGroupUpdate 校验管理员更新用户分组时的显式分组值。
// 用户分组不再允许清空为 NULL，避免默认分组模板同步遗漏这类隐式跟随用户。
func normalizePlanGroupUpdate(raw string) (string, error) {
	return paymentpkg.NormalizePlanGroupKey(raw, false)
}

// syncEmbyPolicy 为已持久化用户应用统一计算后的 Emby Policy。
// 同步失败但失败任务记录成功时，已提交的本地变更仍应对前端返回成功；
// 只有失败任务也记录失败，才需要把错误继续上抛给调用方。
func (s *UserService) syncEmbyPolicy(user *models.User, reason string) error {
	if user.EmbyID == "" {
		return nil
	}
	if user.ID == "" || s.applyPolicy == nil {
		return nil
	}
	if err := s.applyPolicy(user.ID, reason); err != nil {
		if s.recordPolicyFailure == nil {
			return fmt.Errorf("同步 Emby 用户策略失败：%w；未配置同步失败任务记录器", err)
		}
		if recordErr := s.recordPolicyFailure(user.ID, reason, err); recordErr != nil {
			return fmt.Errorf("同步 Emby 用户策略失败：%w；记录同步失败任务失败：%v", err, recordErr)
		}
		log.Printf("[User] Emby Policy 同步失败，已记录单用户失败任务: userID=%s reason=%s", user.ID, reason)
		return nil
	}
	refreshed, err := s.findUserByID(user.ID)
	if err != nil {
		return nil
	}
	user.EmbyDisabled = refreshed.EmbyDisabled
	return nil
}
