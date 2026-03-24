package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"hyperl2/internal/config"
	"hyperl2/internal/pipeline"
)

func main() {
	var (
		configPath = flag.String("config", "configs/dev-air/config.json", "Path to configuration JSON")
		mode       = flag.String("mode", "run", "Mode: run|generate-datasets")
		timeoutSec = flag.Int("timeout-sec", 0, "Optional hard timeout in seconds (0 disables)")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}

	switch *mode {
	case "generate-datasets":
		txDir := filepath.Join("datasets", "tx")
		proofDir := filepath.Join("datasets", "proofs")
		paths, err := pipeline.GenerateDatasets(cfg, txDir, proofDir)
		if err != nil {
			fatalf("generate datasets: %v", err)
		}
		fmt.Printf("generated tx blob: %s\n", paths.TxBlobPath)
		fmt.Printf("generated tx manifest: %s\n", paths.TxManifestPath)
		fmt.Printf("generated proof dataset: %s\n", paths.ProofDatasetPath)
		return
	case "run":
		// continue
	default:
		fatalf("unsupported mode %q", *mode)
	}

	runner, err := pipeline.NewRunner(cfg, *configPath)
	if err != nil {
		fatalf("init runner: %v", err)
	}

	ctx := context.Background()
	if *timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*timeoutSec)*time.Second)
		defer cancel()
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	result, err := runner.Run(ctx)
	if err != nil {
		fatalf("run benchmark: %v", err)
	}

	fmt.Printf("report: %s\n", result.Path)
	fmt.Printf("pass: %t\n", result.Report.Pass)
	fmt.Printf("cert avg verified tps: %.3f\n", result.Report.CertificationAvgTPS)
	fmt.Printf("cert verified tx: %d / expected %d\n",
		result.Report.CertificationVerifiedTx,
		result.Report.ExpectedCertificationTx,
	)
	fmt.Printf("drop count: %d\n", result.Report.DropCount)
	fmt.Printf("proof failures: %d\n", result.Report.ProofFailures)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
