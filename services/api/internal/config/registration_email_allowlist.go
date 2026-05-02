package config

import (
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

// 注册邮箱域名白名单
//
// 数据形态：multiline 字符串，每行一个域名。空白名单语义为“关闭限制”，保持当前
// 任意合法邮箱均可注册的开放行为；非空白名单按精确字符串匹配，不做后缀匹配，
// 避免把 mail.gmail.com 等子域隐式放进 gmail.com 的允许范围。
//
// 规范化与校验在保存时完成，运行期读取拿到的就是“小写、去重、按字典序排序”的稳定列表。

// validRegistrationDomainPattern 是非常保守的主机名字符集校验：
// 只接受字母、数字、连字符、点；任何协议、端口、路径、通配符、空白都会在 split 前被拒绝。
//
// 真正的句法校验放在 splitRegistrationEmailDomains 里逐行处理，这里仅作为字符集兜底。
func validRegistrationDomainChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-' || r == '.':
		return true
	default:
		return false
	}
}

// splitRegistrationEmailDomains 把多行原始输入拆分成规范化后的域名列表。
// 任意一行不合法 → 整次返回错误，不接受“部分行有效部分行无效”这种半拉子配置。
//
// 输入清洗顺序：
//  1. 统一换行符
//  2. 按行 trim 空白
//  3. 跳过空行
//  4. 拒绝包含协议（"://"）、端口（":"）、路径（"/"）、查询/锚点、显式 "@"、通配符 "*" 的行
//  5. 转小写
//  6. 字符集校验与点段（label）合规性
//  7. 去重 + 按字典序排序
func splitRegistrationEmailDomains(value string) ([]string, error) {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	seen := make(map[string]struct{})
	domains := make([]string, 0)

	for lineIdx, raw := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)

		if strings.Contains(lower, "://") {
			return nil, fmt.Errorf("第 %d 行的域名不能包含协议", lineIdx+1)
		}
		if strings.ContainsAny(lower, "/?#") {
			return nil, fmt.Errorf("第 %d 行的域名不能包含路径或查询", lineIdx+1)
		}
		if strings.Contains(lower, ":") {
			return nil, fmt.Errorf("第 %d 行的域名不能包含端口", lineIdx+1)
		}
		if strings.Contains(lower, "@") {
			return nil, fmt.Errorf("第 %d 行只能填写域名，不要包含 @ 或完整邮箱", lineIdx+1)
		}
		if strings.Contains(lower, "*") {
			return nil, fmt.Errorf("第 %d 行的域名不支持通配符", lineIdx+1)
		}
		if strings.ContainsAny(lower, " \t") {
			return nil, fmt.Errorf("第 %d 行的域名不能包含空白字符", lineIdx+1)
		}

		if strings.HasPrefix(lower, ".") || strings.HasSuffix(lower, ".") || strings.Contains(lower, "..") {
			return nil, fmt.Errorf("第 %d 行的域名格式无效", lineIdx+1)
		}

		// 必须至少包含一个点（顶级域名 + 主域名），与“注册邮箱域名”语义一致；
		// 单段主机名（如 localhost）不应作为对外注册邮箱域名。
		if !strings.Contains(lower, ".") {
			return nil, fmt.Errorf("第 %d 行的域名必须包含至少一个点", lineIdx+1)
		}

		for _, r := range lower {
			if !validRegistrationDomainChar(r) {
				return nil, fmt.Errorf("第 %d 行的域名包含非法字符", lineIdx+1)
			}
		}

		labels := strings.Split(lower, ".")
		for _, label := range labels {
			if label == "" {
				return nil, fmt.Errorf("第 %d 行的域名格式无效", lineIdx+1)
			}
			if len(label) > 63 {
				return nil, fmt.Errorf("第 %d 行的域名段过长", lineIdx+1)
			}
			if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return nil, fmt.Errorf("第 %d 行的域名段不能以连字符开头或结尾", lineIdx+1)
			}
		}

		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		domains = append(domains, lower)
	}

	slices.Sort(domains)
	return domains, nil
}

// normalizeRegistrationEmailDomains 把任意输入规范化为存库形态：
// 每行一个、小写、去重、按字典序排序。空输入直接返回空字符串。
//
// 该函数同时完成校验：任何非法行都会让整次保存失败，配置中心不会落脏数据。
func normalizeRegistrationEmailDomains(value string) (string, error) {
	domains, err := splitRegistrationEmailDomains(value)
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return "", nil
	}
	return strings.Join(domains, "\n"), nil
}

func validateRegistrationEmailDomains(value string) error {
	_, err := splitRegistrationEmailDomains(value)
	return err
}

// GetRegistrationAllowedEmailDomains 读取当前生效的允许注册邮箱域名列表。
//
// 解析失败时返回空列表（视为白名单关闭），避免脏配置把整个注册链路打死。
// 正常路径下 settings 里持久化的就是已规范化的形态，这里再做一次解析是为了兼容
// 历史手工写入的脏数据与多行换行差异。
func (s *ConfigService) GetRegistrationAllowedEmailDomains() []string {
	raw := s.GetString("registration_allowed_email_domains")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	domains, err := splitRegistrationEmailDomains(raw)
	if err != nil {
		return nil
	}
	return domains
}

// IsRegistrationEmailAllowed 判断给定邮箱是否允许进入注册链路。
//
//   - 空白名单 → 视为关闭限制，返回 nil
//   - 非空白名单 → 解析邮箱后按域名精确匹配（不做后缀匹配）
//   - 邮箱本身不合法 → 返回 ErrRegistrationEmailInvalid
//   - 域名不在白名单 → 返回 ErrRegistrationEmailDomainNotAllowed
//
// 邮箱解析使用 net/mail.ParseAddress，确保对 "Display Name <addr@host>" 这类
// 带显示名的写法也能正确取到 host；不能用 strings.SplitN("@") 一刀切。
func (s *ConfigService) IsRegistrationEmailAllowed(email string) error {
	domains := s.GetRegistrationAllowedEmailDomains()
	if len(domains) == 0 {
		return nil
	}
	return matchRegistrationEmailAgainstDomains(email, domains)
}

// matchRegistrationEmailAgainstDomains 把“拿白名单”和“匹配语义”解耦：
// 给定一份已规范化的允许域名列表（小写、去重、排序），判断 email 是否命中。
//
// 单独抽出便于测试匹配语义本身，不需要打通 ConfigService → settings 表的真实链路。
func matchRegistrationEmailAgainstDomains(email string, domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return ErrRegistrationEmailInvalid
	}

	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed == nil || parsed.Address == "" {
		return ErrRegistrationEmailInvalid
	}

	at := strings.LastIndex(parsed.Address, "@")
	if at <= 0 || at == len(parsed.Address)-1 {
		return ErrRegistrationEmailInvalid
	}
	domain := strings.ToLower(parsed.Address[at+1:])

	if !slices.Contains(domains, domain) {
		return ErrRegistrationEmailDomainNotAllowed
	}
	return nil
}
