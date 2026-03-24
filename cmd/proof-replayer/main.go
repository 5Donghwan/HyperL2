package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"

	"hyperl2/internal/pipeline"
)

func main() {
	var (
		listen       = flag.String("listen", ":9083", "HTTP listen address")
		proofSeed    = flag.String("proof-seed", "hyperl2-cert-seed", "Deterministic proof seed")
		proofDataset = flag.String("proof-dataset", "", "Optional proof dataset jsonl")
		l1Endpoint   = flag.String("l1-endpoint", "http://127.0.0.1:9090/submit-certified-block", "L1 cert submit endpoint")
		badProofAt   = flag.Uint("bad-proof-at", 0, "Optional block index to inject proof error")
	)
	flag.Parse()

	book := pipeline.NewProofBook(*proofSeed)
	if *proofDataset != "" {
		if err := book.LoadJSONL(*proofDataset); err != nil {
			fmt.Fprintf(os.Stderr, "load proof dataset: %v\n", err)
			os.Exit(1)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/block-commit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var commit pipeline.BlockCommitEnvelope
		if err := json.NewDecoder(r.Body).Decode(&commit); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		proofBytes := book.ProofForBlock(commit.BlockIndex)
		proofCopy := make([]byte, len(proofBytes))
		copy(proofCopy, proofBytes)
		if *badProofAt != 0 && uint32(*badProofAt) == commit.BlockIndex && len(proofCopy) > 0 {
			proofCopy[0] ^= 0xff
		}

		submit := pipeline.L1SubmitEnvelope{
			Phase:             commit.Phase,
			BlockIndex:        commit.BlockIndex,
			CommitmentHashHex: commit.CommitmentHashHex,
			ProofHex:          hex.EncodeToString(proofCopy),
			TxCount:           commit.TxCount,
			TxPayloadHex:      commit.TxPayloadHex,
			CreatedAtUnixMs:   commit.CreatedAtUnixMs,
		}
		if err := postJSON(*l1Endpoint, submit); err != nil {
			http.Error(w, fmt.Sprintf("submit to l1 failed: %v", err), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	fmt.Printf("proof-replayer listening on %s\n", *listen)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func postJSON(url string, payload any) error {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("status=%d", resp.StatusCode)
	}
	return nil
}
