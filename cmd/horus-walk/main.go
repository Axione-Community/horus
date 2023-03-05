package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"horus/agent"

	"github.com/vma/getopt"
	"github.com/vma/glog"
)

var (
	// Revision is the git revision, set at compilation
	Revision string

	// Build is the build time, set at compilation
	Build string

	// Branch is the git branch, set at compilation
	Branch string

	showVersion    = getopt.BoolLong("version", 'v', "Print version and build date")
	debug          = getopt.IntLong("debug", 'd', 0, "debug level")
	host           = getopt.StringLong("host", 'h', "", "snmp host")
	port           = getopt.IntLong("port", 'p', 161, "snmp port")
	community      = getopt.StringLong("community", 'c', "public", "snmp community")
	postProcessors = getopt.ListLong("post-processors", 'P', "post processors to apply to values", "pp1,pp2,...")
	rxBufSize      = getopt.IntLong("rx-buf-size", 'b', 0, "snmp RX buffer size (0: keep internal default value)")
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	getopt.SetParameters("OID")
	getopt.Parse()

	args := getopt.Args()

	if len(args) < 1 || *host == "" {
		getopt.PrintUsage(os.Stderr)
		os.Exit(1)
	}

	glog.WithConf(glog.Conf{Verbosity: *debug})

	if *showVersion {
		fmt.Printf("Revision:%s Branch:%s Build:%s\n", Revision, Branch, Build)
		return
	}

	if *rxBufSize > 0 {
		agent.RxBufSize = *rxBufSize
	}

	var buf strings.Builder
	for i, pp := range *postProcessors {
		if i > 0 {
			buf.WriteString(",")
		}
		fmt.Fprintf(&buf, "%q", pp)
	}
	jsReq := fmt.Sprintf(`{
"uid": "horus-walk",
"IndexedMeasures": [
 {
  "Metrics": [
   {
    "Oid": "%s",
    "Active": true,
    "PostProcessors": [%s]
   }
  ]
 }
],
"device": {
 "id": 1,
 "hostname": "%s",
 "ip_address": "%[3]s",
 "snmp_community": "%s",
 "category": "xxx",
 "vendor": "xxx",
 "model": "xxx"
 }
}`, args[0], buf.String(), *host, *community)

	var req agent.SnmpRequest
	if err := json.Unmarshal([]byte(jsReq), &req); err != nil {
		log.Fatalf("ERR: unmarshal snmp request from json: %v", err)
	}

	ctx := context.Background()
	if err := req.Dial(ctx); err != nil {
		log.Fatalf("Dial: %v", err)
	}
	defer req.Close()
	res := req.Poll(ctx)
	if res.PollErr != "" {
		log.Printf("Poll error: %v", res.PollErr)
		if !res.IsPartial {
			return
		} else {
			log.Print("*** PARTIAL RESULT ***")
		}
	}
	if len(res.Indexed) == 0 || len(res.Indexed[0].Results) == 0 {
		log.Print("no result...")
		return
	}
	for _, m := range res.Indexed[0].Results[0] {
		fmt.Printf("%s.%s = ", m.Oid, m.Index)
		switch val := m.Value.(type) {
		case []byte:
			fmt.Printf("%q\n", val)
		default:
			fmt.Printf("%v\n", val)
		}
	}
}
