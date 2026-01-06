package obs

import "github.com/prometheus/client_golang/prometheus"

var (
	InferRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "inference_requests_total",
			Help: "Total number of /infer requests",
		},
		[]string{"status", "provider", "model"},
	)

	QueueWait = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "inference_queue_wait_seconds",
			Help:    "Time a request spent waiting in the dispatcher queue",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider", "model"},
	)

	ExecTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "inference_exec_seconds",
			Help:    "Time spent executing provider call (worker execution time)",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider", "model"},
	)

	TotalTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "inference_total_seconds",
			Help:    "Total end-to-end time (queue_wait + exec)",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider", "model"},
	)
)

func MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(InferRequests, QueueWait, ExecTime, TotalTime)
}
