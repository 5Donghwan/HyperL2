package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hyperl2/internal/config"
)

func TestRunnerPassesInHappyPath(t *testing.T) {
	cfg := config.Config{
		Name:              "test-happy",
		BlockTimeMillis:   100,
		GasLimit:          45_000_000,
		Lanes:             4,
		MaxLanes:          8,
		QueueCapacity:     1024,
		CertMode:          true,
		SkipState:         true,
		SkipSignature:     true,
		ProofSeed:         "seed",
		Warmup:            config.Phase{TPS: 200, DurationSeconds: 1},
		Certification:     config.Phase{TPS: 500, DurationSeconds: 2},
		MetricsListenAddr: ":0",
		ReportDir:         filepath.Join(t.TempDir(), "reports"),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate cfg: %v", err)
	}

	cfgPath := writeConfig(t, cfg)
	runner, err := NewRunner(cfg, cfgPath)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Report.Pass {
		t.Fatalf("expected pass report, got %+v", result.Report)
	}
	if result.Report.DropCount != 0 {
		t.Fatalf("expected no drops")
	}
}

func TestRunnerFailsOnBadProof(t *testing.T) {
	cfg := config.Config{
		Name:              "test-bad-proof",
		BlockTimeMillis:   100,
		GasLimit:          45_000_000,
		Lanes:             4,
		MaxLanes:          8,
		QueueCapacity:     1024,
		CertMode:          true,
		SkipState:         true,
		SkipSignature:     true,
		ProofSeed:         "seed",
		Warmup:            config.Phase{TPS: 200, DurationSeconds: 1},
		Certification:     config.Phase{TPS: 500, DurationSeconds: 2},
		MetricsListenAddr: ":0",
		ReportDir:         filepath.Join(t.TempDir(), "reports"),
		Fault:             config.FaultInjection{BadProofAt: 10},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate cfg: %v", err)
	}
	cfgPath := writeConfig(t, cfg)
	runner, err := NewRunner(cfg, cfgPath)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Report.Pass {
		t.Fatalf("expected fail report when bad proof injected")
	}
	if result.Report.ProofFailures == 0 {
		t.Fatalf("expected proof failures > 0")
	}
}

func writeConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cfg.json")
	payload, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	return path
}
