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
	"time"

	"horus/log"
	"horus/model"

	"github.com/lib/pq"
)

// UnlockDevices retrieves all ongoing requests from all active agents
// and unlocks all devices without any polling job and whose last job is
// over maxLockTime seconds. Is called periodically on a separate goroutine.
func UnlockDevices(maxLockTime int) {
	agents := currentAgentsCopy()

	var currentDevs []int
	for _, agent := range agents {
		log.Debug2f("unlock dev: get ongoing from agent #%d (%s:%d)", agent.ID, agent.Host, agent.Port)
		client := &http.Client{Timeout: time.Duration(HTTPTimeout) * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s:%d%s", agent.Host, agent.Port, model.OngoingURI))
		if err != nil {
			log.Debug2f("agent #%d: get ongoing: %v", agent.ID, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			log.Warningf("agent #%d: get ongoing: %s", agent.ID, resp.Status)
			resp.Body.Close()
			continue
		}
		b, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Errorf("agent #%d: get ongoing: read body: %v", agent.ID, err)
			continue
		}
		var ongoing model.OngoingPolls
		if err := json.Unmarshal(b, &ongoing); err != nil {
			log.Errorf("agent #%d: get ongoing: json unmarshal: %v", agent.ID, err)
			continue
		}
		currentDevs = append(currentDevs, ongoing.Devices...)
		log.Debugf("agent #%d: %d running jobs", agent.ID, len(ongoing.Devices))
	}
	if len(currentDevs) > 0 {
		log.Debugf("unlocking devices without ongoing poll")
		sqlExec("", "unlockFromOngoing", unlockFromOngoingStmt, pq.Array(currentDevs))
	}
	log.Debugf("unlocking all devices with last poll time older than %ds", maxLockTime)
	sqlExec("", "unlockAllDev", unlockAllDevStmt, maxLockTime)
}
