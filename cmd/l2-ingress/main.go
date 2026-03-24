package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"hyperl2/internal/pipeline"
)

func main() {
	var (
		endpoint    = flag.String("endpoint", "", "Target stream endpoint, e.g. tcp://127.0.0.1:7001 or unix:///tmp/hyperl2.sock")
		blocks      = flag.Int("blocks", 10, "Number of blocks to stream")
		tps         = flag.Int("tps", 20000, "Target tx per second")
		blockTimeMs = flag.Int("block-time-ms", 1000, "Block interval in milliseconds")
		startBlock  = flag.Uint("start-block", 0, "Start block index")
	)
	flag.Parse()

	if *endpoint == "" {
		fmt.Fprintln(os.Stderr, "--endpoint is required")
		os.Exit(1)
	}
	if *blocks <= 0 || *tps <= 0 || *blockTimeMs <= 0 {
		fmt.Fprintln(os.Stderr, "blocks, tps, block-time-ms must be > 0")
		os.Exit(1)
	}

	conn, err := dial(*endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial endpoint: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	interval := time.Duration(*blockTimeMs) * time.Millisecond
	txPerBlock := int(float64(*tps) * interval.Seconds())
	nextTick := time.Now()
	for i := 0; i < *blocks; i++ {
		blockIndex := uint32(*startBlock) + uint32(i)
		txs := pipeline.GenerateSyntheticTxs(blockIndex, txPerBlock)
		for _, tx := range txs {
			enc := tx.MarshalBinary()
			if _, err := conn.Write(enc[:]); err != nil {
				fmt.Fprintf(os.Stderr, "stream write failed at block %d: %v\n", blockIndex, err)
				os.Exit(1)
			}
		}
		fmt.Printf("streamed block=%d tx=%d\n", blockIndex, txPerBlock)

		nextTick = nextTick.Add(interval)
		if sleep := time.Until(nextTick); sleep > 0 {
			time.Sleep(sleep)
		}
	}
}

func dial(endpoint string) (net.Conn, error) {
	switch {
	case strings.HasPrefix(endpoint, "tcp://"):
		return net.Dial("tcp", strings.TrimPrefix(endpoint, "tcp://"))
	case strings.HasPrefix(endpoint, "unix://"):
		return net.Dial("unix", strings.TrimPrefix(endpoint, "unix://"))
	default:
		return nil, fmt.Errorf("unsupported endpoint format %q", endpoint)
	}
}
