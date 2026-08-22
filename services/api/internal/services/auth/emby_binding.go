package auth

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
	"gorm.io/gorm"
)

// 管理员 Emby 账号绑定 / 解绑相关错误。
var (
	// ErrEmbyBindingTargetRequired 请求体缺少目标 Emby 用户 ID。
	ErrEmbyBindingTargetRequired = errors.New("请选择 Emby 用户")

	// ErrEmbyBindingUserNotFound 表示前端提交的 Emby 用户已不存在。
	ErrEmbyBindingUserNotFound = errors.New("Emby 用户不存在")

	// ErrEmbyServiceUnavailable 表示 Emby 配置缺失、服务不可达或 API Key 不可用。
	ErrEmbyServiceUnavailable = errors.New("Emby 服务暂不可用")

	// ErrEmbyUserSearchQueryRequired 表示 Emby 用户候选查询缺少关键词。
	ErrEmbyUserSearchQueryRequired = errors.New("请输入至少 2 个字符搜索 Emby 用户")
	ErrEmbyTokenRevocation         = errors.New("Emby 登录撤销失败")

	// ErrEmbyAlreadyBound 当前账号已绑定其他 Emby 用户，需要先解绑。
	ErrEmbyAlreadyBound = errors.New("当前账号已绑定其他 Emby 用户，请先解除绑定")
)

const (
	defaultAdminEmbyUserSearchLimit = 20
	maxAdminEmbyUserSearchLimit     = 50
	minAdminEmbyUserSearchQueryLen  = 2
)

// ErrEmbyUserOccupied Emby 用户已被其他本地账号占用。错误消息中带冲突方本地用户名，
// 不暴露 user ID。每次调用都新建一个实例，避免错误消息被全局复用。
type ErrEmbyUserOccupied struct {
	ConflictUsername string
}

func (e *ErrEmbyUserOccupied) Error() string {
	if e.ConflictUsername == "" {
		return "该 Emby 用户已被其他本地账号占用"
	}
	return "该 Emby 用户已被本地账号 " + e.ConflictUsername + " 占用"
}

// IsEmbyUserOccupied 用于 handler 层做 errors.As 判断。
func IsEmbyUserOccupied(err error) bool {
	var target *ErrEmbyUserOccupied
	return errors.As(err, &target)
}

// AdminEmbyUserOption 表示管理员绑定弹窗中的单个 Emby 用户候选项。
type AdminEmbyUserOption struct {
	EmbyID         string `json:"embyId"`
	Name           string `json:"name"`
	HasPassword    bool   `json:"hasPassword"`
	BoundUsername  string `json:"boundUsername,omitempty"`
	BoundToCurrent bool   `json:"boundToCurrent"`
	Available      bool   `json:"available"`
}

// ListAdminEmbyUsersResponse 管理员 Emby 用户候选列表响应。
type ListAdminEmbyUsersResponse struct {
	Data []AdminEmbyUserOption `json:"data"`
}

// ListAdminEmbyUsersRequest 管理员 Emby 用户候选查询请求。
type ListAdminEmbyUsersRequest struct {
	Query string
	Limit int
}

// BindEmbyAccountRequest 管理员关联 Emby 账号请求。
type BindEmbyAccountRequest struct {
	EmbyID string `json:"embyId"`
}

// BindEmbyAccountResponse 管理员关联 Emby 账号成功响应。
type BindEmbyAccountResponse struct {
	EmbyID       string `json:"embyId"`
	EmbyUsername string `json:"embyUsername"`
}

// ListAdminEmbyUsers 返回管理员可选择绑定的 Emby 用户候选列表。
//
// 该方法要求调用方提供搜索关键词，并只返回有限候选，避免弹窗打开时把全量
// Emby 用户列表暴露给浏览器。当前 Emby 封装仍通过 API Key 拉取用户列表后在
// 服务端过滤；后续若确认 Emby 存在稳定用户查询端点，可把过滤下推到 Emby 侧。
func (s *AuthService) ListAdminEmbyUsers(userID string, req ListAdminEmbyUsersRequest) (*ListAdminEmbyUsersResponse, error) {
	query := strings.TrimSpace(req.Query)
	if len([]rune(query)) < minAdminEmbyUserSearchQueryLen {
		return nil, ErrEmbyUserSearchQueryRequired
	}
	limit := normalizeAdminEmbyUserSearchLimit(req.Limit)
	normalizedQuery := strings.ToLower(query)

	embyService := s.newEmbyClient()
	embyUsers, err := embyService.GetUsers()
	if err != nil {
		log.Printf("[Admin Emby Binding] op=list userID=%s result=emby_unavailable err=%v", userID, err)
		return nil, ErrEmbyServiceUnavailable
	}

	matchedUsers := make([]embyint.EmbyUser, 0, limit)
	embyIDs := make([]string, 0, limit)
	for _, embyUser := range embyUsers {
		embyID := strings.TrimSpace(embyUser.ID)
		if embyID == "" {
			continue
		}
		if !matchesAdminEmbyUserSearch(embyUser, normalizedQuery) {
			continue
		}
		matchedUsers = append(matchedUsers, embyUser)
		embyIDs = append(embyIDs, embyID)
		if len(matchedUsers) >= limit {
			break
		}
	}

	boundUsers, err := s.findUsersByEmbyIDs(embyIDs)
	if err != nil {
		log.Printf("[Admin Emby Binding] op=list userID=%s result=local_lookup_failed err=%v", userID, err)
		return nil, err
	}

	boundByEmbyID := make(map[string]models.User, len(boundUsers))
	for _, user := range boundUsers {
		if embyID := strings.TrimSpace(user.EmbyID); embyID != "" {
			boundByEmbyID[embyID] = user
		}
	}

	options := make([]AdminEmbyUserOption, 0, len(matchedUsers))
	for _, embyUser := range matchedUsers {
		embyID := strings.TrimSpace(embyUser.ID)
		if embyID == "" {
			continue
		}

		option := AdminEmbyUserOption{
			EmbyID:         embyID,
			Name:           embyUser.Name,
			HasPassword:    embyUser.HasPassword,
			BoundToCurrent: false,
			Available:      true,
		}
		if boundUser, ok := boundByEmbyID[embyID]; ok {
			option.BoundUsername = boundUser.Username
			option.BoundToCurrent = boundUser.ID == userID
			option.Available = option.BoundToCurrent
		}
		options = append(options, option)
	}

	log.Printf("[Admin Emby Binding] op=list userID=%s result=success queryLen=%d total=%d limit=%d",
		userID, len([]rune(query)), len(options), limit)
	return &ListAdminEmbyUsersResponse{Data: options}, nil
}

// BindEmbyAccount 把 Emby 用户绑定到当前本地用户（管理员控制台自助接入）。
//
// 关键流程：
//  1. 校验请求体中的 embyId；
//  2. 调用 embyService.GetUserByID 校验目标 Emby 用户仍存在；
//  3. 应用层先读当前用户：已绑同一目标视为幂等成功，绑定到其他目标返回
//     ErrEmbyAlreadyBound；
//  4. 应用层再查冲突方：若另一本地账号已绑定目标 EmbyID，返回带冲突用户名的
//     ErrEmbyUserOccupied；
//  5. UPDATE 当前 user 的 emby_id；DB 偏唯一索引兜底并发，捕获唯一约束错误同样
//     翻译为 ErrEmbyUserOccupied。
//
// 日志按 [Admin Emby Binding] 前缀打入口、关键决策、失败点；严禁输出
// Emby API Key 或完整返回体。
func (s *AuthService) BindEmbyAccount(userID string, req *BindEmbyAccountRequest) (*BindEmbyAccountResponse, error) {
	return s.BindEmbyAccountWithContext(context.Background(), userID, userID, req)
}

// BindEmbyAccountWithContext revokes legacy mappings before binding a new Emby
// identity; an idempotent bind to the same identity leaves the active login.
func (s *AuthService) BindEmbyAccountWithContext(ctx context.Context, userID, actor string, req *BindEmbyAccountRequest) (*BindEmbyAccountResponse, error) {
	targetEmbyID := ""
	if req != nil {
		targetEmbyID = strings.TrimSpace(req.EmbyID)
	}
	if targetEmbyID == "" {
		return nil, ErrEmbyBindingTargetRequired
	}

	embyService := s.newEmbyClient()
	embyUser, err := embyService.GetUserByID(targetEmbyID)
	if err != nil {
		if errors.Is(err, embyint.ErrEmbyUserNotFound) {
			log.Printf("[Admin Emby Binding] op=bind userID=%s targetEmbyId=%s result=emby_user_not_found",
				userID, targetEmbyID)
			return nil, ErrEmbyBindingUserNotFound
		}
		log.Printf("[Admin Emby Binding] op=bind userID=%s targetEmbyId=%s result=emby_unavailable err=%v",
			userID, targetEmbyID, err)
		return nil, ErrEmbyServiceUnavailable
	}
	if embyUser == nil || strings.TrimSpace(embyUser.ID) == "" {
		log.Printf("[Admin Emby Binding] op=bind userID=%s targetEmbyId=%s result=emby_user_not_found err=empty_emby_user",
			userID, targetEmbyID)
		return nil, ErrEmbyBindingUserNotFound
	}
	targetEmbyID = strings.TrimSpace(embyUser.ID)

	// 当前用户视角校验
	current, err := s.findUserByIDForBinding(userID)
	if err != nil {
		log.Printf("[Admin Emby Binding] op=bind userID=%s result=lookup_failed err=%v", userID, err)
		return nil, err
	}

	if current.EmbyID == targetEmbyID {
		log.Printf("[Admin Emby Binding] op=bind userID=%s embyId=%s result=idempotent_success",
			userID, targetEmbyID)
		return &BindEmbyAccountResponse{
			EmbyID:       targetEmbyID,
			EmbyUsername: embyUser.Name,
		}, nil
	}
	if current.EmbyID != "" {
		log.Printf("[Admin Emby Binding] op=bind userID=%s currentEmbyId=%s targetEmbyId=%s result=already_bound",
			userID, current.EmbyID, targetEmbyID)
		return nil, ErrEmbyAlreadyBound
	}

	// 查目标 EmbyID 是否被其他本地账号占用
	if conflictUser, err := s.findOccupyingUser(targetEmbyID, userID); err != nil {
		log.Printf("[Admin Emby Binding] op=bind userID=%s targetEmbyId=%s result=occupy_check_failed err=%v",
			userID, targetEmbyID, err)
		return nil, err
	} else if conflictUser != nil {
		log.Printf("[Admin Emby Binding] op=bind userID=%s targetEmbyId=%s result=occupied_by_other conflictUsername=%s",
			userID, targetEmbyID, conflictUser.Username)
		return nil, &ErrEmbyUserOccupied{ConflictUsername: conflictUser.Username}
	}
	count, err := s.revokeUserTokensFn(ctx, current.ID, embytokenpkg.RevokeReasonSecurityRevoke, actor)
	if err != nil {
		return nil, ErrEmbyTokenRevocation
	}
	log.Printf("[Admin Emby Binding] op=bind userID=%s step=token_revoked count=%d", userID, count)

	// 写入。db 层唯一索引兜底并发。
	if err := s.updateUserEmbyID(userID, targetEmbyID); err != nil {
		if isEmbyIDUniqueViolation(err) {
			// 并发场景：在 occupy 检查与 UPDATE 之间，另一管理员抢先绑定了同一 EmbyID。
			// 再查一次冲突方，尽量给出具体用户名；查不到也给出兜底文案。
			conflictUser, _ := s.findOccupyingUser(targetEmbyID, userID)
			log.Printf("[Admin Emby Binding] op=bind userID=%s targetEmbyId=%s result=concurrent_conflict",
				userID, targetEmbyID)
			occupied := &ErrEmbyUserOccupied{}
			if conflictUser != nil {
				occupied.ConflictUsername = conflictUser.Username
			}
			return nil, occupied
		}
		log.Printf("[Admin Emby Binding] op=bind userID=%s targetEmbyId=%s result=update_failed err=%v",
			userID, targetEmbyID, err)
		return nil, err
	}

	log.Printf("[Admin Emby Binding] op=bind userID=%s embyId=%s result=success", userID, targetEmbyID)
	return &BindEmbyAccountResponse{
		EmbyID:       targetEmbyID,
		EmbyUsername: embyUser.Name,
	}, nil
}

// UnbindEmbyAccount 解除当前本地用户的 Emby 关联，幂等执行。
func (s *AuthService) UnbindEmbyAccount(userID string) error {
	return s.UnbindEmbyAccountWithContext(context.Background(), userID, "system:auth-service")
}

// UnbindEmbyAccountWithContext revokes every historical mapping before
// clearing emby_id; idempotent unbind also cleans legacy active mappings.
func (s *AuthService) UnbindEmbyAccountWithContext(ctx context.Context, userID, actor string) error {
	current, err := s.findUserByIDForBinding(userID)
	if err != nil {
		log.Printf("[Admin Emby Binding] op=unbind userID=%s result=lookup_failed err=%v", userID, err)
		return err
	}
	count, err := s.revokeUserTokensFn(ctx, current.ID, embytokenpkg.RevokeReasonEmbyUnbound, actor)
	if err != nil {
		return ErrEmbyTokenRevocation
	}
	log.Printf("[Admin Emby Binding] op=unbind userID=%s step=token_revoked count=%d", userID, count)

	previousEmbyID := current.EmbyID
	if previousEmbyID == "" {
		log.Printf("[Admin Emby Binding] op=unbind userID=%s result=idempotent_success", userID)
		return nil
	}

	if err := s.updateUserEmbyID(userID, ""); err != nil {
		log.Printf("[Admin Emby Binding] op=unbind userID=%s previousEmbyId=%s result=update_failed err=%v",
			userID, previousEmbyID, err)
		return err
	}

	log.Printf("[Admin Emby Binding] op=unbind userID=%s previousEmbyId=%s result=success",
		userID, previousEmbyID)
	return nil
}

// 下面这些方法对外不暴露，定义为方法以便测试时通过依赖替换 hook 注入 mock。
// 当前实现直接走 db.DB；测试用例通过 newAuthServiceForBinding 注入函数级桩件。

func (s *AuthService) findUserByIDForBinding(userID string) (*models.User, error) {
	if s.findUserByIDForBindingFn != nil {
		return s.findUserByIDForBindingFn(userID)
	}
	var user models.User
	if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) findOccupyingUser(embyID, excludeUserID string) (*models.User, error) {
	if s.findOccupyingUserFn != nil {
		return s.findOccupyingUserFn(embyID, excludeUserID)
	}
	var user models.User
	err := db.DB.Where("emby_id = ? AND id <> ?", embyID, excludeUserID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) findUsersByEmbyIDs(embyIDs []string) ([]models.User, error) {
	if s.findUsersByEmbyIDsFn != nil {
		return s.findUsersByEmbyIDsFn(embyIDs)
	}
	if len(embyIDs) == 0 {
		return []models.User{}, nil
	}

	var users []models.User
	if err := db.DB.
		Where("emby_id IN ?", embyIDs).
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func normalizeAdminEmbyUserSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultAdminEmbyUserSearchLimit
	}
	if limit > maxAdminEmbyUserSearchLimit {
		return maxAdminEmbyUserSearchLimit
	}
	return limit
}

func matchesAdminEmbyUserSearch(user embyint.EmbyUser, normalizedQuery string) bool {
	if normalizedQuery == "" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(user.Name))
	embyID := strings.ToLower(strings.TrimSpace(user.ID))
	return strings.Contains(name, normalizedQuery) || strings.Contains(embyID, normalizedQuery)
}

func (s *AuthService) updateUserEmbyID(userID, embyID string) error {
	if s.updateUserEmbyIDFn != nil {
		return s.updateUserEmbyIDFn(userID, embyID)
	}
	return db.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Update("emby_id", embyID).Error
}

func isEmbyIDUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	target := "emby_id"
	return strings.Contains(strings.ToLower(pgErr.ConstraintName), target) ||
		strings.Contains(strings.ToLower(pgErr.Detail), target)
}
