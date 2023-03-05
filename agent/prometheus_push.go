package agent

import (
	"bytes"
	"errors"
	"fmt"
	"horus/log"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

type PromClient struct {
	Endpoints []string
	Timeout   int

	totalPushErrors    int
	totalPushCount     int
	lastPushDurationMs int
}

var promCli *PromClient

func NewPromClient(endpoints []string, timeout int) error {
	if len(endpoints) == 0 || timeout <= 0 {
		return errors.New("prometheus endpoint(s) and timeout must be defined")
	}
	for i, ep := range endpoints {
		if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
			endpoints[i] = "http://" + ep
		}
	}
	promCli = &PromClient{
		Endpoints: endpoints,
		Timeout:   timeout,
	}
	return nil
}

func (c *PromClient) Close() {}

func (c *PromClient) Push(pollRes PollResult) {
	if c == nil {
		return
	}

	ts := append(pollRes.SNMPMetricsToPromTS(), pollRes.pollStatsToPromTS()...)
	if len(ts) == 0 {
		log.Infof("prom cli[%s]: skip pushing empty poll result for device #%d", pollRes.RequestID, pollRes.DeviceID)
		return
	}
	pb, err := proto.Marshal(&prompb.WriteRequest{Timeseries: ts})
	if err != nil {
		log.Errorf("prom cli[%s]: proto marshal: %v", pollRes.RequestID, err)
		return
	}
	data := snappy.Encode(nil, pb)

	var wg sync.WaitGroup
	for _, ep := range c.Endpoints {
		wg.Add(1)
		go func(ep string) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(StopCtx, http.MethodPost, ep, bytes.NewBuffer(data))
			if err != nil {
				log.Errorf("prom cli[%s]: http req for %s: %v", pollRes.RequestID, ep, err)
				return
			}
			req.Header.Add("X-Prometheus-Remote-Write-Version", "0.1.0")
			req.Header.Add("Content-Encoding", "snappy")
			req.Header.Set("Content-Type", "application/x-protobuf")
			client := &http.Client{Timeout: time.Duration(c.Timeout) * time.Second}
			log.Debugf("prom cli[%s]: posting %d metrics to %s", pollRes.RequestID, len(ts), ep)
			start := time.Now()
			resp, err := client.Do(req)
			if err != nil {
				log.Errorf("prom cli[%s]: remote write to %s: %v", pollRes.RequestID, ep, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				body, _ := io.ReadAll(resp.Body)
				log.Errorf("prom cli[%s]: remote write to %s: returned %d: %s", pollRes.RequestID, ep, resp.StatusCode, body)
				c.totalPushErrors++
			} else {
				c.totalPushCount++
				c.lastPushDurationMs = int(time.Since(start) / time.Millisecond)
				log.Infof("prom cli[%s]: remote write to %s succeeded in %dms: %s", pollRes.RequestID, ep, c.lastPushDurationMs, resp.Status)
			}
			snmpPushCount.Set(float64(c.totalPushCount))
			snmpPushDuration.Set(float64(c.lastPushDurationMs) / 1000)
		}(ep)
	}
	wg.Wait()
}

func (p *PollResult) SNMPMetricsToPromTS() []prompb.TimeSeries {
	var promTS []prompb.TimeSeries
	stamp := p.stamp.UnixNano() / int64(time.Millisecond)

	for _, scalar := range p.Scalar {
		if !scalar.ToProm {
			continue
		}
		for _, res := range scalar.Results {
			labels := make([]prompb.Label, 0, len(p.Tags)+3)
			sample := prompb.Sample{Timestamp: stamp}
			labels = append(labels, prompb.Label{Name: "__name__", Value: scalar.Name + "_" + res.Name})
			labels = append(labels, prompb.Label{Name: "oid", Value: res.Oid})
			for k, v := range p.Tags {
				labels = append(labels, prompb.Label{Name: k, Value: v})
			}
			if res.AsLabel {
				labels = append(labels, prompb.Label{Name: res.Name, Value: fmt.Sprint(res.Value)})
				sample.Value = 1
			} else {
				switch v := res.Value.(type) {
				case float64:
					sample.Value = v
				case int64:
					sample.Value = float64(v)
				case int:
					sample.Value = float64(v)
				case uint:
					sample.Value = float64(v)
				case bool:
					if v {
						sample.Value = 1
					}
				default:
					continue
				}
			}
			promTS = append(promTS, prompb.TimeSeries{Labels: labels, Samples: []prompb.Sample{sample}})
		}
	}

	for _, indexed := range p.Indexed {
		if !indexed.ToProm {
			continue
		}
		for _, indexedRes := range indexed.Results {
			// loops over index: indexedRes contains all metrics of one interface
			var labels []prompb.Label
			for k, v := range p.Tags {
				labels = append(labels, prompb.Label{Name: k, Value: v})
			}
			for _, res := range indexedRes {
				if res.AsLabel {
					labels = append(labels, prompb.Label{Name: res.Name, Value: fmt.Sprint(res.Value)})
				}
			}
			if len(labels) == len(indexedRes) {
				// label-only measure
				if !indexed.LabelsOnly {
					log.Debugf(">> skipping non label-only measure %s with only labels", indexed.Name)
					continue
				}
				log.Debug2f("indexed measure %s is labels-only", indexed.Name)
				labels = append(labels, prompb.Label{Name: "__name__", Value: indexed.Name})
				samples := []prompb.Sample{{Timestamp: stamp, Value: 1}}
				promTS = append(promTS, prompb.TimeSeries{Labels: labels, Samples: samples})
				continue
			}

			for _, res := range indexedRes {
				// loops over oid i.e. each metric of a given interface
				if res.AsLabel {
					continue
				}
				var value float64
				switch v := res.Value.(type) {
				case float64:
					value = v
				case int64:
					value = float64(v)
				case int:
					value = float64(v)
				case uint:
					value = float64(v)
				case bool:
					if v {
						value = 1
					}
				default:
					continue
				}
				iLabels := make([]prompb.Label, len(labels), len(labels)+3)
				copy(iLabels, labels)
				iLabels = append(iLabels, prompb.Label{Name: "__name__", Value: fmt.Sprintf("%s_%s", indexed.Name, res.Name)},
					prompb.Label{Name: "oid", Value: res.Oid}, prompb.Label{Name: "index", Value: res.Index})
				samples := []prompb.Sample{{Timestamp: stamp, Value: value}}
				promTS = append(promTS, prompb.TimeSeries{Labels: iLabels, Samples: samples})
			}
		}
	}
	return promTS
}

func (p *PollResult) pollStatsToPromTS() []prompb.TimeSeries {
	pollStats := make(map[string]float64)
	pollStats["snmp_poll_timeout_count"] = 0
	if ErrIsTimeout(p.pollErr) {
		pollStats["snmp_poll_timeout_count"] = 1
	}
	pollStats["snmp_poll_refused_count"] = 0
	if ErrIsRefused(p.pollErr) {
		pollStats["snmp_poll_refused_count"] = 1
	}
	pollStats["snmp_poll_duration_seconds"] = float64(p.Duration / 1000)
	pollStats["snmp_poll_metric_count"] = float64(p.metricCount)

	promTS := make([]prompb.TimeSeries, 0, len(pollStats))
	stamp := p.stamp.UnixNano() / int64(time.Millisecond)

	for k, v := range pollStats {
		labels := make([]prompb.Label, 0, len(p.Tags)+1)
		for tn, tv := range p.Tags {
			labels = append(labels, prompb.Label{Name: tn, Value: tv})
		}
		labels = append(labels, prompb.Label{Name: "__name__", Value: k})
		samples := []prompb.Sample{{Timestamp: stamp, Value: v}}
		promTS = append(promTS, prompb.TimeSeries{Labels: labels, Samples: samples})
	}
	return promTS
}
