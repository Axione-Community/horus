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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/kosctelecom/horus/log"
	"github.com/kosctelecom/horus/model"
	"github.com/lib/pq"
)

// UnlockDevices retrieves all ongoing requests from all active agents
// and unlocks all devices without any polling job and whose last job
// is past its global polling frequency. Is called periodically on a
// separate goroutine.
func UnlockDevices() {
	agents := currentAgentsCopy()

	var currentReqs []string
	for _, agent := range agents {
		log.Debug2f("unlock dev: get ongoing from agent #%d (%s:%d)", agent.ID, agent.Host, agent.Port)
		client := &http.Client{Timeout: time.Duration(HTTPTimeout) * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s:%d%s", agent.Host, agent.Port, model.OngoingURI))
		if err != nil {
			log.Debug2f("agent #%d: get ongoing: %v", agent.ID, err)
			sqlExec("agent #"+strconv.Itoa(agent.ID), "unlockFromAgent", unlockFromAgentStmt, agent.ID)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Warningf("agent #%d: get ongoing: %s", agent.ID, resp.Status)
			sqlExec("agent #"+strconv.Itoa(agent.ID), "unlockFromAgent", unlockFromAgentStmt, agent.ID)
			continue
		}
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("agent #%d: get ongoing: read body: %v", agent.ID, err)
			continue
		}
		var ongoing model.OngoingPolls
		if err := json.Unmarshal(b, &ongoing); err != nil {
			log.Errorf("agent #%d: get ongoing: json unmarshal: %v", agent.ID, err)
			continue
		}
		currentReqs = append(currentReqs, ongoing.Requests...)
		log.Debugf("agent #%d: %d running jobs", agent.ID, len(ongoing.Requests))
	}
	log.Debugf("unlocking %d devices without ongoing poll", len(currentReqs))
	sqlExec("", "unlockFromOngoing", unlockFromOngoingStmt, pq.Array(currentReqs))
}
