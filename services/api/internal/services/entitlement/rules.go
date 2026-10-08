package entitlement

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

const (
	Duration  = "duration"
	Permanent = "permanent"
)

var (
	ErrInvalidBenefit = errors.New("权益配置无效")
	ErrInvalidHolding = errors.New("用户权益记录无效")
	ErrAlreadyOwned   = errors.New("已拥有对应永久权益")
	ErrGroupsNotReady = errors.New("请先完成分组等级和资源库包含关系配置")
)

// Benefit is the immutable grant instruction stored with a purchased product.
type Benefit = models.PlanBenefit

// Holding separates permanent access from absence; a duration always has a deadline.
type Holding struct {
	WatchRetentionStartedAt     *time.Time `json:"watchRetentionStartedAt,omitempty" gorm:"column:watch_retention_started_at"`
	WatchRetentionInvalidatedAt *time.Time `json:"watchRetentionInvalidatedAt,omitempty" gorm:"column:watch_retention_invalidated_at"`

	PlanGroup    string     `json:"planGroup"`
	ValidityType string     `json:"validityType"`
	ExpiresAt    *time.Time `json:"expiresAt"`
}

// ValidateBenefits rejects duplicates and ambiguous validity before any state is changed.
func ValidateBenefits(benefits []Benefit) error {
	if len(benefits) == 0 {
		return ErrInvalidBenefit
	}
	seen := make(map[string]bool, len(benefits))
	for _, benefit := range benefits {
		if strings.TrimSpace(benefit.PlanGroup) == "" || benefit.PlanGroup != strings.TrimSpace(benefit.PlanGroup) || seen[benefit.PlanGroup] {
			return ErrInvalidBenefit
		}
		seen[benefit.PlanGroup] = true
		switch benefit.ValidityType {
		case Duration:
			if benefit.DurationDays < 1 {
				return ErrInvalidBenefit
			}
		case Permanent:
			if benefit.DurationDays != 0 {
				return ErrInvalidBenefit
			}
		default:
			return ErrInvalidBenefit
		}
	}
	return nil
}

// validateHoldings prevents missing configuration from silently removing purchased access.
func validateHoldings(owned []Holding, ranks map[string]int) error {
	seen := map[string]bool{}
	for _, holding := range owned {
		if seen[holding.PlanGroup] {
			return ErrInvalidHolding
		}
		seen[holding.PlanGroup] = true
		if _, ok := ranks[holding.PlanGroup]; !ok {
			return fmt.Errorf("%w: %s", ErrGroupsNotReady, holding.PlanGroup)
		}
		if holding.ValidityType == Permanent && holding.ExpiresAt == nil {
			continue
		}
		if holding.ValidityType != Duration || holding.ExpiresAt == nil {
			return ErrInvalidHolding
		}
	}
	return nil
}

// Resolve selects one complete group at an explicit reconciliation time, never on a timer of its own.
func Resolve(owned []Holding, ranks map[string]int, at time.Time) (*Holding, error) {
	if err := validateHoldings(owned, ranks); err != nil {
		return nil, err
	}
	var selected *Holding
	for _, holding := range owned {
		if holding.WatchRetentionInvalidatedAt != nil || holding.ValidityType == Duration && !holding.ExpiresAt.After(at) {
			continue
		}
		if selected != nil && ranks[holding.PlanGroup] == ranks[selected.PlanGroup] {
			return nil, ErrGroupsNotReady
		}
		if selected == nil || ranks[holding.PlanGroup] > ranks[selected.PlanGroup] {
			copy := holding
			selected = &copy
		}
	}
	return selected, nil
}

// Grant applies a bundle atomically in memory; callers persist it under the user's row lock.
// Covered entries are skipped, but a wholly covered purchase returns ErrAlreadyOwned.
func Grant(owned []Holding, benefits []Benefit, ranks map[string]int, at time.Time, location *time.Location) ([]Holding, error) {
	if err := ValidateBenefits(benefits); err != nil {
		return nil, err
	}
	if err := validateHoldings(owned, ranks); err != nil {
		return nil, err
	}
	if location == nil {
		return nil, errors.New("未配置全局业务时区")
	}
	byGroup := make(map[string]Holding, len(owned)+len(benefits))
	for _, holding := range owned {
		byGroup[holding.PlanGroup] = holding
	}
	changed := false
	for _, benefit := range benefits {
		rank, ok := ranks[benefit.PlanGroup]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrGroupsNotReady, benefit.PlanGroup)
		}
		// Use the pre-purchase state: a permanent high group in this same bundle
		// must not erase the separate low-group permanent grant the customer bought.
		covered := false
		for _, holding := range owned {
			if holding.WatchRetentionInvalidatedAt == nil && holding.ValidityType == Permanent && ranks[holding.PlanGroup] >= rank {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		next := Holding{PlanGroup: benefit.PlanGroup, ValidityType: benefit.ValidityType}
		if previous, ok := byGroup[benefit.PlanGroup]; ok && previous.WatchRetentionInvalidatedAt == nil {
			next.WatchRetentionStartedAt = previous.WatchRetentionStartedAt
		}
		if benefit.ValidityType == Duration {
			base := at
			if previous, ok := byGroup[benefit.PlanGroup]; ok && previous.ExpiresAt != nil && previous.ExpiresAt.After(base) {
				base = *previous.ExpiresAt
			}
			expires := base.In(location).AddDate(0, 0, benefit.DurationDays).UTC()
			if !expires.After(base) || expires.Year() > 9999 || expires.Year() < 1 {
				return nil, ErrInvalidBenefit
			}
			next.ExpiresAt = &expires
		}
		byGroup[benefit.PlanGroup] = next
		changed = true
	}
	if !changed {
		return nil, ErrAlreadyOwned
	}
	result := make([]Holding, 0, len(byGroup))
	for _, holding := range byGroup {
		result = append(result, holding)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PlanGroup < result[j].PlanGroup })
	return result, nil
}

// Adjust changes only the selected group; unlike a purchase, an explicit administrator action may shorten or revoke access.
func Adjust(owned []Holding, target Holding, revoke bool) ([]Holding, error) {
	if strings.TrimSpace(target.PlanGroup) == "" {
		return nil, ErrInvalidHolding
	}
	if !revoke {
		if err := validateHoldings([]Holding{target}, map[string]int{target.PlanGroup: 0}); err != nil {
			return nil, err
		}
	}
	result := make([]Holding, 0, len(owned)+1)
	for _, holding := range owned {
		if holding.PlanGroup != target.PlanGroup {
			result = append(result, holding)
		}
	}
	if !revoke {
		result = append(result, target)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PlanGroup < result[j].PlanGroup })
	return result, nil
}
