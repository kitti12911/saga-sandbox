package saga

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	orm "github.com/kitti12911/lib-orm/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/kitti12911/saga-sandbox/internal/database"
	"github.com/kitti12911/saga-sandbox/internal/messaging"
)

// newTestDB wires a sqlmock-backed bun DB. The saga models carry default:now()
// columns, so bun emits inserts as INSERT ... RETURNING — Query expectations
// whose returned row count doubles as RowsAffected. Ordered expectations also
// prove what a flow does NOT run (e.g. no capture enqueue on idempotent hits).
func newTestDB(t *testing.T) (*orm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqldb, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqldb.Close() })
	return orm.Wrap(bun.NewDB(sqldb, pgdialect.New())), mock
}

func paymentReq() PaymentRequest {
	return PaymentRequest{IdempotencyKey: "ik1", AccountID: "acc1", Amount: 900, Currency: "THB"}
}

func timestampCols() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(time.Now(), time.Now())
}

func instanceRows(id, state string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "saga_type", "idempotency_key", "state", "current_step", "last_error"}).
		AddRow(id, sagaTypePayment, "ik1", state, int32(captureStep), "")
}

func TestStartPaymentNewSaga(t *testing.T) {
	db, mock := newTestDB(t)
	o := New(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "saga_instances" .* ON CONFLICT \(saga_type, idempotency_key\) DO NOTHING RETURNING`).
		WillReturnRows(timestampCols())
	mock.ExpectQuery(`INSERT INTO "saga_steps" .* RETURNING`).WillReturnRows(timestampCols())
	mock.ExpectQuery(`INSERT INTO "saga_outbox" .* RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(time.Now()))
	mock.ExpectCommit()

	view, err := o.StartPayment(context.Background(), paymentReq())
	require.NoError(t, err)
	assert.NotEmpty(t, view.SagaID)
	assert.Equal(t, database.SagaRunning, view.State)
	assert.Equal(t, int32(captureStep), view.CurrentStep)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStartPaymentIdempotentReturnsExisting(t *testing.T) {
	db, mock := newTestDB(t)
	o := New(db)

	mock.ExpectBegin()
	// conflict: zero returned rows -> saga already exists for the key
	mock.ExpectQuery(`INSERT INTO "saga_instances"`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}))
	// no step/outbox inserts: flow jumps straight to loading the existing saga
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* WHERE \(saga_type = 'payment'\) AND \(idempotency_key = 'ik1'\)`).
		WillReturnRows(instanceRows("existing-id", database.SagaRunning))
	mock.ExpectCommit()

	view, err := o.StartPayment(context.Background(), paymentReq())
	require.NoError(t, err)
	assert.Equal(t, "existing-id", view.SagaID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStartPaymentErrors(t *testing.T) {
	cases := []struct {
		name    string
		expect  func(mock sqlmock.Sqlmock)
		wantErr string
	}{
		{
			name: "instance insert fails",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "saga_instances"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "insert saga instance",
		},
		{
			name: "load existing fails",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "saga_instances"`).
					WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}))
				mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "load existing saga",
		},
		{
			name: "step insert fails",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "saga_instances"`).WillReturnRows(timestampCols())
				mock.ExpectQuery(`INSERT INTO "saga_steps"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "insert saga step",
		},
		{
			name: "outbox insert fails",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`INSERT INTO "saga_instances"`).WillReturnRows(timestampCols())
				mock.ExpectQuery(`INSERT INTO "saga_steps"`).WillReturnRows(timestampCols())
				mock.ExpectQuery(`INSERT INTO "saga_outbox"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "insert saga outbox",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newTestDB(t)
			tc.expect(mock)

			_, err := New(db).StartPayment(context.Background(), paymentReq())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "start payment")
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestGetSaga(t *testing.T) {
	db, mock := newTestDB(t)
	o := New(db)

	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* WHERE \(id = 's1'\)`).
		WillReturnRows(instanceRows("s1", database.SagaCompleted))

	view, err := o.GetSaga(context.Background(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", view.SagaID)
	assert.Equal(t, database.SagaCompleted, view.State)
}

func TestGetSagaNotFound(t *testing.T) {
	db, mock := newTestDB(t)

	mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := New(db).GetSaga(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrSagaNotFound)
}

func TestGetSagaQueryError(t *testing.T) {
	db, mock := newTestDB(t)

	mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).WillReturnError(errors.New("boom"))

	_, err := New(db).GetSaga(context.Background(), "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get saga")
}

func okReply() messaging.Reply {
	return messaging.Reply{MsgID: "r1", SagaID: "s1", InReplyTo: "m1", Status: messaging.StatusOK}
}

func TestHandleReplyCompletesSaga(t *testing.T) {
	db, mock := newTestDB(t)
	o := New(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
		WillReturnRows(instanceRows("s1", database.SagaRunning))
	mock.ExpectExec(`UPDATE "saga_steps" AS "saga_step" SET status = 'SUCCEEDED'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "saga_instances" AS "saga_instance" SET state = 'COMPLETED'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, o.HandleReply(context.Background(), okReply()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandleReplyFailedCompensates(t *testing.T) {
	db, mock := newTestDB(t)
	o := New(db)
	reply := okReply()
	reply.Status = messaging.StatusFailed
	reply.Error = "card declined"

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
		WillReturnRows(instanceRows("s1", database.SagaRunning))
	mock.ExpectExec(`UPDATE "saga_steps" AS "saga_step" SET status = 'FAILED'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "saga_instances" AS "saga_instance" SET state = 'COMPENSATED', last_error = 'card declined'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, o.HandleReply(context.Background(), reply))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandleReplyUnknownSagaIgnored(t *testing.T) {
	db, mock := newTestDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	require.NoError(t, New(db).HandleReply(context.Background(), okReply()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandleReplyTerminalSagaIgnored(t *testing.T) {
	db, mock := newTestDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).
		WillReturnRows(instanceRows("s1", database.SagaCompleted))
	// no UPDATE statements: terminal sagas ignore late replies
	mock.ExpectCommit()

	require.NoError(t, New(db).HandleReply(context.Background(), okReply()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandleReplyErrors(t *testing.T) {
	cases := []struct {
		name    string
		expect  func(mock sqlmock.Sqlmock)
		wantErr string
	}{
		{
			name: "load fails",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "load saga for reply",
		},
		{
			name: "step update fails",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectExec(`UPDATE "saga_steps"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "update saga step",
		},
		{
			name: "instance update fails",
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectExec(`UPDATE "saga_steps"`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE "saga_instances"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "update saga instance",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newTestDB(t)
			tc.expect(mock)

			err := New(db).HandleReply(context.Background(), okReply())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "handle reply")
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// --- sweeper DB paths ---

func stepRows(attempts int, request []byte) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "saga_id", "step_index", "step_name", "kind", "status", "attempts", "request"}).
		AddRow("st1", "s1", captureStep, captureStepName, database.KindForward, database.StepPending, attempts, request)
}

func captureRequestJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(messaging.Command{
		MsgID:          "m1",
		SagaID:         "s1",
		IdempotencyKey: "ik1",
		Type:           messaging.TypeCapture,
		Payload:        messaging.PaymentPayload{AccountID: "acc1", Amount: 900, Currency: "THB"},
	})
	require.NoError(t, err)
	return payload
}

func TestNewSweeperDefaults(t *testing.T) {
	db, _ := newTestDB(t)

	s := NewSweeper(db, 0, 0, 0, 0)
	assert.Equal(t, defaultSweepInterval, s.interval)
	assert.Equal(t, defaultStuckAfter, s.stuckAfter)
	assert.Equal(t, defaultMaxAttempts, s.maxAttempts)
	assert.Equal(t, defaultSweepBatch, s.batchSize)

	s = NewSweeper(db, time.Second, 2*time.Second, 3, 4)
	assert.Equal(t, time.Second, s.interval)
	assert.Equal(t, 2*time.Second, s.stuckAfter)
	assert.Equal(t, 3, s.maxAttempts)
	assert.Equal(t, 4, s.batchSize)
}

func TestSweepOnceRedrivesStuckSaga(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewSweeper(db, 0, 0, 5, 0)

	// stuck scan
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* WHERE \(state = 'RUNNING'\) AND \(updated_at < .*\) ORDER BY .* LIMIT 100`).
		WillReturnRows(instanceRows("s1", database.SagaRunning))
	// recover s1 -> redrive
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
		WillReturnRows(instanceRows("s1", database.SagaRunning))
	mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
		WillReturnRows(stepRows(1, captureRequestJSON(t)))
	mock.ExpectQuery(`INSERT INTO "saga_outbox" .* RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(time.Now()))
	mock.ExpectExec(`UPDATE "saga_steps" AS "saga_step" SET attempts = attempts \+ 1`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "saga_instances" AS "saga_instance" SET updated_at = now\(\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, s.sweepOnce(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSweepOnceEscalatesExhaustedSaga(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewSweeper(db, 0, 0, 3, 0)

	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* WHERE \(state = 'RUNNING'\)`).
		WillReturnRows(instanceRows("s1", database.SagaRunning))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
		WillReturnRows(instanceRows("s1", database.SagaRunning))
	mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
		WillReturnRows(stepRows(3, captureRequestJSON(t)))
	mock.ExpectExec(`UPDATE "saga_instances" AS "saga_instance" SET state = 'NEEDS_INTERVENTION'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, s.sweepOnce(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSweepOnceSelectError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewSweeper(db, 0, 0, 0, 0)

	mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).WillReturnError(errors.New("boom"))

	err := s.sweepOnce(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "select stuck sagas")
}

func TestSweepOnceContinuesAfterRecoverError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewSweeper(db, 0, 0, 0, 0)

	rows := sqlmock.NewRows([]string{"id", "state"}).
		AddRow("s1", database.SagaRunning).
		AddRow("s2", database.SagaRunning)
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* WHERE \(state = 'RUNNING'\)`).WillReturnRows(rows)

	// s1 recovery fails on load; the sweep logs and keeps going
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	// s2 vanished before the lock: no rows -> recover is a no-op
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	require.NoError(t, s.sweepOnce(context.Background()))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverSkipsAdvancedSaga(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewSweeper(db, 0, 0, 0, 0)

	mock.ExpectBegin()
	// a reply advanced the saga between scan and lock
	mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
		WillReturnRows(instanceRows("s1", database.SagaCompleted))
	mock.ExpectCommit()

	require.NoError(t, s.recover(context.Background(), "s1"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRecoverErrors(t *testing.T) {
	cases := []struct {
		name    string
		expect  func(t *testing.T, mock sqlmock.Sqlmock)
		wantErr string
	}{
		{
			name: "step load fails",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "load forward step",
		},
		{
			name: "escalate update fails",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
					WillReturnRows(stepRows(99, captureRequestJSON(t)))
				mock.ExpectExec(`UPDATE "saga_instances"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "escalate saga",
		},
		{
			name: "redrive empty request payload",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
					WillReturnRows(stepRows(0, nil))
				mock.ExpectRollback()
			},
			wantErr: "has no request payload",
		},
		{
			name: "redrive malformed request",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
					WillReturnRows(stepRows(0, []byte("{not json")))
				mock.ExpectRollback()
			},
			wantErr: "unmarshal step command",
		},
		{
			name: "redrive outbox insert fails",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
					WillReturnRows(stepRows(0, captureRequestJSON(t)))
				mock.ExpectQuery(`INSERT INTO "saga_outbox"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "insert redrive outbox",
		},
		{
			name: "redrive attempts bump fails",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
					WillReturnRows(stepRows(0, captureRequestJSON(t)))
				mock.ExpectQuery(`INSERT INTO "saga_outbox"`).
					WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(time.Now()))
				mock.ExpectExec(`UPDATE "saga_steps"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "bump step attempts",
		},
		{
			name: "redrive saga touch fails",
			expect: func(t *testing.T, mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .* FROM "saga_instances" .* FOR UPDATE`).
					WillReturnRows(instanceRows("s1", database.SagaRunning))
				mock.ExpectQuery(`SELECT .* FROM "saga_steps"`).
					WillReturnRows(stepRows(0, captureRequestJSON(t)))
				mock.ExpectQuery(`INSERT INTO "saga_outbox"`).
					WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(time.Now()))
				mock.ExpectExec(`UPDATE "saga_steps"`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE "saga_instances"`).WillReturnError(errors.New("boom"))
				mock.ExpectRollback()
			},
			wantErr: "touch saga",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newTestDB(t)
			s := NewSweeper(db, 0, 0, 5, 0)
			tc.expect(t, mock)

			err := s.recover(context.Background(), "s1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "recover saga s1")
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewSweeper(db, time.Millisecond, 0, 0, 0)

	// The first tick's sweep fails; Run logs and keeps looping until cancel.
	mock.ExpectQuery(`SELECT .* FROM "saga_instances"`).WillReturnError(errors.New("boom"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on context cancel")
	}
}
