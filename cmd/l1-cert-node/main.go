package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"hyperl2/internal/frame"
	"hyperl2/internal/metrics"
	"hyperl2/internal/pipeline"
)

func main() {
	var (
		listen    = flag.String("listen", ":9090", "HTTP listen address")
		proofSeed = flag.String("proof-seed", "hyperl2-cert-seed", "Deterministic proof seed")
	)
	flag.Parse()

	m := metrics.NewRegistry()
	node := pipeline.NewL1CertNode(*proofSeed, m)

	mux := http.NewServeMux()
	mux.Handle("/metrics", m.PrometheusHandler())
	mux.HandleFunc("/submit-certified-block", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload pipeline.L1SubmitEnvelope
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		commitBytes, err := hex.DecodeString(payload.CommitmentHashHex)
		if err != nil || len(commitBytes) != 32 {
			http.Error(w, "invalid commitment hash", http.StatusBadRequest)
			return
		}
		proofBytes, err := hex.DecodeString(payload.ProofHex)
		if err != nil {
			http.Error(w, "invalid proof hex", http.StatusBadRequest)
			return
		}
		txPayloadBytes, err := hex.DecodeString(payload.TxPayloadHex)
		if err != nil {
			http.Error(w, "invalid tx payload hex", http.StatusBadRequest)
			return
		}
		txs, err := frame.DecodeTxSlice(txPayloadBytes)
		if err != nil {
			http.Error(w, "invalid tx payload", http.StatusBadRequest)
			return
		}
		var commitment [32]byte
		copy(commitment[:], commitBytes)
		createdAt := time.UnixMilli(payload.CreatedAtUnixMs)
		if payload.CreatedAtUnixMs == 0 {
			createdAt = time.Now()
		}
		proof := pipeline.BuildProofRecord(payload.BlockIndex, commitment, proofBytes)
		resp := node.SubmitCertifiedBlock(pipeline.SubmitRequest{
			Phase:          payload.Phase,
			BlockIndex:     payload.BlockIndex,
			CommitmentHash: commitment,
			Proof:          proof,
			TxCount:        payload.TxCount,
			Txs:            txs,
			CreatedAt:      createdAt,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	fmt.Printf("l1-cert-node listening on %s\n", *listen)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
