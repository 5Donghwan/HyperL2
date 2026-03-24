package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Registry struct {
	ingressTx       atomic.Uint64
	verifiedTx      atomic.Uint64
	dropCount       atomic.Uint64
	queueDepth      atomic.Int64
	finalizeLatency atomic.Uint64
	finalizeSamples atomic.Uint64
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) AddIngressTx(n uint64) {
	r.ingressTx.Add(n)
}

func (r *Registry) AddVerifiedTx(n uint64) {
	r.verifiedTx.Add(n)
}

func (r *Registry) IncDrop(n uint64) {
	r.dropCount.Add(n)
}

func (r *Registry) SetQueueDepth(depth int64) {
	r.queueDepth.Store(depth)
}

func (r *Registry) ObserveFinalizeLatencyMs(ms uint64) {
	r.finalizeLatency.Add(ms)
	r.finalizeSamples.Add(1)
}

func (r *Registry) IngressTx() uint64 {
	return r.ingressTx.Load()
}

func (r *Registry) VerifiedTx() uint64 {
	return r.verifiedTx.Load()
}

func (r *Registry) DropCount() uint64 {
	return r.dropCount.Load()
}

func (r *Registry) QueueDepth() int64 {
	return r.queueDepth.Load()
}

func (r *Registry) AverageFinalizeLatencyMs() float64 {
	samples := r.finalizeSamples.Load()
	if samples == 0 {
		return 0
	}
	return float64(r.finalizeLatency.Load()) / float64(samples)
}

func (r *Registry) PrometheusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "# TYPE cert_l2_ingress_tps counter\n")
		_, _ = fmt.Fprintf(w, "cert_l2_ingress_tps %d\n", r.IngressTx())
		_, _ = fmt.Fprintf(w, "# TYPE cert_l1_verified_tps counter\n")
		_, _ = fmt.Fprintf(w, "cert_l1_verified_tps %d\n", r.VerifiedTx())
		_, _ = fmt.Fprintf(w, "# TYPE cert_queue_depth gauge\n")
		_, _ = fmt.Fprintf(w, "cert_queue_depth %d\n", r.QueueDepth())
		_, _ = fmt.Fprintf(w, "# TYPE cert_drop_count counter\n")
		_, _ = fmt.Fprintf(w, "cert_drop_count %d\n", r.DropCount())
		_, _ = fmt.Fprintf(w, "# TYPE cert_finalize_latency_ms gauge\n")
		_, _ = fmt.Fprintf(w, "cert_finalize_latency_ms %.3f\n", r.AverageFinalizeLatencyMs())
	})
}
