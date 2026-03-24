package frame

import (
	"testing"
)

func TestTx12RoundTrip(t *testing.T) {
	in := Tx12{SenderIndex: 1, ReceiverIndex: 2, Amount: 3}
	enc := in.MarshalBinary()
	out, err := UnmarshalTx12(enc[:])
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: got %+v want %+v", out, in)
	}
}

func TestBatchFrameV1RoundTrip(t *testing.T) {
	in := BatchFrameV1{
		Flags:      7,
		BlockIndex: 42,
		LaneID:     3,
		Payload: []Tx12{
			{SenderIndex: 10, ReceiverIndex: 11, Amount: 12},
			{SenderIndex: 20, ReceiverIndex: 21, Amount: 22},
		},
	}
	payload := in.MarshalBinary()
	var out BatchFrameV1
	if err := out.UnmarshalBinary(payload); err != nil {
		t.Fatalf("unmarshal batch failed: %v", err)
	}
	if out.BlockIndex != in.BlockIndex || out.LaneID != in.LaneID || out.Flags != in.Flags {
		t.Fatalf("header mismatch")
	}
	if len(out.Payload) != len(in.Payload) {
		t.Fatalf("payload len mismatch")
	}
	for i := range out.Payload {
		if out.Payload[i] != in.Payload[i] {
			t.Fatalf("payload[%d] mismatch got %+v want %+v", i, out.Payload[i], in.Payload[i])
		}
	}
}

func TestBatchCommitmentDeterministic(t *testing.T) {
	bf := BatchFrameV1{
		BlockIndex: 1,
		LaneID:     0,
		Payload:    []Tx12{{SenderIndex: 1, ReceiverIndex: 2, Amount: 3}},
	}
	h1 := bf.CommitmentHash()
	h2 := bf.CommitmentHash()
	if h1 != h2 {
		t.Fatalf("commitment hash is not deterministic")
	}
}
