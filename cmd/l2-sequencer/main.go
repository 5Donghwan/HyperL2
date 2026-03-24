package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"hyperl2/internal/frame"
	"hyperl2/internal/pipeline"
)

func main() {
	var (
		listenSpec         = flag.String("listen-stream", "tcp://:7001", "Stream listener spec, tcp://host:port or unix:///path")
		aggregatorEndpoint = flag.String("aggregator-endpoint", "http://127.0.0.1:9082/lane-batch", "Aggregator endpoint")
		lanes              = flag.Int("lanes", 4, "Lane count")
		blockTimeMs        = flag.Int("block-time-ms", 1000, "Block interval in milliseconds")
		phase              = flag.String("phase", "certification", "Phase label")
		startBlock         = flag.Uint("start-block", 0, "Starting block index")
	)
	flag.Parse()

	if *lanes <= 0 || *blockTimeMs <= 0 {
		fmt.Fprintln(os.Stderr, "lanes and block-time-ms must be > 0")
		os.Exit(1)
	}

	ln, err := listen(*listenSpec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	fmt.Printf("l2-sequencer stream listen on %s\n", *listenSpec)

	txCh := make(chan frame.Tx12, 131072)
	var dropped atomic.Uint64

	go acceptLoop(ln, txCh, &dropped)

	interval := time.Duration(*blockTimeMs) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	currentBlock := uint32(*startBlock)
	buffer := make([]frame.Tx12, 0, 4096)
	var rr uint64

	for {
		select {
		case tx := <-txCh:
			buffer = append(buffer, tx)
		case <-ticker.C:
			laned := make([][]frame.Tx12, *lanes)
			for _, tx := range buffer {
				lane := int(rr % uint64(*lanes))
				rr++
				laned[lane] = append(laned[lane], tx)
			}
			createdAt := time.Now().UnixMilli()
			for laneID := 0; laneID < *lanes; laneID++ {
				bf := pipeline.BuildLaneBatchFrame(currentBlock, uint16(laneID), laned[laneID])
				env := pipeline.LaneBatchEnvelope{
					Phase:           *phase,
					BlockIndex:      currentBlock,
					LaneID:          uint16(laneID),
					BatchHex:        hex.EncodeToString(bf.MarshalBinary()),
					CreatedAtUnixMs: createdAt,
				}
				if err := postJSON(*aggregatorEndpoint, env); err != nil {
					fmt.Fprintf(os.Stderr, "forward lane=%d block=%d failed: %v\n", laneID, currentBlock, err)
				}
			}
			fmt.Printf("sealed block=%d tx=%d dropped=%d\n", currentBlock, len(buffer), dropped.Load())
			currentBlock++
			buffer = buffer[:0]
		}
	}
}

func acceptLoop(ln net.Listener, txCh chan<- frame.Tx12, dropped *atomic.Uint64) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "accept error: %v\n", err)
			continue
		}
		go readTxStream(conn, txCh, dropped)
	}
}

func readTxStream(conn net.Conn, txCh chan<- frame.Tx12, dropped *atomic.Uint64) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	buf := make([]byte, 12)
	for {
		if _, err := io.ReadFull(reader, buf); err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "stream read error: %v\n", err)
			}
			return
		}
		tx, err := frame.UnmarshalTx12(buf)
		if err != nil {
			dropped.Add(1)
			continue
		}
		select {
		case txCh <- tx:
		default:
			dropped.Add(1)
		}
	}
}

func listen(spec string) (net.Listener, error) {
	switch {
	case strings.HasPrefix(spec, "tcp://"):
		return net.Listen("tcp", strings.TrimPrefix(spec, "tcp://"))
	case strings.HasPrefix(spec, "unix://"):
		path := strings.TrimPrefix(spec, "unix://")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		_ = os.Remove(path)
		return net.Listen("unix", path)
	default:
		return nil, fmt.Errorf("unsupported listen spec %q", spec)
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
