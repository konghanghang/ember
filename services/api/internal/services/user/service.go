package user

import (
	"errors"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
)

type emailVerifier interface {
	VerifyCode(email, code, codeType string) error
}

type embyClient interface {
	AuthenticateUser(username, password string) (*embyint.EmbyUser, error)
	UpdateUserPassword(embyUserID, newPassword string) error
	SetUserPolicy(embyUserID string, policy embyint.EmbyUserPolicy) error
	DeleteUser(embyUserID string) error
}

// UserService 用户服务
type UserService struct {
	emailVerifier    emailVerifier
	newEmbyClient    func() embyClient
	findUserByID     func(userID string) (*models.User, error)
	findUserByEmail  func(email string) (*models.User, error)
	saveUser         func(user *models.User) error
	deleteResetCodes func(email string) (int64, error)
}

func NewUserService() *UserService {
	return NewUserServiceWithEmailVerifier(emailpkg.NewEmailService())
}

func NewUserServiceWithEmailVerifier(verifier emailVerifier) *UserService {
	service := &UserService{}
	service.setEmailVerifier(verifier)
	service.setDefaults()
	return service
}

func (s *UserService) setEmailVerifier(verifier emailVerifier) {
	if verifier == nil {
		verifier = emailpkg.NewEmailService()
	}
	s.emailVerifier = verifier
}

func (s *UserService) setDefaults() {
	if s.newEmbyClient == nil {
		s.newEmbyClient = func() embyClient { return embyint.NewEmbyService() }
	}
	if s.findUserByID == nil {
		s.findUserByID = func(userID string) (*models.User, error) {
			var user models.User
			if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
				return nil, err
			}
			return &user, nil
		}
	}
	if s.findUserByEmail == nil {
		s.findUserByEmail = func(email string) (*models.User, error) {
			var user models.User
			if err := db.DB.Where("email = ?", email).First(&user).Error; err != nil {
				return nil, err
			}
			return &user, nil
		}
	}
	if s.saveUser == nil {
		s.saveUser = func(user *models.User) error {
			return db.DB.Save(user).Error
		}
	}
	if s.deleteResetCodes == nil {
		s.deleteResetCodes = func(email string) (int64, error) {
			result := db.DB.Where("email = ? AND \"type\" = ?", email, models.VerificationTypeReset).
				Delete(&models.EmailVerification{})
			return result.RowsAffected, result.Error
		}
	}
}

func (s *UserService) getEmailVerifier() emailVerifier {
	if s.emailVerifier == nil {
		s.emailVerifier = emailpkg.NewEmailService()
	}
	s.setDefaults()
	return s.emailVerifier
}

var ErrInvalidExpiresAfter = errors.New("expiresAfter 必须是 YYYY-MM-DD 格式")
var ErrInvalidEmbyStatus = errors.New("embyStatus 仅支持 available/disabled/unlinked")

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

	embyService := embyint.NewEmbyService()
	if err := embyService.SetUserPolicy(user.EmbyID, embyint.EmbyUserPolicy{IsDisabled: shouldDisable}); err != nil {
		return errors.New("同步 Emby 用户状态失败：" + err.Error())
	}

	user.EmbyDisabled = shouldDisable
	return nil
}
