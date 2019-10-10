package agent

import (
	"context"
	"fmt"
	"horus-core/log"
	"horus-core/model"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vma/glog"
)

// snmpQueue is a fixed size snmp job queue.
type snmpQueue struct {
	size     int
	requests chan SnmpRequest
	workers  chan struct{}
}

var (
	// MockMode activates mock snmp polling mode.
	MockMode bool

	// MaxRequests is the maximum number of parallel polling requests.
	MaxRequests int

	// GracefulQuitMode rejects new poll jobs, waiting for ongoing ones to finish before exiting.
	GracefulQuitMode bool

	// StatsUpdFreq is the frequency (in seconds) at wich various stats are retrieved and logged.
	StatsUpdFreq int

	// InterPollDelay is the delay to sleep before starting a new poll to smooth load
	InterPollDelay time.Duration

	// StopCtx is a context used to stop the agent gracefully.
	StopCtx context.Context

	// ongoingReqs is a map with all ongoing request ids as key (value not used).
	ongoingReqs = make(map[string]bool)
	ongoingMu   sync.RWMutex
	waiting     int64

	snmpq       snmpQueue
	pollResults chan *PollResult
)

// Init initializes the worker queue and starts the job dispatcher
// and the result handler.
func Init() error {
	if MaxRequests == 0 {
		return fmt.Errorf("agent: MaxRequests must be set")
	}

	glog.Infof("initializing %d workers", MaxRequests)
	snmpq = snmpQueue{
		size:     MaxRequests,
		requests: make(chan SnmpRequest, MaxRequests),
		workers:  make(chan struct{}, MaxRequests),
	}
	pollResults = make(chan *PollResult, MaxRequests)
	log.Debug2("starting dispatcher loop")
	go snmpq.dispatch(StopCtx)
	log.Debug2("starting results handler")
	go handleResults()
	log.Debug2("starting mem usage logger")
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
func AddSnmpRequest(req SnmpRequest) bool {
	select {
	case snmpq.workers <- struct{}{}:
		log.Debug2f("got worker, adding snmp req %s", req.UID)
		snmpq.requests <- req
		return true
	default:
		log.Debug2("snmp work queue full")
		return false
	}
}

// CurrentLoad returns the current load of the agent.
func CurrentLoad() float64 {
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
			req.Debug(1, "new request from queue")
			atomic.AddInt64(&waiting, 1)
			sincePrevPoll := time.Since(prevPoll)
			if sincePrevPoll < InterPollDelay {
				// sleep if needed, to smooth the load
				req.Debug(1, "waiting before poll")
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
func (s *snmpQueue) poll(ctx context.Context, req SnmpRequest) {
	defer func() {
		req.Debug(1, "done polling")
		<-s.workers
	}()
	req.Debug(1, "start polling")
	ongoingMu.Lock()
	ongoingReqs[req.UID] = true
	ongoingMu.Unlock()
	atomic.AddInt64(&waiting, -1)
	if err := req.Dial(ctx); err != nil {
		req.Errorf("unable to connect to snmp device: %v", err)
		res := MakePollResult(req) // needed for report
		res.pollErr = err
		pollResults <- &res
		return
	}
	pollResults <- req.Poll(ctx)
	req.Close()
	return
}

// updateStats updates and prints various agent stats.
func updateStats() {
	if StatsUpdFreq <= 0 {
		return
	}
	tick := time.NewTicker(time.Duration(StatsUpdFreq) * time.Second)
	defer tick.Stop()
	for range tick.C {
		var m runtime.MemStats
		var snmpSampleCount int
		runtime.ReadMemStats(&m)
		if snmpCollector != nil {
			snmpSampleCount = len(snmpCollector.Samples)
		}
		currSampleCount.Set(float64(snmpSampleCount))
		ongoingPollCount.Set(float64(len(ongoingReqs)))
		heapMem.Set(float64(m.Alloc))
		snmpScrapes.Set(float64(snmpCollector.scrapeCount))
		snmpScrapeDuration.Set(float64(snmpCollector.scrapeDuration) / float64(time.Second))
		log.Debugf("ongoing=%d prom_samples=%d scrape_count=%d scrape_dur=%v heap=%dMiB", len(ongoingReqs),
			snmpSampleCount, snmpCollector.scrapeCount, snmpCollector.scrapeDuration, m.Alloc/1024/1024)
	}
}
