package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry holds all metrics for the 5Glos Gateway
type Registry struct {
	// Connection metrics
	ActiveConnections prometheus.Gauge
	TotalConnections  prometheus.Counter
	ConnectionErrors  prometheus.Counter

	// AMF metrics
	AMFInstances    prometheus.Gauge
	AMFConnections  *prometheus.GaugeVec
	AMFHealthStatus *prometheus.GaugeVec

	// UE metrics
	ActiveUESessions  prometheus.Gauge
	TotalUESessions   prometheus.Counter
	UESessionDuration prometheus.Histogram

	// NGAP metrics
	NGAPMessages       *prometheus.CounterVec
	NGAPMessageSize    prometheus.Histogram
	NGAPProcessingTime prometheus.Histogram

	// Load balancer metrics
	LoadBalancerDecisions *prometheus.CounterVec
	SessionAffinityHits   prometheus.Counter
	SessionAffinityMisses prometheus.Counter
}

// NewRegistry creates and registers all metrics
func NewRegistry() *Registry {
	return &Registry{
		// Connection metrics
		ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "fglos_active_connections_total",
			Help: "Number of active SCTP connections",
		}),
		TotalConnections: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fglos_total_connections_total",
			Help: "Total number of SCTP connections handled",
		}),
		ConnectionErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fglos_connection_errors_total",
			Help: "Total number of connection errors",
		}),

		// AMF metrics
		AMFInstances: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "fglos_amf_instances_total",
			Help: "Number of discovered AMF instances",
		}),
		AMFConnections: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "fglos_amf_connections",
				Help: "Number of connections per AMF instance",
			},
			[]string{"amf_id", "amf_address"},
		),
		AMFHealthStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "fglos_amf_health_status",
				Help: "Health status of AMF instances (1=healthy, 0=unhealthy)",
			},
			[]string{"amf_id", "amf_address"},
		),

		// UE metrics
		ActiveUESessions: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "fglos_active_ue_sessions_total",
			Help: "Number of active UE sessions",
		}),
		TotalUESessions: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fglos_total_ue_sessions_total",
			Help: "Total number of UE sessions handled",
		}),
		UESessionDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "fglos_ue_session_duration_seconds",
			Help:    "Duration of UE sessions in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		}),

		// NGAP metrics
		NGAPMessages: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "fglos_ngap_messages_total",
				Help: "Total number of NGAP messages by type and direction",
			},
			[]string{"message_type", "direction"},
		),
		NGAPMessageSize: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "fglos_ngap_message_size_bytes",
			Help:    "Size of NGAP messages in bytes",
			Buckets: prometheus.ExponentialBuckets(64, 2, 10),
		}),
		NGAPProcessingTime: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "fglos_ngap_processing_time_seconds",
			Help:    "Time taken to process NGAP messages",
			Buckets: prometheus.DefBuckets,
		}),

		// Load balancer metrics
		LoadBalancerDecisions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "fglos_load_balancer_decisions_total",
				Help: "Load balancer decisions by strategy and outcome",
			},
			[]string{"strategy", "outcome"},
		),
		SessionAffinityHits: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fglos_session_affinity_hits_total",
			Help: "Number of session affinity hits",
		}),
		SessionAffinityMisses: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fglos_session_affinity_misses_total",
			Help: "Number of session affinity misses",
		}),
	}
}

// RecordConnection records a new connection
func (r *Registry) RecordConnection() {
	r.TotalConnections.Inc()
	r.ActiveConnections.Inc()
}

// RecordConnectionClosed records a closed connection
func (r *Registry) RecordConnectionClosed() {
	r.ActiveConnections.Dec()
}

// RecordConnectionError records a connection error
func (r *Registry) RecordConnectionError() {
	r.ConnectionErrors.Inc()
}

// RecordUESession records a new UE session
func (r *Registry) RecordUESession() {
	r.TotalUESessions.Inc()
	r.ActiveUESessions.Inc()
}

// RecordUESessionClosed records a closed UE session
func (r *Registry) RecordUESessionClosed(duration float64) {
	r.ActiveUESessions.Dec()
	r.UESessionDuration.Observe(duration)
}

// RecordNGAPMessage records an NGAP message
func (r *Registry) RecordNGAPMessage(messageType, direction string, size int) {
	r.NGAPMessages.WithLabelValues(messageType, direction).Inc()
	r.NGAPMessageSize.Observe(float64(size))
}

// RecordNGAPProcessingTime records NGAP processing time
func (r *Registry) RecordNGAPProcessingTime(duration float64) {
	r.NGAPProcessingTime.Observe(duration)
}

// UpdateAMFInstances updates the number of AMF instances
func (r *Registry) UpdateAMFInstances(count int) {
	r.AMFInstances.Set(float64(count))
}

// UpdateAMFConnections updates connections for an AMF
func (r *Registry) UpdateAMFConnections(amfID, amfAddress string, count int) {
	r.AMFConnections.WithLabelValues(amfID, amfAddress).Set(float64(count))
}

// UpdateAMFHealth updates health status for an AMF
func (r *Registry) UpdateAMFHealth(amfID, amfAddress string, healthy bool) {
	status := 0.0
	if healthy {
		status = 1.0
	}
	r.AMFHealthStatus.WithLabelValues(amfID, amfAddress).Set(status)
}

// RecordLoadBalancerDecision records a load balancer decision
func (r *Registry) RecordLoadBalancerDecision(strategy, outcome string) {
	r.LoadBalancerDecisions.WithLabelValues(strategy, outcome).Inc()
}

// RecordSessionAffinityHit records a session affinity hit
func (r *Registry) RecordSessionAffinityHit() {
	r.SessionAffinityHits.Inc()
}

// RecordSessionAffinityMiss records a session affinity miss
func (r *Registry) RecordSessionAffinityMiss() {
	r.SessionAffinityMisses.Inc()
}
