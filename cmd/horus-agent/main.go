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

package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"horus/agent"
	"horus/log"
	"horus/model"

	"github.com/vma/getopt"
	"github.com/vma/glog"
	"github.com/vma/httplogger"
)

var (
	// Revision is the git revision, set at compilation
	Revision string

	// Build is the build time, set at compilation
	Build string

	// Branch is the git branch, set at compilation
	Branch string

	dbgLevel       = getopt.IntLong("debug", 'd', 0, "debug level")
	port           = getopt.Int16Long("port", 'p', 8080, "API webserver listen port", "port")
	showVersion    = getopt.BoolLong("version", 'v', "Print version and build date")
	snmpJobCount   = getopt.IntLong("snmp-jobs", 'j', 1, "Number of simultaneous snmp jobs", "count")
	maxMemLoad     = getopt.IntLong("max-mem-load", 'm', 90, "Max memory usage allowed before rejecting new jobs", "percent")
	mock           = getopt.BoolLong("mock", 0, "Run the agent in mock mode (no actual snmp query)")
	statUpdFreq    = getopt.IntLong("stat-frequency", 's', 0, "Agent stats update frequency (disabled if 0)", "sec")
	interPollDelay = getopt.IntLong("inter-poll-delay", 't', 5, "time to wait between successive poll start", "msec")
	logDir         = getopt.StringLong("log", 0, "", "directory for log files, disabled if empty (all log goes to stderr)", "dir")

	// prometheus conf
	maxResAge = getopt.IntLong("prom-max-age", 0, 0, "Maximum time to keep prometheus samples in mem, disabled if 0", "sec")
	sweepFreq = getopt.IntLong("prom-sweep-frequency", 0, 120, "Prometheus old samples cleaning frequency", "sec")

	// influx conf
	influxHost    = getopt.StringLong("influx-host", 0, "", "influx server address (push to influx disabled if empty)")
	influxUser    = getopt.StringLong("influx-user", 0, "", "influx user")
	influxPasswd  = getopt.StringLong("influx-password", 0, "", "influx user password")
	influxDB      = getopt.StringLong("influx-db", 0, "", "influx database")
	influxRP      = getopt.StringLong("influx-rp", 0, "autogen", "influx retention policy for pushed data")
	influxTimeout = getopt.IntLong("influx-timeout", 0, 5, "influx write timeout in second")
	influxRetries = getopt.IntLong("influx-retries", 0, 2, "influx write retries in case of error")

	// kafka conf
	kafkaHosts     = getopt.ListLong("kafka-hosts", 'k', "kafka broker hosts list (push to kafka disabled if empty)", "host1,host2,...")
	kafkaTopic     = getopt.StringLong("kafka-topic", 0, "", "kafka snmp results topic")
	kafkaPartition = getopt.IntLong("kafka-partition", 0, 0, "kafka write partition")

	// NATS conf
	natsHosts          = getopt.ListLong("nats-hosts", 'n', "NATS hosts list (push to NATS disabled if empty)", "host1,host2,...")
	natsSubject        = getopt.StringLong("nats-subject", 0, "horus.metrics", "NATS subject for snmp results")
	natsName           = getopt.StringLong("nats-name", 0, "", "NATS connection name")
	natsReconnectDelay = getopt.IntLong("nats-reconnect-delay", 0, 10, "NATS delay before reconnecting", "seconds")

	// fping conf
	pingPacketCount = getopt.IntLong("fping-packet-count", 0, 15, "number of ping requests sent to each host")
	maxPingProcs    = getopt.IntLong("fping-max-procs", 0, 5, "max number of simultaneous fping processes")
)

func main() {
	getopt.FlagLong(&agent.RxBufSize, "snmp-buf-size", 'b', "UDP receive buffer size for snmp replies (in bytes)")
	getopt.SetParameters("")
	getopt.Parse()

	glog.WithConf(glog.Conf{Verbosity: *dbgLevel, LogDir: *logDir, PrintLocation: *dbgLevel > 0})

	agent.Revision, agent.Branch, agent.Build = Revision, Branch, Build

	if *showVersion {
		fmt.Printf("Revision:%s Branch:%s Build:%s\n", Revision, Branch, Build)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for {
			select {
			case <-c:
				glog.Warning("interrupt received, canceling all requests")
				cancel()
				time.Sleep(500 * time.Millisecond) // wait for all job reports to be sent
				os.Exit(0)
			case <-ctx.Done():
				return
			}
		}
	}()

	if *maxPingProcs > 0 {
		if _, err := exec.LookPath("fping"); err != nil {
			glog.Exit("fping binary not found in PATH. Please install fping and/or set $PATH accordingly.")
		}
		if *pingPacketCount == 0 {
			glog.Exit("fping-packet-count cannot be zero")
		}
	}

	if *maxResAge == 0 && *influxHost == "" && len(*kafkaHosts) == 0 && len(*natsHosts) == 0 {
		getopt.PrintUsage(os.Stderr)
		glog.Exit("either prom-max-age, influx-host,kafka-host, or nats-host must be defined")
	}

	agent.MockMode = *mock
	agent.MaxSNMPRequests = *snmpJobCount
	agent.MaxAllowedLoad = float64(*maxMemLoad) / 100
	agent.StatsUpdFreq = *statUpdFreq
	agent.InterPollDelay = time.Duration(*interPollDelay) * time.Millisecond
	agent.PingPacketCount = *pingPacketCount
	agent.MaxPingProcs = *maxPingProcs
	agent.StopCtx = ctx

	if err := agent.Init(); err != nil {
		glog.Exitf("init agent: %v", err)
	}

	agent.InitCollectors(*maxResAge, *sweepFreq)

	if *influxHost != "" {
		if err := agent.NewInfluxClient(*influxHost, *influxUser, *influxPasswd,
			*influxDB, *influxRP, *influxTimeout, *influxRetries); err != nil {
			glog.Exitf("init influx client: %v", err)
		}
	}

	if len(*kafkaHosts) != 0 {
		if err := agent.NewKafkaClient(*kafkaHosts, *kafkaTopic, *kafkaPartition); err != nil {
			glog.Exitf("init kafka client: %v", err)
		}
	}

	if len(*natsHosts) != 0 {
		if err := agent.NewNatsClient(*natsHosts, *natsSubject, *natsName, *natsReconnectDelay); err != nil {
			glog.Exitf("init NATS client: %v", err)
		}
	}

	http.HandleFunc(model.SnmpJobURI, agent.HandleSnmpRequest)
	http.HandleFunc(model.CheckURI, agent.HandleCheck)
	http.HandleFunc(model.OngoingURI, agent.HandleOngoing)
	http.HandleFunc(model.PingJobURI, agent.HandlePingRequest)
	http.HandleFunc("/-/stop", handleStop)
	http.HandleFunc("/-/debug", handleDebugLevel)
	http.HandleFunc("/-/freeosmem", handleFreeOSMem)
	logger := httplogger.CommonLogger(log.Writer{})
	log.Infof("starting web server on port %d", *port)
	glog.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), logger(http.DefaultServeMux)))
}

// handleStop handles agent graceful stop. Waits for all polling
// jobs to finish and for a last prom scrape before exiting.
func handleStop(w http.ResponseWriter, r *http.Request) {
	log.Infof("** graceful stop request from %s", r.RemoteAddr)
	initialScrapeCount := agent.SnmpScrapeCount()
	agent.GracefulQuitMode = true
	if agent.CurrentSNMPLoad() == 0 {
		goto end
	}
	for agent.CurrentSNMPLoad() > 0 {
		time.Sleep(500 * time.Millisecond)
	}
	if *maxResAge > 0 {
		// wait for a final prom scrape with a 5mn timeout
		remainingLoops := 600
		for agent.SnmpScrapeCount() == initialScrapeCount && remainingLoops > 0 {
			time.Sleep(500 * time.Millisecond)
			remainingLoops--
		}
	}
end:
	_, cancel := context.WithCancel(context.Background())
	cancel()
	time.Sleep(500 * time.Millisecond)
	w.WriteHeader(http.StatusNoContent)
	os.Exit(0)
}

// handleDebugLevel sets the application debug level dynamically.
func handleDebugLevel(w http.ResponseWriter, r *http.Request) {
	level := r.FormValue("level")
	if level == "" {
		fmt.Fprintf(w, "level=%d", glog.GetLevel())
		return
	}
	dbgLevel, err := strconv.Atoi(level)
	if err != nil || dbgLevel < 0 || dbgLevel > 3 {
		log.Errorf("invalid level %s", level)
		http.Error(w, "invalid debug level "+level, 400)
		return
	}
	glog.SetLevel(int32(dbgLevel))
	w.WriteHeader(http.StatusOK)
}

func handleFreeOSMem(w http.ResponseWriter, r *http.Request) {
	log.Warning("forcing GC & OS memory release...")
	runtime.GC()
	debug.FreeOSMemory()
}
