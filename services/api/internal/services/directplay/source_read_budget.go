package directplay

import (
	"context"
	"errors"
	"time"
)

// sourceReadLimit bounds read-only source phases without interrupting retained
// provider writes. The small test seam does not create a deployment setting.
func (s *Service) sourceReadLimit() time.Duration {
	if s.sourceReadBudget > 0 {
		return s.sourceReadBudget
	}
	return sourceReadTimeout
}

// sourceReadResult distinguishes Ember's optional acceleration budget from a
// real provider failure and preserves caller cancellation before any fallback.
func sourceReadResult(parent, read context.Context, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(read.Err(), context.DeadlineExceeded) {
		return ErrSourceReadTimeout
	}
	return err
}
