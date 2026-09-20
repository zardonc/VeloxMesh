package heuristic

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	duration       prometheus.Observer
	tasks          prometheus.Counter
	errors         prometheus.Counter
	classification *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	duration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "scheduler_batch_score_duration_ms",
		Help: "BatchScoreTasks duration in milliseconds.",
	})
	tasks := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_tasks_scored_total",
		Help: "Tasks scored by the scheduler.",
	})
	errors := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_scoring_errors_total",
		Help: "Scheduler scoring errors.",
	})
	classification := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "scheduler_classification_source",
		Help: "Scheduler classification source counts.",
	}, []string{"source"})
	if reg != nil {
		for _, collector := range []prometheus.Collector{duration, tasks, errors, classification} {
			if err := reg.Register(collector); err != nil {
				if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
					panic(err)
				}
			}
		}
	}
	return &Metrics{duration: duration, tasks: tasks, errors: errors, classification: classification}
}

func (m *Metrics) Observe(durationMs float64, source string, taskCount int) {
	if m == nil {
		return
	}
	m.duration.Observe(durationMs)
	m.tasks.Add(float64(taskCount))
	m.classification.WithLabelValues(source).Inc()
}

func (m *Metrics) Error() {
	if m != nil {
		m.errors.Inc()
	}
}
