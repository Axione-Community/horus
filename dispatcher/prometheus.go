package dispatcher

import "github.com/prometheus/client_golang/prometheus"

var (
	Revision string
	Branch   string
	Build    string

	totalSNMPJobs = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_snmp_poll_count",
		Help: "Number of snmp jobs on current cycle",
	})
	acceptedSNMPJobs = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_snmp_poll_accepted_count",
		Help: "Number of snmp jobs accepted by agents on current cycle",
	})
	discardedSNMPJobs = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_snmp_poll_discarded_count",
		Help: "Number of snmp jobs discarded on current cycle",
	})
	totalPingHosts = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_ping_host_count",
		Help: "Number of ping hosts on current cycle",
	})
	acceptedPingHosts = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_ping_host_accepted_count",
		Help: "Number of ping hosts accepted by agents on current cycle",
	})
	discardedPingHosts = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_ping_host_discarded_count",
		Help: "Number of ping hosts discarded on current cycle",
	})
)

func RegisterPromMetrics() {
	version := prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "dispatcher_version",
			Help:        "Dispatcher current version",
			ConstLabels: prometheus.Labels{"revision": Revision, "branch": Branch, "build": Build},
		},
		func() float64 { return 1 })
	masterStatus := prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "dispatcher_master",
			Help: "Master status of dispatcher",
		},
		func() float64 {
			if IsMaster {
				return 1
			}
			return 0
		})
	activeAgentCount := prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "dispatcher_active_agent_count",
			Help: "Current number of working agents",
		},
		func() float64 { return float64(ActiveAgentCount()) })
	prometheus.MustRegister(masterStatus)
	prometheus.MustRegister(version)
	prometheus.MustRegister(activeAgentCount)
	prometheus.MustRegister(acceptedSNMPJobs)
	prometheus.MustRegister(discardedSNMPJobs)
	prometheus.MustRegister(totalSNMPJobs)
	prometheus.MustRegister(totalPingHosts)
	prometheus.MustRegister(acceptedPingHosts)
	prometheus.MustRegister(discardedPingHosts)
}
