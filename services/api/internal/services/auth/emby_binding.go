package auth

import (
	"errors"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// 管理员 Emby 账号绑定 / 解绑相关错误。
var (
	// ErrEmbyBindingCredentialRequired 请求体缺少 embyUsername 或 embyPassword。
	ErrEmbyBindingCredentialRequired = errors.New("请提供 Emby 用户名和密码")

	// ErrEmbyBindingAuthFailed Emby 凭据校验失败，或 Emby 服务暂不可达且当前
	// 无法从 embyService.AuthenticateUser 错误体中稳定区分网络错与凭据错。
	//
	// TODO(emby-binding): 后续在 EmbyService.AuthenticateUser 区分错误类型
	// （typed error / sentinel）后，将"无法连接到 Emby 服务器"类错误单独
	// 翻译成 ErrEmbyServiceUnavailable，再由 handler 映射 502。
	ErrEmbyBindingAuthFailed = errors.New("Emby 用户名或密码错误")

	// ErrEmbyAlreadyBound 当前账号已绑定其他 Emby 用户，需要先解绑。
	ErrEmbyAlreadyBound = errors.New("当前账号已绑定其他 Emby 用户，请先解除绑定")
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

// BindEmbyAccountRequest 管理员关联 Emby 账号请求。
type BindEmbyAccountRequest struct {
	EmbyUsername string `json:"embyUsername"`
	EmbyPassword string `json:"embyPassword"`
}

// BindEmbyAccountResponse 管理员关联 Emby 账号成功响应。
type BindEmbyAccountResponse struct {
	EmbyID       string `json:"embyId"`
	EmbyUsername string `json:"embyUsername"`
}

// BindEmbyAccount 把 Emby 用户绑定到当前本地用户（管理员控制台自助接入）。
//
// 关键流程：
//  1. 校验请求体；
//  2. 调用 embyService.AuthenticateUser 拿到目标 EmbyID；任何失败都翻译为
//     ErrEmbyBindingAuthFailed（短期内无法区分凭据错与网络错）；
//  3. 应用层先读当前用户：已绑同一目标视为幂等成功，绑定到其他目标返回
//     ErrEmbyAlreadyBound；
//  4. 应用层再查冲突方：若另一本地账号已绑定目标 EmbyID，返回带冲突用户名的
//     ErrEmbyUserOccupied；
//  5. UPDATE 当前 user 的 emby_id；DB 偏唯一索引兜底并发，捕获唯一约束错误同样
//     翻译为 ErrEmbyUserOccupied。
//
// 日志按 [Admin Emby Binding] 前缀打入口、关键决策、失败点；严禁输出 Emby 密码与
// AuthenticateUser 完整返回体。
func (s *AuthService) BindEmbyAccount(userID string, req *BindEmbyAccountRequest) (*BindEmbyAccountResponse, error) {
	embyUsername := strings.TrimSpace(req.EmbyUsername)
	embyPassword := req.EmbyPassword
	if embyUsername == "" || embyPassword == "" {
		return nil, ErrEmbyBindingCredentialRequired
	}

	embyService := s.newEmbyClient()
	embyUser, err := embyService.AuthenticateUser(embyUsername, embyPassword)
	if err != nil {
		log.Printf("[Admin Emby Binding] op=bind userID=%s embyUsername=%s result=auth_failed err=%v",
			userID, embyUsername, err)
		return nil, ErrEmbyBindingAuthFailed
	}
	if embyUser == nil || strings.TrimSpace(embyUser.ID) == "" {
		log.Printf("[Admin Emby Binding] op=bind userID=%s embyUsername=%s result=auth_failed err=empty_emby_user",
			userID, embyUsername)
		return nil, ErrEmbyBindingAuthFailed
	}
	targetEmbyID := embyUser.ID

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
	current, err := s.findUserByIDForBinding(userID)
	if err != nil {
		log.Printf("[Admin Emby Binding] op=unbind userID=%s result=lookup_failed err=%v", userID, err)
		return err
	}

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

// 下面三个方法对外不暴露，定义为方法以便测试时通过依赖替换 hook 注入 mock。
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
