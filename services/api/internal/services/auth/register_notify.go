package auth

import (
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
)

func (s *AuthService) notifyNewRegistration(user models.User, mode string) {
	go func(user models.User, mode string) {
		if s.notifier == nil || !s.notifier.IsConfigured() {
			return
		}

		var expiresAt *string
		if user.ExpiresAt != nil {
			formatted := user.ExpiresAt.UTC().Format("2006-01-02 15:04:05 MST")
			expiresAt = &formatted
		}

		s.notifier.NotifyNewRegistration(notifierint.RegistrationNotification{
			ID:               user.ID,
			UserName:         user.Username,
			Email:            user.Email,
			EmbyID:           user.EmbyID,
			RegistrationMode: mode,
			ExpiresAt:        expiresAt,
		})
	}(user, mode)
}
