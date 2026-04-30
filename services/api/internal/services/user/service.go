package user

import (
	"errors"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/gorm"
)

type emailVerifier interface {
	VerifyCode(email, code, codeType string) error
	CheckCode(email, code, codeType string) error
	ConsumeCodeTx(tx *gorm.DB, email, code, codeType string) error
}

type embyClient interface {
	AuthenticateUser(username, password string) (*embyint.EmbyUser, error)
	UpdateUserPassword(embyUserID, newPassword string) error
	CreateEmbyUser(username, password string) (*embyint.EmbyUser, error)
	SetUserPolicy(embyUserID string, policy embyint.EmbyUserPolicy) error
	DeleteUser(embyUserID string) error
}

type UserServiceDeps struct {
	EmailVerifier      emailVerifier
	NewEmbyClient      func() embyClient
	FindUserByID       func(userID string) (*models.User, error)
	FindUserByUsername func(username string) (*models.User, error)
	FindUserByEmail    func(email string) (*models.User, error)
	CreateUser         func(user *models.User) error
	GetPlanGroupByKey  func(key string) (*models.PlanGroup, error)
	SaveUser           func(user *models.User) error
	Compensation       *accountpkg.EmbyCompensation
	NewCompensation    func() *accountpkg.EmbyCompensation
}

// UserService 用户服务
type UserService struct {
	emailVerifier      emailVerifier
	newEmbyClient      func() embyClient
	findUserByID       func(userID string) (*models.User, error)
	findUserByUsername func(username string) (*models.User, error)
	findUserByEmail    func(email string) (*models.User, error)
	createUser         func(user *models.User) error
	getPlanGroupByKey  func(key string) (*models.PlanGroup, error)
	saveUser           func(user *models.User) error
	compensation       *accountpkg.EmbyCompensation
	newCompensation    func() *accountpkg.EmbyCompensation
}

func NewUserService() *UserService {
	return NewUserServiceWithDeps(UserServiceDeps{})
}

func NewUserServiceWithEmailVerifier(verifier emailVerifier) *UserService {
	return NewUserServiceWithDeps(UserServiceDeps{EmailVerifier: verifier})
}

func NewUserServiceWithDeps(deps UserServiceDeps) *UserService {
	service := &UserService{
		emailVerifier:      deps.EmailVerifier,
		newEmbyClient:      deps.NewEmbyClient,
		findUserByID:       deps.FindUserByID,
		findUserByUsername: deps.FindUserByUsername,
		findUserByEmail:    deps.FindUserByEmail,
		createUser:         deps.CreateUser,
		getPlanGroupByKey:  deps.GetPlanGroupByKey,
		saveUser:           deps.SaveUser,
		compensation:       deps.Compensation,
		newCompensation:    deps.NewCompensation,
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
				Select("password", "passwordResetRequired", "updatedAt").
				Updates(map[string]any{
					"password":              user.Password,
					"passwordResetRequired": user.PasswordResetRequired,
					"updatedAt":             time.Now(),
				}).Error
		}
	}
	if service.newCompensation == nil {
		service.newCompensation = func() *accountpkg.EmbyCompensation {
			return accountpkg.NewEmbyCompensation(nil)
		}
	}
	return service
}

func (s *UserService) getEmailVerifier() emailVerifier {
	return s.emailVerifier
}

var ErrInvalidExpiresAfter = errors.New("expiresAfter 必须是 YYYY-MM-DD 格式")
var ErrInvalidEmbyStatus = errors.New("embyStatus 仅支持 available/disabled/unlinked")
var ErrInvalidPlanGroup = paymentpkg.ErrPlanGroupInvalid

func normalizePlanGroupStrict(raw string) (string, error) {
	return paymentpkg.NormalizePlanGroupKey(raw, false)
}

func isUserExpired(expiresAt *time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return expiresAt.Before(time.Now().UTC())
}

func (s *UserService) syncEmbyPolicy(user *models.User) error {
	if user.EmbyID == "" {
		return nil
	}

	shouldDisable := !user.IsActive || isUserExpired(user.ExpiresAt)
	if shouldDisable == user.EmbyDisabled {
		return nil
	}

	embyService := s.newEmbyClient()
	if err := embyService.SetUserPolicy(user.EmbyID, embyint.EmbyUserPolicy{IsDisabled: shouldDisable}); err != nil {
		return errors.New("同步 Emby 用户状态失败：" + err.Error())
	}

	user.EmbyDisabled = shouldDisable
	return nil
}
