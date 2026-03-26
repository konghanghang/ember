package user

import (
	"errors"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
)

type emailVerifier interface {
	VerifyCode(email, code, codeType string) error
}

// UserService 用户服务
type UserService struct {
	emailVerifier emailVerifier
}

func NewUserService() *UserService {
	return NewUserServiceWithEmailVerifier(emailpkg.NewEmailService())
}

func NewUserServiceWithEmailVerifier(verifier emailVerifier) *UserService {
	service := &UserService{}
	service.setEmailVerifier(verifier)
	return service
}

func (s *UserService) setEmailVerifier(verifier emailVerifier) {
	if verifier == nil {
		verifier = emailpkg.NewEmailService()
	}
	s.emailVerifier = verifier
}

func (s *UserService) getEmailVerifier() emailVerifier {
	if s.emailVerifier == nil {
		s.emailVerifier = emailpkg.NewEmailService()
	}
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
