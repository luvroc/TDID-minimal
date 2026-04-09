package relay

import (
	"context"

	"tdid-final/host/chain"
)

type ReceiptWorker struct {
	audit AuditClient
}

func NewReceiptWorker(audit AuditClient) *ReceiptWorker {
	return &ReceiptWorker{audit: audit}
}

func (w *ReceiptWorker) Reconcile(ctx context.Context, traceID string, transferID string) error {
	if w == nil || w.audit == nil {
		return nil
	}
	_, err := w.audit.MatchTransfer(ctx, chain.AuditMatchRequest{TraceID: traceID, TransferID: transferID})
	return err
}

func (w *ReceiptWorker) SubmitSource(ctx context.Context, traceID string, transferID string, receiptHash string) error {
	if w == nil || w.audit == nil {
		return nil
	}
	_, err := w.audit.SubmitSourceReceipt(ctx, chain.AuditReceiptRequest{TraceID: traceID, TransferID: transferID, ReceiptHash: receiptHash})
	return err
}

func (w *ReceiptWorker) SubmitTarget(ctx context.Context, traceID string, transferID string, receiptHash string) error {
	if w == nil || w.audit == nil {
		return nil
	}
	_, err := w.audit.SubmitTargetReceipt(ctx, chain.AuditReceiptRequest{TraceID: traceID, TransferID: transferID, ReceiptHash: receiptHash})
	return err
}
