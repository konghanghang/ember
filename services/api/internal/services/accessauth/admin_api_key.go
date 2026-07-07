package accessauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AdminAPIKeySettingKey = "external_api_key_hash"
	AdminAPIKeyPrefix     = "ember_sk_"
	adminAPIKeyRandomSize = 32
)

var ErrAdminAPIKeyStoreUnavailable = errors.New("admin api key store unavailable")

type AdminAPIKeyStatus struct {
	Configured bool `json:"configured"`
}

type GeneratedAdminAPIKey struct {
	Configured bool   `json:"configured"`
	APIKey     string `json:"apiKey"`
}

type adminAPIKeyStore interface {
	LoadHash() (string, error)
	SaveHash(hash string, updatedByUserID string) error
	ClearHash(updatedByUserID string) error
}

type AdminAPIKeyService struct {
	store adminAPIKeyStore
}

func NewAdminAPIKeyService() *AdminAPIKeyService {
	return &AdminAPIKeyService{store: gormAdminAPIKeyStore{}}
}

func newAdminAPIKeyServiceWithStore(store adminAPIKeyStore) *AdminAPIKeyService {
	return &AdminAPIKeyService{store: store}
}

// Status 返回全局 Admin API Key 是否已启用，不暴露 hash 或明文。
func (s *AdminAPIKeyService) Status() (*AdminAPIKeyStatus, error) {
	hash, err := s.store.LoadHash()
	if err != nil {
		return nil, err
	}
	return &AdminAPIKeyStatus{Configured: strings.TrimSpace(hash) != ""}, nil
}

// Generate 生成新的高熵 Admin API Key，持久化 hash，并只在本次返回明文。
func (s *AdminAPIKeyService) Generate(updatedByUserID string) (*GeneratedAdminAPIKey, error) {
	apiKey, err := generateAdminAPIKey()
	if err != nil {
		return nil, err
	}

	if err := s.store.SaveHash(HashAdminAPIKey(apiKey), updatedByUserID); err != nil {
		return nil, err
	}

	return &GeneratedAdminAPIKey{
		Configured: true,
		APIKey:     apiKey,
	}, nil
}

// Disable 清空 Admin API Key hash，后续所有 API Key 请求立即失效。
func (s *AdminAPIKeyService) Disable(updatedByUserID string) (*AdminAPIKeyStatus, error) {
	if err := s.store.ClearHash(updatedByUserID); err != nil {
		return nil, err
	}
	return &AdminAPIKeyStatus{Configured: false}, nil
}

// Validate 使用 constant-time compare 校验请求中的 Admin API Key。
func (s *AdminAPIKeyService) Validate(apiKey string) (bool, error) {
	if !LooksLikeAdminAPIKey(apiKey) {
		return false, nil
	}

	storedHash, err := s.store.LoadHash()
	if err != nil {
		return false, err
	}
	storedHash = strings.TrimSpace(storedHash)
	if storedHash == "" {
		return false, nil
	}

	requestHash := HashAdminAPIKey(apiKey)
	return subtle.ConstantTimeCompare([]byte(requestHash), []byte(storedHash)) == 1, nil
}

// LooksLikeAdminAPIKey 只判断格式前缀和基本长度，不代表密钥有效。
func LooksLikeAdminAPIKey(apiKey string) bool {
	apiKey = strings.TrimSpace(apiKey)
	return strings.HasPrefix(apiKey, AdminAPIKeyPrefix) && len(apiKey) > len(AdminAPIKeyPrefix)+24
}

// HashAdminAPIKey 返回用于 settings 表保存的 sha256 hex hash。
func HashAdminAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}

func generateAdminAPIKey() (string, error) {
	buf := make([]byte, adminAPIKeyRandomSize)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return AdminAPIKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

type gormAdminAPIKeyStore struct{}

func (gormAdminAPIKeyStore) LoadHash() (string, error) {
	if db.DB == nil {
		return "", ErrAdminAPIKeyStoreUnavailable
	}

	var setting models.Setting
	err := db.DB.Select("key", "value").Where("key = ?", AdminAPIKeySettingKey).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (gormAdminAPIKeyStore) SaveHash(hash string, updatedByUserID string) error {
	return upsertAdminAPIKeyHash(hash, updatedByUserID)
}

func (gormAdminAPIKeyStore) ClearHash(updatedByUserID string) error {
	return upsertAdminAPIKeyHash("", updatedByUserID)
}

func upsertAdminAPIKeyHash(hash string, updatedByUserID string) error {
	if db.DB == nil {
		return ErrAdminAPIKeyStoreUnavailable
	}

	setting := models.Setting{
		Key:         AdminAPIKeySettingKey,
		Value:       strings.TrimSpace(hash),
		IsEncrypted: false,
	}
	if strings.TrimSpace(updatedByUserID) != "" {
		updatedBy := strings.TrimSpace(updatedByUserID)
		setting.UpdatedByUserID = &updatedBy
	}

	err := db.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"value":              setting.Value,
			"is_encrypted":       false,
			"updated_by_user_id": setting.UpdatedByUserID,
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&setting).Error
	if err == nil {
		configpkg.InvalidateCachedSetting(AdminAPIKeySettingKey)
	}
	return err
}
