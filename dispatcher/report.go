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

package dispatcher

import (
	"net/http"
	"strconv"

	"horus/log"
)

// HandleReport saves the polling report to db and unlocks the device.
func HandleReport(w http.ResponseWriter, r *http.Request) {
	if !IsMaster {
		http.Error(w, "Unavailable On Slave", http.StatusServiceUnavailable)
		return
	}

	reqUID := r.FormValue("request_id")
	devID := r.FormValue("device_id")
	agentID := r.FormValue("agent_id")
	pollDur := r.FormValue("poll_duration_ms")
	pollErr := r.FormValue("poll_error")
	if pollDur == "" {
		pollDur = "0"
	}
	currLoad := r.FormValue("current_load")
	metricCount := r.FormValue("metric_count")
	log.Debugf("report: uid=%s device_id=%s agent_id=%s snmp_dur=%s snmp_err=`%s` metric_count=%s curr_load=%s",
		reqUID, devID, agentID, pollDur, pollErr, metricCount, currLoad)
	if err := sqlExec(reqUID, "unlockDevFromReportStmt", unlockDevFromReportStmt, devID); err != nil {
		log.Errorf("%s - unlock dev %s from request: %v", reqUID, devID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	currentAgentsMu.Lock()
	defer currentAgentsMu.Unlock()
	for i, agent := range currentAgents {
		if strconv.Itoa(agent.ID) == agentID {
			if load, err := strconv.ParseFloat(currLoad, 64); err == nil {
				currentAgents[i].setLoad(load)
			} else {
				log.Warningf("%s - unable to parse current_load: %v", reqUID, err)
			}
			break
		}
	}
	w.WriteHeader(http.StatusOK)
}
