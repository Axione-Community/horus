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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"horus/log"
	"horus/model"
)

// pingQueue is a fixed size ping job queue.
type pingQueue struct {
	requests chan model.PingRequest
	workers  chan struct{}
}

// PingMeasure is the result of a ping request.
type PingMeasure struct {
	// HostID is the host db id
	HostID int

	// Hostname is the pinged host name
	Hostname string

	// IPAddr is the ip address of the pinged host
	IPAddr string

	// Tags is the tag map associated with the result
	Tags map[string]string `json:"tags,omitempty"`

	// Min is the minimal RTT in seconds
	Min float64

	// Max is the maximal RTT in seconds
	Max float64

	// Avg is the average RTT in seconds
	Avg float64

	// Loss is the packet loss percentage
	Loss float64

	// Stamp is the ping request datetime
	Stamp time.Time
}

var (
	// MaxPingProcs is the simultaneous fping process limit for this agent
	MaxPingProcs int

	// PingPacketCount is number of ping requests to send to target (-C param of fping)
	PingPacketCount int

	// pingQ is the ping jobs queue
	pingQ pingQueue
)

// AddPingRequest adds a new ping request to the queue.
// Returns true if it was successfuly added (i.e. the queue is not full)
func AddPingRequest(req model.PingRequest) bool {
	select {
	case pingQ.workers <- struct{}{}:
		log.Debug2f("adding ping req %s", req.UID)
		pingQ.requests <- req
		return true
	default:
		log.Debug2("ping work queue full")
		return false
	}
}

// dispatch treats the ping requests as they come in.
func (p *pingQueue) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Debug("cancelled, terminating ping dispatch loop")
			return
		case req := <-p.requests:
			log.Debugf("%s - new ping request from queue", req.UID)
			go p.ping(ctx, req)
		}
	}
}

// ping launches the fping process synchronously.
//
// example of command:
// fping -q -p 50 -i 10 -t 100 -C 15 10.2.0.26 10.2.1.49 10.2.4.81 10.2.3.25...
func (p *pingQueue) ping(ctx context.Context, req model.PingRequest) {
	defer func() {
		<-p.workers
	}()
	log.Debugf("%s - start pinging %d hosts", req.UID, len(req.Hosts))
	req.Stamp = time.Now()
	args := []string{"-q", "-p", "50", "-i", "10", "-t", "100", "-C", strconv.Itoa(PingPacketCount)}
	for _, host := range req.Hosts {
		if host.IPAddr == "" {
			log.Infof("ping: skipping host %s without ipaddr", host.Name)
			continue
		}
		args = append(args, host.IPAddr)
	}
	log.Debug2f("%s - launching fping %s...", req.UID, args)
	cmd := exec.Command("fping", args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		if !strings.HasPrefix(err.Error(), "exit status 1") {
			// fping returns 1 if some hosts are unreachable
			log.Warningf("%s - fping failed: %v", req.UID, err)
		}
	}
	log.Debugf("%s - ping completed", req.UID)
	measures := processOutput(req, out.String())
	log.Debug2f("%s - ping metrics processed", req.UID)
	for _, m := range measures {
		pingCollector.Push(m)
	}
	log.Debugf("%s - ping measures pushed to collector", req.UID)
}

// processOutput parses fping output and returns the ping measure
// for each host.
//
// example of output:
// ICMP Time Exceeded from 172.2.5.70 for ICMP Echo sent to 10.2.5.104
// 10.2.7.26 : 17.82 17.73 17.67 17.78 17.58 17.61 17.69 17.64 17.76 17.59 17.62 17.60 17.67 17.58 17.69
// 10.2.1.49 : 8.14 8.12 8.10 7.94 7.85 8.01 8.02 8.07 8.00 7.92 7.98 7.93 8.04 8.00 8.10
// 10.2.4.81 : 12.95 12.82 12.87 12.86 12.72 17.87 12.89 12.82 12.82 17.25 16.05 12.77 12.77 12.71 12.86
// 10.2.3.25 : 16.73 11.55 11.32 12.79 12.68 11.31 11.39 11.30 16.60 16.58 - - - - 11.35
// ...
func processOutput(req model.PingRequest, output string) []PingMeasure {
	log.Debug2f(">> %s - processing output\n%s\n<<", req.UID, output)
	var metrics = make(map[string][]float64)
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if strings.HasPrefix(line, "ICMP Time Exceeded from") {
			tokens := strings.Fields(line)
			ipAddr := tokens[len(tokens)-1]
			log.Debug2f("%s: min=- max=- avg=- loss=100%%", ipAddr)
			metrics[ipAddr] = []float64{0, 0}
		} else {
			tokens := strings.Fields(line)
			if len(tokens) < 3 {
				log.Errorf("processOutput: invalid output line `%s`", line)
				continue
			}
			ipAddr := tokens[0]
			for _, tok := range tokens[2:] {
				if tok == "-" {
					metrics[ipAddr] = append(metrics[ipAddr], 0)
				} else {
					rtt, _ := strconv.ParseFloat(tok, 64)
					metrics[ipAddr] = append(metrics[ipAddr], rtt)
				}
			}
		}
	}
	res := make([]PingMeasure, 0, len(metrics))
	for ipAddr, samples := range metrics {
		min, max, avg, loss := computeStats(samples)
		log.Debug2f("%s: min=%.2f max=%.2f avg=%.2f loss=%.2f%%", ipAddr, min, max, avg, 100*loss)
		meas := PingMeasure{
			IPAddr: ipAddr,
			Min:    min / 1000,
			Max:    max / 1000,
			Avg:    avg / 1000,
			Loss:   loss,
			Stamp:  req.Stamp,
		}
		for _, host := range req.Hosts {
			if host.IPAddr == ipAddr {
				meas.Tags = map[string]string{
					"id":         strconv.Itoa(host.ID),
					"host":       host.Name,
					"ip_address": meas.IPAddr,
					"category":   host.Category,
					"vendor":     host.Vendor,
					"model":      host.Model,
				}
				if host.Tags != "" {
					var hostTags map[string]interface{}
					if err := json.Unmarshal([]byte(host.Tags), &hostTags); err != nil {
						log.Errorf("host tag json unmarshal: %v", err)
					} else {
						for k, v := range hostTags {
							meas.Tags[k] = fmt.Sprint(v)
						}
					}
				}
				break
			}
		}
		res = append(res, meas)
	}
	return res
}

// computeStats calculates the min, max, avg times (in ms) and loss proportion
// from a line of fping measures.
func computeStats(samples []float64) (min, max, avg, loss float64) {
	var firstPositiveIdx int = -1
	var sum float64

	sort.Float64s(samples)
	for i, rtt := range samples {
		sum += rtt
		if rtt > 0 && min == 0 {
			min = rtt
			firstPositiveIdx = i
		}
	}
	if firstPositiveIdx == -1 {
		loss = 1.0
		return
	}
	max = samples[len(samples)-1]
	loss = float64(firstPositiveIdx) / float64(len(samples))
	avg = sum / float64(len(samples)-firstPositiveIdx)
	return
}
