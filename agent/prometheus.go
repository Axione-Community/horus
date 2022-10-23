// Copyright 2019-2020 Kosc Telecom.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"hash/fnv"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/kosctelecom/horus/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PromSample is a prometheus metric.
type PromSample struct {
	// Name is the prometheus metric name in the form of <snmp measurement name>_<snmp metric name>.
	Name string

	// Desc is the metric description (usually the snmp oid).
	Desc string

	// Value is the metric value.
	Value float64

	// Tags is a map of labels common to a device
	Tags map[string]string

	// Labels is a map of labels common to a measure
	Labels map[string]string

	// MetricLabels is a map of labels specific to a metric (like index or oid)
	MetricLabels map[string]string

	// Stamp is the metric timestamp (the snmp poll start time).
	Stamp time.Time
}

// PromCollector represents a prometheus collector
type PromCollector struct {
	// Samples is the map of last samples kept in memory.
	Samples map[uint64]*PromSample

	// MaxResultAge is the max time a sample is kept in memory.
	MaxResultAge time.Duration

	// SweepFreq is the cleanup goroutine frequency to remove old metrics.
	SweepFreq time.Duration

	scrapeCount    int
	scrapeDuration time.Duration
	promSamples    chan *PromSample
	sync.Mutex
}

var (
	workersCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_worker_count",
		Help: "Number of max workers for this agent.",
	})
	currSampleCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_sample_count",
		Help: "Number of prom samples currently in memory of the agent.",
	})
	ongoingPollCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_snmp_poll_count",
		Help: "Number of currently ongoing snmp polls on this agent.",
	})
	heapMem = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_heap_memory_bytes",
		Help: "Heap memory usage for this agent.",
	})
	sysMem = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_system_memory_bytes",
		Help: "System memory for this agent.",
	})
	snmpScrapes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_snmp_scrape_total",
		Help: "Number of total prometheus snmp scrapes count.",
	})
	snmpScrapeDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_snmp_scrape_duration_seconds",
		Help: "snmp scrape duration.",
	})
	totalPollCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agent_snmp_poll_count_total",
		Help: "Total number of polled devices since startup.",
	})
)

var (
	snmpCollector     *SnmpCollector
	pingCollector     *PingCollector
	pollStatCollector *PromCollector
)

// InitCollectors initializes the snmp and ping collectors with retention time and cleanup frequency.
// We have three collectors:
// - /metrics for internal poll related metrics
// - /snmpmetrics for snmp polling results
// - /pingmetrics for ping results
func InitCollectors(maxResAge, sweepFreq int) {
	workersCount.Set(float64(MaxSNMPRequests))
	sysMem.Set(totalMem)
	prometheus.MustRegister(currSampleCount)
	prometheus.MustRegister(ongoingPollCount)
	prometheus.MustRegister(workersCount)
	prometheus.MustRegister(heapMem)
	prometheus.MustRegister(sysMem)
	prometheus.MustRegister(snmpScrapes)
	prometheus.MustRegister(snmpScrapeDuration)
	prometheus.MustRegister(totalPollCount)
	http.Handle("/metrics", promhttp.Handler())

	if sc := NewCollector(maxResAge, sweepFreq, "/snmpmetrics"); sc != nil {
		snmpCollector = &SnmpCollector{PromCollector: sc}
	}
	if pc := NewCollector(maxResAge, sweepFreq, "/pingmetrics"); pc != nil {
		pingCollector = &PingCollector{PromCollector: pc}
	}

	pollStatCollector = NewCollector(coalesceInt(maxResAge, 60), coalesceInt(sweepFreq, 30), "")
}

// NewCollector creates a new prometheus collector
func NewCollector(maxResAge, sweepFreq int, endpoint string) *PromCollector {
	if maxResAge <= 0 || sweepFreq <= 0 {
		return nil
	}

	collector := &PromCollector{
		Samples:      make(map[uint64]*PromSample),
		MaxResultAge: time.Duration(maxResAge) * time.Second,
		SweepFreq:    time.Duration(sweepFreq) * time.Second,
		promSamples:  make(chan *PromSample),
	}
	if endpoint == "" {
		// adds to default register
		prometheus.MustRegister(collector)
	} else {
		http.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			registry := prometheus.NewRegistry()
			registry.MustRegister(collector)
			h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
			h.ServeHTTP(w, r)
		})
	}
	go collector.processSamples()
	return collector
}

// processSamples processes each new sample popped from the channel
// and regularly cleans older results
func (c *PromCollector) processSamples() {
	sweepTick := time.NewTicker(c.SweepFreq).C
	for {
		select {
		case s := <-c.promSamples:
			id := s.computeKey()
			c.Lock()
			c.Samples[id] = s
			c.Unlock()
		case <-sweepTick:
			minStamp := time.Now().Add(-c.MaxResultAge)
			log.Debug2f("coll %p: cleaning samples older than %s", c, minStamp.Format(time.RFC3339))
			c.Lock()
			var outdatedCount int
			for id, res := range c.Samples {
				if res.Stamp.Before(minStamp) {
					delete(c.Samples, id)
					outdatedCount++
				}
			}
			if len(c.Samples) == 0 {
				// recreate map to solve go mem leak issue (https://github.com/golang/go/issues/20135)
				c.Samples = make(map[uint64]*PromSample)
			}
			c.Unlock()
			log.Debugf("coll %p: %d prom samples after cleanup, %d outdated samples deleted", c, len(c.Samples), outdatedCount)
		}
	}
}

// Describe implements prometheus.Collector
func (c *PromCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("dummy", "dummy", nil, nil)
}

// Collect implements prometheus.Collector
func (c *PromCollector) Collect(ch chan<- prometheus.Metric) {
	samples := make([]PromSample, 0, len(c.Samples))
	start := time.Now()
	c.Lock()
	for _, s := range c.Samples {
		dup := s.copyWithMergedLabels()
		samples = append(samples, dup)
	}
	c.Unlock()
	log.Debug2f("%d current snmp samples copied for scrape", len(samples))
	for i, sample := range samples {
		desc := prometheus.NewDesc(sample.Name, sample.Desc, nil, sample.Labels)
		metr, err := prometheus.NewConstMetric(desc, prometheus.UntypedValue, sample.Value)
		if err != nil {
			log.Errorf("collect: NewConstMetric: %v (sample: %+v)", err, sample)
			continue
		}
		ch <- prometheus.NewMetricWithTimestamp(sample.Stamp, metr)
		sample.Labels = nil
		samples[i] = PromSample{}
	}
	log.Debugf("scrape done in %dms (%d samples)", time.Since(start)/time.Millisecond, len(samples))
	c.scrapeCount++
	c.scrapeDuration = time.Since(start)
}

// computeKey calculates a consistent hash for the sample. It is used as the
// samples map key instead of the `sid` string for memory efficiency.
func (s *PromSample) computeKey() uint64 {
	keys := make([]string, 0, len(s.Tags)+len(s.Labels)+len(s.MetricLabels))
	for k := range s.Tags {
		keys = append(keys, k)
	}
	for k := range s.Labels {
		keys = append(keys, k)
	}
	for k := range s.MetricLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sid := s.Name
	for _, k := range keys {
		v, ok := s.Tags[k]
		if !ok {
			v, ok = s.Labels[k]
		}
		if !ok {
			v, ok = s.MetricLabels[k]
		}
		sid += k + v
	}
	h := fnv.New64a()
	h.Write([]byte(sid))
	return h.Sum64()
}

// coalesceInt returns its first non-zero argument, or zero.
func coalesceInt(nums ...int) int {
	for _, num := range nums {
		if num > 0 {
			return num
		}
	}
	return 0
}

func (s *PromSample) copyWithMergedLabels() PromSample {
	dup := PromSample{
		Name:  s.Name,
		Desc:  s.Desc,
		Value: s.Value,
		Stamp: s.Stamp,
	}
	l := map[string]string{}
	for k, v := range s.Tags {
		l[k] = v
	}
	for k, v := range s.Labels {
		l[k] = v
	}
	for k, v := range s.MetricLabels {
		l[k] = v
	}
	dup.Labels = l
	return dup
}
