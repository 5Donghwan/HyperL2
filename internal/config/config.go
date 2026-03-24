package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Phase struct {
	TPS             int `json:"tps"`
	DurationSeconds int `json:"duration_seconds"`
}

type FaultInjection struct {
	PauseAtBlock uint32 `json:"pause_at_block"`
	PauseMillis  int    `json:"pause_millis"`
	BadProofAt   uint32 `json:"bad_proof_at"`
}

type Config struct {
	Name              string         `json:"name"`
	BlockTimeMillis   int            `json:"block_time_millis"`
	GasLimit          uint64         `json:"gas_limit"`
	Lanes             int            `json:"lanes"`
	MaxLanes          int            `json:"max_lanes"`
	QueueCapacity     uint64         `json:"queue_capacity"`
	CertMode          bool           `json:"cert_mode"`
	SkipState         bool           `json:"skip_state"`
	SkipSignature     bool           `json:"skip_signature"`
	ProofSeed         string         `json:"proof_seed"`
	Warmup            Phase          `json:"warmup"`
	Certification     Phase          `json:"certification"`
	MetricsListenAddr string         `json:"metrics_listen_addr"`
	ReportDir         string         `json:"report_dir"`
	TxDatasetPath     string         `json:"tx_dataset_path"`
	ProofDatasetPath  string         `json:"proof_dataset_path"`
	Fault             FaultInjection `json:"fault"`
}

func Load(path string) (Config, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.BlockTimeMillis <= 0 {
		return fmt.Errorf("block_time_millis must be > 0")
	}
	if c.Lanes <= 0 || c.Lanes > c.MaxLanes {
		return fmt.Errorf("lanes must be in [1, max_lanes]")
	}
	if c.MaxLanes <= 0 {
		return fmt.Errorf("max_lanes must be > 0")
	}
	if c.QueueCapacity < 2 || c.QueueCapacity&(c.QueueCapacity-1) != 0 {
		return fmt.Errorf("queue_capacity must be power-of-two and >=2")
	}
	if c.Warmup.TPS < 0 || c.Certification.TPS <= 0 {
		return fmt.Errorf("tps values are invalid")
	}
	if c.Warmup.DurationSeconds < 0 || c.Certification.DurationSeconds <= 0 {
		return fmt.Errorf("duration values are invalid")
	}
	if c.ReportDir == "" {
		return fmt.Errorf("report_dir is required")
	}
	if c.MetricsListenAddr == "" {
		return fmt.Errorf("metrics_listen_addr is required")
	}
	return nil
}

func (c Config) BlockInterval() time.Duration {
	return time.Duration(c.BlockTimeMillis) * time.Millisecond
}

func (c Config) BlocksForPhase(p Phase) int {
	if p.DurationSeconds == 0 {
		return 0
	}
	return int((time.Duration(p.DurationSeconds) * time.Second) / c.BlockInterval())
}

func (c Config) TxPerBlock(tps int) int {
	if tps <= 0 {
		return 0
	}
	return int(float64(tps) * c.BlockInterval().Seconds())
}
