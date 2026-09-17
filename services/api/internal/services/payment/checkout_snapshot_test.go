package payment

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// TestCheckoutRetryUsesPaymentSnapshotThroughFulfillment locks the charge and grant to one order across a failed creation and plan edits.
func TestCheckoutRetryUsesPaymentSnapshotThroughFulfillment(t *testing.T) {
	plan := &models.Plan{ID: "plan_1", Name: "Monthly", Price: 1200, Currency: "usd", Days: 30, PlanGroup: "VIP_A"}
	var payment *models.Payment
	var requests []url.Values
	var keys []string
	service := &PaymentService{
		getCheckoutConfig: func() (*checkoutConfig, error) {
			return &checkoutConfig{StripeSecret: "sk_test", SuccessURL: "https://example.com/success", CancelURL: "https://example.com/cancel", PaymentMethods: []string{"card"}}, nil
		},
		getCheckoutUser:         func(id string) (*models.User, error) { return &models.User{ID: id, PlanGroup: strPtr("VIP_A")}, nil },
		getCheckoutPlan:         func(string, string) (*models.Plan, error) { return plan, nil },
		expirePendingPaymentsFn: func(string, string, time.Time) error { return nil },
		reservePendingPaymentFn: func(userID string, p *models.Plan, now time.Time) (*models.Payment, error) {
			if payment == nil {
				payment = &models.Payment{ID: "pay_retry", UserID: userID, PlanID: p.ID, Amount: p.Price, Currency: p.Currency, Days: p.Days, Status: models.PaymentPending, CreatedAt: now, UpdatedAt: now}
			}
			return payment, nil
		},
		backfillCheckoutSession: func(id, sid, checkoutURL string) error {
			if id != payment.ID {
				t.Fatalf("wrong payment: %s", id)
			}
			payment.StripeSessionID = sid
			payment.CheckoutURL = checkoutURL
			return nil
		},
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatal(err)
			}
			requests = append(requests, form)
			keys = append(keys, req.Header.Get("Idempotency-Key"))
			if len(requests) == 1 {
				return nil, errors.New("simulated connection loss")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cs_retry","url":"https://checkout.example.com/retry"}`))}, nil
		})},
	}
	if _, err := service.CreateCheckoutSession("user_1", &CreateCheckoutRequest{PlanID: plan.ID}); err == nil {
		t.Fatal("expected first creation failure")
	}
	if payment.CheckoutURL != "" {
		t.Fatal("failed creation backfilled checkout")
	}
	plan.Price, plan.Currency, plan.Days = 9900, "hkd", 90
	if _, err := service.CreateCheckoutSession("user_1", &CreateCheckoutRequest{PlanID: plan.ID}); err != nil {
		t.Fatal(err)
	}
	for i, form := range requests {
		for key, want := range map[string]string{"line_items[0][price_data][unit_amount]": "1200", "line_items[0][price_data][currency]": "usd", "metadata[days]": "30", "metadata[payment_id]": "pay_retry"} {
			if got := form.Get(key); got != want {
				t.Fatalf("attempt %d %s = %q, want %q", i+1, key, got, want)
			}
		}
		if keys[i] != "checkout:pay_retry" {
			t.Fatalf("changed order identity: %s", keys[i])
		}
	}
	if plan.Price != 9900 || plan.Currency != "hkd" || plan.Days != 90 {
		t.Fatal("checkout mutated current plan")
	}
	if _, err := service.CreateCheckoutSession("user_1", &CreateCheckoutRequest{PlanID: plan.ID}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("backfilled checkout requested Stripe again: %d", len(requests))
	}

	database, mock, cleanup := newPaymentSQLMockDB(t)
	defer cleanup()
	dbpkg.DB = database
	currentExpiry := time.Now().UTC().AddDate(0, 0, 7).Truncate(time.Second)
	mock.ExpectBegin()
	expectPaymentFulfillmentRef(mock, *payment)
	expectPaymentUserLock(mock, payment.UserID, "VIP_A", currentExpiry)
	expectPaymentLock(mock, *payment)
	expectPaymentPlanRead(mock, payment.PlanID, "VIP_A")
	expectPaymentPlanGroupLookup(mock, "VIP_A")
	mock.ExpectExec(`UPDATE "users" SET "expires_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).WithArgs(currentExpiry.AddDate(0, 0, 30), sqlmock.AnyArg(), payment.UserID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "payments" SET "status"=\$1,"stripe_payment_intent_id"=\$2,"updated_at"=\$3 WHERE id = \$4`).WithArgs(models.PaymentCompleted, "pi_retry", sqlmock.AnyArg(), payment.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	event := &stripeWebhookEvent{Type: "checkout.session.completed"}
	event.Data.Object = stripeCheckoutSessionObject{ID: payment.StripeSessionID, PaymentIntent: "pi_retry", PaymentStatus: "paid", Metadata: map[string]string{"days": "90"}}
	if err := service.dispatchWebhook(event, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	payment.Status = models.PaymentCompleted
	mock.ExpectBegin()
	expectPaymentFulfillmentRef(mock, *payment)
	mock.ExpectRollback()
	if err := service.dispatchWebhook(event, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
