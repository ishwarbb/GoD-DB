package node

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the node
type Metrics struct {
	// Operation counters
	OperationsTotal *prometheus.CounterVec
	
	// Latency histograms
	OperationDurations *prometheus.HistogramVec
	
	// Storage metrics
	KeysTotal prometheus.Gauge
	BytesStored prometheus.Gauge
	
	// Node metrics
	GossipMessagesTotal *prometheus.CounterVec
	HashRingChangesTotal prometheus.Counter
	
	// Replication metrics
	ReplicationOperationsTotal *prometheus.CounterVec
	QuorumFailuresTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		// Operation counters
		OperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "goddb_operations_total",
				Help: "Total number of operations processed",
			},
			[]string{"operation", "status"},
		),
		
		// Latency histograms
		OperationDurations: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "goddb_operation_duration_seconds",
				Help:    "Duration of operations in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		
		// Storage metrics
		KeysTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "goddb_keys_total",
				Help: "Total number of keys stored on this node",
			},
		),
		
		BytesStored: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "goddb_bytes_stored",
				Help: "Total bytes stored on this node",
			},
		),
		
		// Node metrics
		GossipMessagesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "goddb_gossip_messages_total",
				Help: "Total number of gossip messages",
			},
			[]string{"type"},
		),
		
		HashRingChangesTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "goddb_hashring_changes_total",
				Help: "Total number of hash ring topology changes",
			},
		),
		
		// Replication metrics
		ReplicationOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "goddb_replication_operations_total",
				Help: "Total number of replication operations",
			},
			[]string{"operation", "status"},
		),
		
		QuorumFailuresTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "goddb_quorum_failures_total",
				Help: "Total number of quorum failures",
			},
			[]string{"operation"},
		),
	}
} 