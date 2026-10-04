package subscription

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestLoadSubmitterAccessProjection exercises the real selected columns rather than replacing the user loader.
func TestLoadSubmitterAccessProjection(t *testing.T) {
	for _, tc := range []struct {
		name              string
		granted, disabled bool
		want              error
	}{
		{"granted", true, false, nil},
		{"no entitlement", false, false, ErrSubscriptionEmbyDisabled},
		{"manual ban", true, true, ErrSubscriptionEmbyDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, mock, closeDB := newPendingRejectSQLMockDB(t)
			defer closeDB()
			withPendingRejectTestDB(t, database)
			// A passed deadline must not cause request-time reconciliation before the configured expiry job.
			past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			mock.ExpectQuery(`SELECT .*"resource_access_granted".* FROM "users"`).WithArgs("user1", 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "emby_id", "resource_access_granted", "emby_access_disabled", "expires_at"}).AddRow("user1", "emby1", tc.granted, tc.disabled, past))
			user, err := loadSubscriptionSubmitter("user1")
			if err != nil {
				t.Fatal(err)
			}
			if err = ensureUserCanSubmitSubscription(user); !errors.Is(err, tc.want) {
				t.Fatalf("access error=%v, want=%v", err, tc.want)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
