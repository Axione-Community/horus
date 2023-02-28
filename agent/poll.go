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
	"context"
	"sync"
	"time"

	"horus/log"
	"horus/model"

	"github.com/vma/glog"
)

// snmpQueue is a fixed size snmp job queue.
type snmpQueue struct {
	size     int
	used     int64
	requests chan *SnmpRequest
	workers  chan struct{}
}

var (
	// MockMode activates mock snmp polling mode.
	MockMode bool

	// MaxSNMPRequests is the maximum number of parallel polling requests.
	MaxSNMPRequests int

	// GracefulQuitMode rejects new poll jobs, waiting for ongoing ones to finish before exiting.
	GracefulQuitMode bool

	// StatsUpdFreq is the frequency (in seconds) at wich various stats are retrieved and logged.
	StatsUpdFreq int

	// InterPollDelay is the delay to sleep before starting a new poll to smooth load
	InterPollDelay time.Duration

	// StopCtx is a context used to stop the agent gracefully.
	StopCtx context.Context

	// ongoingReqs is a map with all active poll requests with uid as key and device id as value
	ongoingReqs = make(map[string]int)
	ongoingMu   sync.RWMutex

	// waiting is the count of snmp requests waiting to be sent
	waiting int64

	// snmpq is the global snmp jobs queue.
	snmpq snmpQueue

	// pollResults is the channel where the poll results are pushed.
	pollResults chan PollResult

	// totalPolls is total number of completed poll since startup
	totalPolls int64
)

// Init initializes the worker queue and starts the job dispatcher
// and the result handler.
func Init() error {
	if MaxSNMPRequests > 0 {
		glog.Infof("initializing %d snmp workers", MaxSNMPRequests)
		snmpq = snmpQueue{
			size:     MaxSNMPRequests,
			requests: make(chan *SnmpRequest, MaxSNMPRequests),
			workers:  make(chan struct{}, MaxSNMPRequests),
		}
		pollResults = make(chan PollResult, 2*MaxSNMPRequests)
		log.Debug2("starting dispatcher loop")
		go snmpq.dispatch(StopCtx)
		log.Debug2("starting results handler")
		go handlePollResults()
	} else {
		log.Info("snmp polling disabled")
	}

	log.Debug2("starting mem usage logger")
	go updateTotalMem()
	go updateUsedMem()
	go updateStats()

	pingQ = pingQueue{
		requests: make(chan model.PingRequest, MaxPingProcs),
		workers:  make(chan struct{}, MaxPingProcs),
	}
	go pingQ.dispatch(StopCtx)
	return nil
}

// AddSnmpRequest adds a new snmp request to the queue.
// Returns true if it was added i.e. a worker slot was acquired.
func AddSnmpRequest(req *SnmpRequest) bool {
	select {
	case snmpq.workers <- struct{}{}:
		log.Debug2f("got worker, adding snmp req %s, usage:%d/%d workers:%d", req.UID, snmpq.used, snmpq.size, len(snmpq.workers))
		snmpq.requests <- req
		snmpq.used++
		return true
	default:
		log.Debug2f("snmp work queue full, usage:%d/%d workers:%d reqs:%d", snmpq.used, snmpq.size, len(snmpq.workers), len(snmpq.requests))
		return false
	}
}

// CurrentSNMPLoad returns the current snmp load of the agent. It is calculated as
// the current number of all snmp requests in queue over the queue size.
func CurrentSNMPLoad() float64 {
	if snmpq.size == 0 {
		return 0
	}
	return float64(len(snmpq.requests)+int(waiting)+len(ongoingReqs)) / float64(snmpq.size)
}

// dispatch treats the poll requests as they come in.
func (s *snmpQueue) dispatch(ctx context.Context) {
	prevPoll := time.Now()
	for {
		select {
		case <-ctx.Done():
			log.Debug("cancelled, terminating snmp dispatch loop")
			return
		case req := <-s.requests:
			req.Debug(2, "new request from queue")
			waiting++
			sincePrevPoll := time.Since(prevPoll)
			if sincePrevPoll < InterPollDelay {
				// sleep if needed, to smooth the load
				req.Debug(2, "waiting before poll")
				time.Sleep(InterPollDelay - sincePrevPoll)
			}
			if MockMode {
				go s.mockPoll(ctx, req)
			} else {
				go s.poll(ctx, req)
			}
			prevPoll = time.Now()
		}
	}
}

// poll polls the snmp device. At the end, pushes the result
// to the results queue and releases the worker slot.
func (s *snmpQueue) poll(ctx context.Context, req *SnmpRequest) {
	req.Debugf(1, "start polling, ongoing: %d, usage: %d/%d", len(ongoingReqs), s.used, s.size)
	ongoingMu.Lock()
	ongoingReqs[req.UID] = req.Device.ID
	ongoingMu.Unlock()
	waiting--
	if err := req.Dial(ctx); err != nil {
		req.Errorf("unable to connect to snmp device: %v", err)
		res := req.MakePollResult() // needed for report
		res.pollErr = err
		pollResults <- res
	} else {
		pollResults <- req.Poll(ctx)
		req.Close()
	}
	ongoingMu.Lock()
	delete(ongoingReqs, req.UID)
	ongoingMu.Unlock()
	<-s.workers
	s.used--
	totalPolls++
	req.Debugf(1, "done polling, ongoing: %d, usage: %d/%d", len(ongoingReqs), s.used, s.size)
}

// updateStats updates and prints various agent stats.
func updateStats() {
	if StatsUpdFreq <= 0 {
		return
	}
	tick := time.NewTicker(time.Duration(StatsUpdFreq) * time.Second)
	defer tick.Stop()
	for range tick.C {
		var snmpSampleCount int
		currSampleCount.Set(float64(snmpSampleCount))
		ongoingPollCount.Set(float64(len(ongoingReqs)))
		totalPollCount.Set(float64(totalPolls))
		heapMem.Set(usedMem)
		log.Debugf("ongoing=%d total_polled=%d heap=%.0fMiB", len(ongoingReqs), totalPolls, usedMem/1024/1024)
	}
}
