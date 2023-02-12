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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"horus/log"
	"horus/model"

	"github.com/vma/glog"
)

// MaxAllowedLoad is memory load treshold to reject new snmp requests.
var MaxAllowedLoad float64

// HandleSnmpRequest handles snmp polling job requests.
func HandleSnmpRequest(w http.ResponseWriter, r *http.Request) {
	log.Debugf("new poll request from %s", r.RemoteAddr)
	if MaxSNMPRequests == 0 {
		log.Debug("snmp polling not enabled, rejecting request")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
		return
	}

	currMemLoad := CurrentMemLoad()
	if currMemLoad >= MaxAllowedLoad {
		log.Warningf("current mem load high (%.2f%%), rejecting new requests", 100*currMemLoad)
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
		return
	}

	if r.Method != http.MethodPost {
		log.Warningf("rejecting request from %s with %s method", r.RemoteAddr, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
		return
	}

	if GracefulQuitMode {
		log.Debug("in graceful quit mode, rejecting all new requests")
		w.WriteHeader(http.StatusLocked)
		fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	var req SnmpRequest
	if err := decoder.Decode(&req); err != nil {
		log.Debugf("invalid json request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
		return
	}

	if AddSnmpRequest(&req) {
		log.Debugf("%s - request successfully queued", req.UID)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
		return
	}

	glog.Warningf("no more workers, rejecting request %s", req.UID)
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
	return
}

// HandleCheck responds to keep-alive checks.
// Returns current worker count in body.
func HandleCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%.4f", CurrentSNMPLoad())
}

// HandleOngoing returns the list of ongoing snmp requests,
// their count, and the total workers count.
func HandleOngoing(w http.ResponseWriter, r *http.Request) {
	var ongoing model.OngoingPolls

	ongoingMu.RLock()
	for uid, devID := range ongoingReqs {
		ongoing.Requests = append(ongoing.Requests, uid)
		ongoing.Devices = append(ongoing.Devices, devID)
	}
	ongoingMu.RUnlock()
	ongoing.Load = CurrentSNMPLoad()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ongoing)
}

// HandlePingRequest handles ping job requests.
// Returns a status 202 when the job is accepted, a 4XX error status otherwise.
func HandlePingRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Debugf("rejecting request from %s with %s method", r.RemoteAddr, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if GracefulQuitMode {
		log.Debug("in graceful quit mode, rejecting all new requests")
		w.WriteHeader(http.StatusLocked)
		return
	}
	log.Debug2f("got new ping request from %s", r.RemoteAddr)
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Debug2f("error reading body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	r.Body.Close()
	var pingReq model.PingRequest
	if err := json.Unmarshal(b, &pingReq); err != nil {
		log.Debugf("invalid ping request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(pingReq.Hosts) == 0 {
		log.Warningf("%s - ping job with no host, rejecting", pingReq.UID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Debug3f("ping request: %+v", pingReq)
	if AddPingRequest(pingReq) {
		log.Debugf("%s - ping job successfully queued (%d hosts)", pingReq.UID, len(pingReq.Hosts))
		w.WriteHeader(http.StatusAccepted)
	} else {
		glog.Warningf("%s - no more workers, rejecting ping request", pingReq.UID)
		w.WriteHeader(http.StatusTooManyRequests)
	}
}
