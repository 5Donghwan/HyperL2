package pipeline

import (
	"testing"
	"time"

	"hyperl2/internal/frame"
	"hyperl2/internal/metrics"
)

func TestL1AcceptsValidProofAndIdempotentDuplicate(t *testing.T) {
	m := metrics.NewRegistry()
	node := NewL1CertNode("seed", m)

	txs := []frame.Tx12{
		{SenderIndex: 11, ReceiverIndex: 22, Amount: 25},
		{SenderIndex: 11, ReceiverIndex: 33, Amount: 10},
	}
	commitment := CommitmentFromTxs(7, txs)
	proof := BuildProofRecord(7, commitment, GenerateProofBytes("seed", 7))
	req := SubmitRequest{
		Phase:          "certification",
		BlockIndex:     7,
		CommitmentHash: commitment,
		Proof:          proof,
		TxCount:        uint32(len(txs)),
		Txs:            txs,
		CreatedAt:      time.Now(),
	}

	resp := node.SubmitCertifiedBlock(req)
	if !resp.Accepted || !resp.ProofOK {
		t.Fatalf("expected accepted proof, got %+v", resp)
	}
	if m.VerifiedTx() != uint64(len(txs)) {
		t.Fatalf("verified tx mismatch: %d", m.VerifiedTx())
	}
	if got := node.Balance(11); got != -35 {
		t.Fatalf("balance mismatch for sender: got %d", got)
	}
	if got := node.Balance(22); got != 25 {
		t.Fatalf("balance mismatch for receiver: got %d", got)
	}
	if got := node.Balance(33); got != 10 {
		t.Fatalf("balance mismatch for receiver2: got %d", got)
	}
	if got := node.StateVersion(); got != 1 {
		t.Fatalf("state version mismatch: got %d", got)
	}

	dup := node.SubmitCertifiedBlock(req)
	if !dup.Accepted || !dup.Duplicate {
		t.Fatalf("expected duplicate accepted, got %+v", dup)
	}
	if m.VerifiedTx() != uint64(len(txs)) {
		t.Fatalf("verified tx should not increase on duplicate")
	}
	if got := node.Balance(11); got != -35 {
		t.Fatalf("duplicate should not reapply state update, got sender=%d", got)
	}
	if got := node.Balance(22); got != 25 {
		t.Fatalf("duplicate should not reapply state update, got receiver=%d", got)
	}
	if got := node.Balance(33); got != 10 {
		t.Fatalf("duplicate should not reapply state update, got receiver2=%d", got)
	}
	if got := node.StateVersion(); got != 1 {
		t.Fatalf("duplicate should not bump state version, got %d", got)
	}
}

func TestL1RejectsBadProof(t *testing.T) {
	m := metrics.NewRegistry()
	node := NewL1CertNode("seed", m)

	txs := []frame.Tx12{{SenderIndex: 99, ReceiverIndex: 77, Amount: 100}}
	commitment := CommitmentFromTxs(8, txs)
	badProof := BuildProofRecord(8, commitment, []byte("bad-proof"))
	resp := node.SubmitCertifiedBlock(SubmitRequest{
		Phase:          "certification",
		BlockIndex:     8,
		CommitmentHash: commitment,
		Proof:          badProof,
		TxCount:        uint32(len(txs)),
		Txs:            txs,
		CreatedAt:      time.Now(),
	})
	if resp.Accepted || resp.ProofOK {
		t.Fatalf("expected rejection for bad proof, got %+v", resp)
	}
	if m.VerifiedTx() != 0 {
		t.Fatalf("verified tx should stay 0")
	}
	if got := node.StateVersion(); got != 0 {
		t.Fatalf("state version should not change on bad proof, got %d", got)
	}
	if got := node.Balance(99); got != 0 {
		t.Fatalf("state should not apply on bad proof, got sender=%d", got)
	}
	if got := node.Balance(77); got != 0 {
		t.Fatalf("state should not apply on bad proof, got receiver=%d", got)
	}
}

func TestL1RejectsPayloadCommitmentMismatch(t *testing.T) {
	m := metrics.NewRegistry()
	node := NewL1CertNode("seed", m)

	txs := []frame.Tx12{{SenderIndex: 1, ReceiverIndex: 2, Amount: 5}}
	commitment := CommitmentFromTxs(9, txs)
	proof := BuildProofRecord(9, commitment, GenerateProofBytes("seed", 9))

	modified := []frame.Tx12{{SenderIndex: 1, ReceiverIndex: 2, Amount: 6}}
	resp := node.SubmitCertifiedBlock(SubmitRequest{
		Phase:          "certification",
		BlockIndex:     9,
		CommitmentHash: commitment,
		Proof:          proof,
		TxCount:        uint32(len(modified)),
		Txs:            modified,
		CreatedAt:      time.Now(),
	})
	if resp.Accepted {
		t.Fatalf("expected payload mismatch rejection")
	}
	if got := node.StateVersion(); got != 0 {
		t.Fatalf("state version should stay 0, got %d", got)
	}
}
