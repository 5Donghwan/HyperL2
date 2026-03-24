package pipeline

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Report struct {
	ConfigName              string     `json:"config_name"`
	StartedAt               string     `json:"started_at"`
	FinishedAt              string     `json:"finished_at"`
	Host                    string     `json:"host"`
	GoVersion               string     `json:"go_version"`
	ConfigSHA256            string     `json:"config_sha256"`
	BinarySHA256            string     `json:"binary_sha256"`
	BlockTimeMillis         int        `json:"block_time_millis"`
	GasLimit                uint64     `json:"gas_limit"`
	Lanes                   int        `json:"lanes"`
	ExpectedCertificationTx uint64     `json:"expected_certification_tx"`
	CertificationSeconds    int        `json:"certification_seconds"`
	CertificationTPSGoal    int        `json:"certification_tps_goal"`
	CertificationVerifiedTx uint64     `json:"certification_verified_tx"`
	CertificationAvgTPS     float64    `json:"certification_avg_tps"`
	CertifiedBlocks         int        `json:"certified_blocks"`
	ProofFailures           int        `json:"proof_failures"`
	DropCount               uint64     `json:"drop_count"`
	AverageFinalizeMs       float64    `json:"average_finalize_ms"`
	Pass                    bool       `json:"pass"`
	Assumptions             []string   `json:"assumptions"`
	Blocks                  []BlockLog `json:"blocks"`
}

func WriteReport(dir string, report Report) (string, error) {
	if err := EnsureReportDir(dir); err != nil {
		return "", fmt.Errorf("ensure report dir: %w", err)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	base := filepath.Join(dir, "cert-report-"+ts)

	jsonPath := base + ".json"
	if err := writeJSON(jsonPath, report); err != nil {
		return "", err
	}
	csvPath := base + ".csv"
	if err := writeCSV(csvPath, report.Blocks); err != nil {
		return "", err
	}
	mdPath := base + ".md"
	if err := writeMarkdown(mdPath, report, jsonPath, csvPath); err != nil {
		return "", err
	}
	return jsonPath, nil
}

func writeJSON(path string, report Report) error {
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write json report: %w", err)
	}
	return nil
}

func writeCSV(path string, blocks []BlockLog) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create csv report: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"phase", "block_index", "tx_count", "proof_verified", "finalized", "duplicate", "state_applied", "state_version", "delta_count", "error", "finalize_latency_ms", "commitment_hash",
	}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, b := range blocks {
		record := []string{
			b.Phase,
			strconv.FormatUint(uint64(b.BlockIndex), 10),
			strconv.FormatUint(uint64(b.TxCount), 10),
			strconv.FormatBool(b.ProofVerified),
			strconv.FormatBool(b.Finalized),
			strconv.FormatBool(b.Duplicate),
			strconv.FormatBool(b.StateApplied),
			strconv.FormatUint(b.StateVersion, 10),
			strconv.Itoa(b.DeltaCount),
			b.Error,
			fmt.Sprintf("%.3f", b.FinalizeMs),
			b.CommitmentHash,
		}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("write csv record: %w", err)
		}
	}
	return nil
}

func writeMarkdown(path string, report Report, jsonPath, csvPath string) error {
	content := fmt.Sprintf(`# HyperL2 Certification Report

- Config: %s
- Host: %s
- Pass: %t
- Certification TPS Goal: %d
- Certification Avg Verified TPS: %.3f
- Certification Verified TX: %d / %d
- Drop Count: %d
- Proof Failures: %d
- Average Finalize Latency (ms): %.3f
- Config SHA256: %s
- Binary SHA256: %s
- JSON Report: %s
- CSV Report: %s

## Assumptions
%s
`,
		report.ConfigName,
		report.Host,
		report.Pass,
		report.CertificationTPSGoal,
		report.CertificationAvgTPS,
		report.CertificationVerifiedTx,
		report.ExpectedCertificationTx,
		report.DropCount,
		report.ProofFailures,
		report.AverageFinalizeMs,
		report.ConfigSHA256,
		report.BinarySHA256,
		jsonPath,
		csvPath,
		markdownBullets(report.Assumptions),
	)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}
	return nil
}

func markdownBullets(lines []string) string {
	if len(lines) == 0 {
		return "- (none)"
	}
	out := ""
	for _, line := range lines {
		out += "- " + line + "\n"
	}
	return out
}
