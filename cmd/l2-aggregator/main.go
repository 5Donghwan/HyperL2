package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"hyperl2/internal/frame"
	"hyperl2/internal/pipeline"
)

type state struct {
	phase     string
	createdAt int64
	txCount   uint32
	lanes     map[uint16][32]byte
	txs       []frame.Tx12
}

func main() {
	var (
		listen        = flag.String("listen", ":9082", "HTTP listen address")
		lanes         = flag.Int("lanes", 4, "Expected lane count")
		proofEndpoint = flag.String("proof-endpoint", "", "Optional proof-replayer endpoint, e.g. http://127.0.0.1:9083/block-commit")
	)
	flag.Parse()

	if *lanes <= 0 {
		fmt.Fprintln(os.Stderr, "lanes must be > 0")
		os.Exit(1)
	}

	var (
		mu     sync.Mutex
		states = map[uint32]*state{}
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/lane-batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var env pipeline.LaneBatchEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		batchBytes, err := hex.DecodeString(env.BatchHex)
		if err != nil {
			http.Error(w, "invalid batch_hex", http.StatusBadRequest)
			return
		}
		var bf frame.BatchFrameV1
		if err := bf.UnmarshalBinary(batchBytes); err != nil {
			http.Error(w, "invalid batch payload", http.StatusBadRequest)
			return
		}
		if bf.BlockIndex != env.BlockIndex || bf.LaneID != env.LaneID {
			http.Error(w, "envelope mismatch", http.StatusBadRequest)
			return
		}

		laneHash := bf.CommitmentHash()
		var maybeCommit *pipeline.BlockCommitEnvelope

		mu.Lock()
		st, ok := states[env.BlockIndex]
		if !ok {
			st = &state{
				phase:     env.Phase,
				createdAt: env.CreatedAtUnixMs,
				lanes:     make(map[uint16][32]byte, *lanes),
				txs:       make([]frame.Tx12, 0, len(bf.Payload)*(*lanes)),
			}
			states[env.BlockIndex] = st
		}
		if st.phase == "" {
			st.phase = env.Phase
		}
		if st.createdAt == 0 {
			st.createdAt = env.CreatedAtUnixMs
		}
		st.lanes[env.LaneID] = laneHash
		for _, tx := range bf.Payload {
			if tx.Amount == 0 {
				continue
			}
			st.txCount++
			st.txs = append(st.txs, tx)
		}
		if len(st.lanes) == *lanes {
			commitment := pipeline.CommitmentFromTxs(env.BlockIndex, st.txs)
			now := st.createdAt
			if now == 0 {
				now = time.Now().UnixMilli()
			}
			maybeCommit = &pipeline.BlockCommitEnvelope{
				Phase:             st.phase,
				BlockIndex:        env.BlockIndex,
				CommitmentHashHex: hex.EncodeToString(commitment[:]),
				TxCount:           st.txCount,
				TxPayloadHex:      hex.EncodeToString(frame.EncodeTxSlice(st.txs)),
				CreatedAtUnixMs:   now,
			}
			delete(states, env.BlockIndex)
		}
		mu.Unlock()

		if maybeCommit != nil {
			if *proofEndpoint != "" {
				if err := postJSON(*proofEndpoint, *maybeCommit); err != nil {
					http.Error(w, "forward to proof-replayer failed", http.StatusBadGateway)
					return
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	fmt.Printf("l2-aggregator listening on %s\n", *listen)
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
