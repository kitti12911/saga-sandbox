// Package messaging defines the NATS JSON contract between the saga
// orchestrator and the payment participant. Kept in sync with the
// payment-sandbox messaging package (v1 contract is intentionally small).
package messaging

// NATS subjects for the payment saga.
// Subjects are dotless: lib-async/watermill-nats auto-provisions a JetStream
// stream named from the topic, and JetStream stream names cannot contain ".".
// These are a fixed cross-service contract — keep saga-sandbox and
// payment-sandbox identical.
const (
	SubjectCaptureCmd   = "payment-capture-cmd"
	SubjectCaptureReply = "payment-capture-reply"
	SubjectRefundCmd    = "payment-refund-cmd"
	SubjectRefundReply  = "payment-refund-reply"
)

// Command kinds.
const (
	TypeCapture = "capture"
	TypeRefund  = "refund"
)

// Reply statuses.
const (
	StatusOK     = "OK"
	StatusFailed = "FAILED"
)

// PaymentPayload is the business payload of a payment command.
type PaymentPayload struct {
	AccountID string `json:"account_id"`
	// Amount is in minor currency units (e.g. cents); never a float.
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Command is the orchestrator -> participant message envelope.
type Command struct {
	MsgID          string         `json:"msg_id"`
	SagaID         string         `json:"saga_id"`
	IdempotencyKey string         `json:"idempotency_key"`
	Type           string         `json:"type"`
	Payload        PaymentPayload `json:"payload"`
}

// Reply is the participant -> orchestrator response envelope.
type Reply struct {
	MsgID     string `json:"msg_id"`
	SagaID    string `json:"saga_id"`
	InReplyTo string `json:"in_reply_to"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}
