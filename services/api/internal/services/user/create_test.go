package user

import (
	"errors"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

func TestCreateUserByAdmin(t *testing.T) {
	notFoundUserByName := func(string) (*models.User, error) { return nil, gorm.ErrRecordNotFound }
	notFoundUserByEmail := func(string) (*models.User, error) { return nil, gorm.ErrRecordNotFound }
	getPlanGroup := func(key string) (*models.PlanGroup, error) {
		return &models.PlanGroup{Key: key, Name: "VIP A"}, nil
	}

	t.Run("rejects invalid never expire payload", func(t *testing.T) {
		expiresAt := time.Now().UTC().Format(time.RFC3339)
		service := &UserService{
			findUserByUsername: notFoundUserByName,
			findUserByEmail:    notFoundUserByEmail,
			getPlanGroupByKey:  getPlanGroup,
		}

		_, err := service.CreateUserByAdmin(&AdminCreateUserRequest{
			Username:    "ember",
			Email:       "ember@example.com",
			Password:    "secret123",
			PlanGroup:   "VIP_A",
			NeverExpire: true,
			ExpiresAt:   &expiresAt,
		})
		if err == nil || err.Error() != "neverExpire=true 时不能再传 expiresAt" {
			t.Fatalf("expected neverExpire validation error, got %v", err)
		}
	})

	t.Run("rejects duplicate username before emby create", func(t *testing.T) {
		client := &stubUserEmbyClient{}
		service := &UserService{
			findUserByUsername: func(username string) (*models.User, error) {
				return &models.User{ID: "user_1", Username: username}, nil
			},
			findUserByEmail:   notFoundUserByEmail,
			getPlanGroupByKey: getPlanGroup,
			newEmbyClient:     func() embyClient { return client },
		}

		_, err := service.CreateUserByAdmin(&AdminCreateUserRequest{
			Username:    "ember",
			Email:       "ember@example.com",
			Password:    "secret123",
			PlanGroup:   "VIP_A",
			NeverExpire: true,
		})
		if err == nil || err.Error() != "用户名已存在" {
			t.Fatalf("expected duplicate username error, got %v", err)
		}
		if client.lastCreateName != "" {
			t.Fatalf("expected emby create not to be called")
		}
	})

	t.Run("success creates emby user and persists explicit plan group", func(t *testing.T) {
		client := &stubUserEmbyClient{
			createUserResp: newTestEmbyUser("emby_1", "ember"),
		}
		expiresAt := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
		var createdUser *models.User
		service := &UserService{
			findUserByUsername: notFoundUserByName,
			findUserByEmail:    notFoundUserByEmail,
			getPlanGroupByKey:  getPlanGroup,
			newEmbyClient:      func() embyClient { return client },
			createUser: func(user *models.User) error {
				createdCopy := *user
				createdUser = &createdCopy
				return nil
			},
		}

		resp, err := service.CreateUserByAdmin(&AdminCreateUserRequest{
			Username:    "ember",
			Email:       "ember@example.com",
			Password:    "secret123",
			PlanGroup:   "vip_a",
			NeverExpire: false,
			ExpiresAt:   &expiresAt,
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if client.lastCreateName != "ember" || client.lastCreatePwd != "secret123" {
			t.Fatalf("unexpected emby create payload: %q %q", client.lastCreateName, client.lastCreatePwd)
		}
		if createdUser == nil {
			t.Fatalf("expected createUser to be called")
		}
		if createdUser.Role != "user" || createdUser.EmbyID != "emby_1" || createdUser.PlanGroup == nil || *createdUser.PlanGroup != "VIP_A" {
			t.Fatalf("unexpected persisted user: %+v", createdUser)
		}
		if createdUser.ExpiresAt == nil || createdUser.ExpiresAt.UTC().Format(time.RFC3339) != expiresAt {
			t.Fatalf("unexpected expiresAt: %+v", createdUser.ExpiresAt)
		}
		if !createdUser.CheckPassword("secret123") {
			t.Fatalf("expected password hash to be persisted")
		}
		if resp.EffectivePlanGroup != "VIP_A" || resp.EffectivePlanGroupName != "VIP A" || resp.PlanGroupName == nil || *resp.PlanGroupName != "VIP A" {
			t.Fatalf("unexpected response payload: %+v", resp)
		}
	})

	t.Run("persistence failure deletes created emby user", func(t *testing.T) {
		client := &stubUserEmbyClient{
			createUserResp: newTestEmbyUser("emby_2", "ember"),
		}
		service := &UserService{
			findUserByUsername: notFoundUserByName,
			findUserByEmail:    notFoundUserByEmail,
			getPlanGroupByKey:  getPlanGroup,
			newEmbyClient:      func() embyClient { return client },
			createUser: func(user *models.User) error {
				return errors.New("boom")
			},
		}

		_, err := service.CreateUserByAdmin(&AdminCreateUserRequest{
			Username:    "ember",
			Email:       "ember@example.com",
			Password:    "secret123",
			PlanGroup:   "VIP_A",
			NeverExpire: true,
		})
		if err == nil || err.Error() != "创建用户失败" {
			t.Fatalf("expected create failure, got %v", err)
		}
		if client.lastDeleteUserID != "emby_2" {
			t.Fatalf("expected emby rollback delete, got %q", client.lastDeleteUserID)
		}
	})
}

func newTestEmbyUser(id, name string) *embyint.EmbyUser {
	return &embyint.EmbyUser{
		ID:   id,
		Name: name,
	}
}
