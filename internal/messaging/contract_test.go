package messaging

import (
	"encoding/json"
	"testing"
)

func TestCommandRoundTrip(t *testing.T) {
	t.Parallel()

	in := Command{
		MsgID:          "m-1",
		SagaID:         "s-1",
		IdempotencyKey: "idem-1",
		Type:           TypeCapture,
		Payload:        PaymentPayload{AccountID: "acc-1", Amount: 1500, Currency: "THB"},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out Command
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: got %+v, want %+v", out, in)
	}
}

func TestReplyRoundTrip(t *testing.T) {
	t.Parallel()

	in := Reply{
		MsgID:     "r-m-1",
		SagaID:    "s-1",
		InReplyTo: "m-1",
		Status:    StatusOK,
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out Reply
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: got %+v, want %+v", out, in)
	}
}
