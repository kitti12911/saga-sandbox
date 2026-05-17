package server

import (
	"context"
	"errors"

	sagav1 "github.com/kitti12911/saga-sandbox/gen/grpc/saga/v1"
	"github.com/kitti12911/saga-sandbox/internal/saga"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SagaHandler adapts the orchestrator to the generated gRPC service.
type SagaHandler struct {
	sagav1.UnimplementedSagaServiceServer

	orchestrator *saga.Orchestrator
}

// NewSagaHandler builds a SagaHandler over the given orchestrator.
func NewSagaHandler(orchestrator *saga.Orchestrator) *SagaHandler {
	return &SagaHandler{orchestrator: orchestrator}
}

// StartPayment validates the request and starts (or returns) the payment saga.
func (h *SagaHandler) StartPayment(
	ctx context.Context,
	req *sagav1.StartPaymentRequest,
) (*sagav1.StartPaymentResponse, error) {
	if req.GetIdempotencyKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	if req.GetAccountId() == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetCurrency() == "" {
		return nil, status.Error(codes.InvalidArgument, "currency is required")
	}

	view, err := h.orchestrator.StartPayment(ctx, saga.PaymentRequest{
		IdempotencyKey: req.GetIdempotencyKey(),
		AccountID:      req.GetAccountId(),
		Amount:         req.GetAmount(),
		Currency:       req.GetCurrency(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &sagav1.StartPaymentResponse{
		SagaId: view.SagaID,
		State:  view.State,
	}, nil
}

// GetSaga returns the current saga state.
func (h *SagaHandler) GetSaga(
	ctx context.Context,
	req *sagav1.GetSagaRequest,
) (*sagav1.GetSagaResponse, error) {
	if req.GetSagaId() == "" {
		return nil, status.Error(codes.InvalidArgument, "saga_id is required")
	}

	view, err := h.orchestrator.GetSaga(ctx, req.GetSagaId())
	if errors.Is(err, saga.ErrSagaNotFound) {
		return nil, status.Error(codes.NotFound, "saga not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &sagav1.GetSagaResponse{
		SagaId:      view.SagaID,
		State:       view.State,
		CurrentStep: view.CurrentStep,
		LastError:   view.LastError,
	}, nil
}
