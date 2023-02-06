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
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"horus/log"
	"horus/model"
)

// loadHistory is the agent's previous loads, used for load avg calculation.
type loadHistory struct {
	// loads is a load map whose key is the timestamp returned by time.UnixNano()
	// On each job post or keepalive, a new value is added and entries older than LoadAvgWindow are removed.
	loads map[int64]float64
	sync.Mutex
}

// Agent represents an snmp agent
type Agent struct {
	// ID is the agent id
	ID int `db:"id"`

	// Host is the agent web server IP address
	Host string `db:"ip_address"`

	// Port is the agent web server listen port
	Port int `db:"port"`

	// Alive indicates wether this agent responds to keep-alives
	Alive bool `db:"is_alive"`

	// name is the agent's unique name (ip:port)
	name string

	// snmpJobURL is the full url for posting agent's snmp jobs
	snmpJobURL string

	// checkURL is the full url for pinging this agent
	checkURL string

	// pingJobURL is the full url for posting agent's ping jobs
	pingJobURL string

	// lh is the agent load history
	lh *loadHistory

	// loadAvg is the load average taken over LoadAvgWindow.
	loadAvg float64
}

// Agents is a map of Agent pointers with agent name as key.
type Agents map[string]*Agent

// ByLoad is an Agent slice implementing Sort interface
// for sorting by average load.
type ByLoad []*Agent

func (a ByLoad) Len() int           { return len(a) }
func (a ByLoad) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByLoad) Less(i, j int) bool { return a[i].loadAvg < a[j].loadAvg }

var (
	// MaxLoadDelta is the maximum average load difference allowed between agents
	// before moving a device to the least loaded agent.
	MaxLoadDelta float64 = 0.1

	// LoadAvgWindow is the window for agent snmp load average calculation.
	LoadAvgWindow time.Duration

	// currentAgents is the list of currently active agents in memory.
	currentAgents   = make(Agents)
	currentAgentsMu sync.RWMutex

	// jobDistrib maps a device to an agent (dev id => agent name)
	jobDistrib   = make(map[int]string)
	jobDistribMu sync.RWMutex
)

// AgentsForDevice returns a list of agents to which a polling request can be sent by order
// of priority. We try to be sticky as much as possible but with balanced load:
// - the current list of active agents is sorted by load
// - if the device is not in jobDistrib map, this list is returned as is.
// - if the device is in jobDistrib map and its associated agent is active,
//   - if the load difference between the associated agent and the least
//     loaded agent is under the MaxLoadDelta, we stick to this agent: a
//     modified load-sorted list is returned where this agent is moved at
//     the first position.
//   - if the load difference exceeds MaxLoadDelta, we rebalance the load:
//     the load sorted list is returned.
// - if the device has an agent restriction (allowed agent list), we remove all other agents from the final
//   list before returning it. In this case, a device can only be polled by one of the affected agents.
func AgentsForDevice(dev *model.Device) []*Agent {
	var selectedAgents []*Agent
	currAgents := currentAgentsCopy()
	for k, a := range currAgents {
		if a.Alive {
			selectedAgents = append(selectedAgents, currAgents[k])
		}
	}
	if len(selectedAgents) == 0 {
		return nil
	}

	log.Debug3f("dev#%d: working agents: %+v", dev.ID, selectedAgents)
	sort.Sort(ByLoad(selectedAgents))
	jobDistribMu.RLock()
	index := getAgentIndex(jobDistrib[dev.ID], selectedAgents)
	jobDistribMu.RUnlock()
	if index == -1 {
		// previously used agent is not in list: send load sorted list
		log.Debug2f("dev#%d: not in job list, req sent to load sorted agents [%s,...] ", dev.ID, selectedAgents[0])
		return filterAllowedAgents(dev, selectedAgents)
	}
	// previously used agent is in list
	agent := selectedAgents[index]
	loadDelta := agent.loadAvg - selectedAgents[0].loadAvg
	if loadDelta <= MaxLoadDelta {
		// acceptable load delta, use same agent first
		selectedAgents = append(selectedAgents[:index], selectedAgents[index+1:]...) // remove
		selectedAgents = append([]*Agent{agent}, selectedAgents...)                  // unshift
		log.Debug2f("dev#%d: stick to prev (%s), delta=%.2f", dev.ID, agent, loadDelta)
	} else {
		log.Debug2f("dev#%d: req sent to load sorted agents [%s,...], delta=%.2f", dev.ID, selectedAgents[0], loadDelta)
	}
	return filterAllowedAgents(dev, selectedAgents)
}

// filterAllowedAgents returns only allowed agents for this device from the agents list.
// Returns initial list if allowed agents list is empty for this device (no restriction).
func filterAllowedAgents(dev *model.Device, agents []*Agent) []*Agent {
	if len(dev.AllowedAgentIDs) == 0 {
		return agents
	}

	log.Debug2f("dev#%d: has allowed agents list, filtering %d agents", dev.ID, len(agents))
	var filtered []*Agent
	for _, a := range agents {
		for _, id := range dev.AllowedAgentIDs {
			if a.ID == int(id) {
				filtered = append(filtered, a)
				break
			}
		}
	}
	log.Debug3f("dev#%d: filtered agents: %d", dev.ID, len(filtered))
	return filtered
}

// CheckAgents sends a keepalive to each agent
// and updates its status & current load.
func CheckAgents() error {
	log.Debug2("start checking agents")

	// make a local copy as check reply can be long
	agents := currentAgentsCopy()
	deadAgents := make(Agents)
	for _, agent := range agents {
		isAlive, load := agent.Check()
		agent.Alive = isAlive
		agent.setLoad(load)
		log.Debugf("%s: alive=%v load=%.2f", agent, isAlive, load)
		sqlExec("agent #"+strconv.Itoa(agent.ID), "checkAgentStmt", checkAgentStmt, agent.ID, isAlive, agent.loadAvg)
		if !isAlive {
			deadAgents[agent.name] = agent
		}
	}
	jobDistribMu.Lock()
	defer jobDistribMu.Unlock()
	for devID, agentName := range jobDistrib {
		// remove all mappings to dead agents
		if _, ok := deadAgents[agentName]; ok {
			delete(jobDistrib, devID)
		}
	}
	log.Debug2("done checking agents")
	return nil
}

// Check pings an agent and returns its active status and ongoing polls count.
// The check is a http query to the agents checkURL which returns a status 200 OK and
// the current load in body when it is healthy.
func (a Agent) Check() (bool, float64) {
	log.Debug2f("checking agent #%d", a.ID)
	client := &http.Client{Timeout: time.Duration(HTTPTimeout) * time.Second}
	resp, err := client.Get(a.checkURL)
	if err != nil {
		log.Debug2f("check agent %s: %v", a.name, err)
		return false, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Warningf("agent #%d responded to check with %s", a.ID, resp.Status)
		return false, 0
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("agent #%d: read check reply read: %v", a.ID, err)
		return false, 0
	}
	load, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		log.Errorf("agent #%d: reply parse: %v", a.ID, err)
	}
	return true, load
}

// String implements the stringer interface for the Agent type.
func (a Agent) String() string {
	return fmt.Sprintf("Agent<id:%d name:%s:%d load:%.4f>", a.ID, a.Host, a.Port, a.loadAvg)
}

// ActiveAgentCount returns the number of current active agents.
func ActiveAgentCount() int {
	count := 0
	currentAgentsMu.RLock()
	defer currentAgentsMu.RUnlock()
	for _, agent := range currentAgents {
		if agent.Alive {
			count++
		}
	}
	return count
}

// LoadAgents retrieves agent list from db and updates the current agent list
// (removes deleted, adds new).
// Note: key for comparision is agent name (host:port).
func LoadAgents() error {
	var agents []Agent
	err := db.Select(&agents, `SELECT id, ip_address, port, is_alive
                                 FROM agents
                                WHERE active = true
                             ORDER BY load`)
	if err != nil {
		return fmt.Errorf("load agents: %v", err)
	}
	log.Debugf("got %d agents from db", len(agents))
	newAgents := make(Agents)
	for _, a := range agents {
		a := a // !!shadowing needed for last assignment
		a.snmpJobURL = fmt.Sprintf("http://%s:%d%s", a.Host, a.Port, model.SnmpJobURI)
		a.checkURL = fmt.Sprintf("http://%s:%d%s", a.Host, a.Port, model.CheckURI)
		a.pingJobURL = fmt.Sprintf("http://%s:%d%s", a.Host, a.Port, model.PingJobURI)
		a.name = fmt.Sprintf("%s:%d", a.Host, a.Port)
		a.lh = &loadHistory{loads: map[int64]float64{}}
		newAgents[a.name] = &a
	}
	log.Debug2f("LoadAgents: new agents = %+v", newAgents)

	agentsCopy := currentAgentsCopy() // copy holds a rlock, must be called outside of next line lock
	currentAgentsMu.Lock()
	defer currentAgentsMu.Unlock()
	for k := range agentsCopy {
		if _, ok := newAgents[k]; !ok {
			delete(currentAgents, k)
		} else {
			delete(newAgents, k)
		}
	}
	for k, a := range newAgents {
		currentAgents[k] = a
	}
	log.Debug2f(">> LoadAgents: curr agents = %+v", currentAgents)
	return nil
}

// setLoad saves agents last instataneous load, updates its
// load average and removes old load samples.
func (a *Agent) setLoad(load float64) {
	a.lh.Lock()
	defer a.lh.Unlock()

	if !a.Alive {
		a.lh.loads = map[int64]float64{}
		a.loadAvg = 0
		return
	}

	var acc float64
	now := time.Now().UnixNano()
	a.lh.loads[now] = load
	for ts, load := range a.lh.loads {
		if ts < now-int64(LoadAvgWindow) {
			delete(a.lh.loads, ts)
		} else {
			acc += load
		}
	}
	a.loadAvg = acc / float64(len(a.lh.loads))
}

// currentAgentsCopy makes a locked copy of current agents map.
func currentAgentsCopy() Agents {
	cpy := make(Agents)
	currentAgentsMu.RLock()
	defer currentAgentsMu.RUnlock()
	for k, a := range currentAgents {
		cpy[k] = a
	}
	return cpy
}

// getAgentIndex returns the index at which the agent with `name`
// is in the `agents` array. Returns -1 if not found.
func getAgentIndex(name string, agents []*Agent) int {
	for i, a := range agents {
		if a.name == name {
			return i
		}
	}
	return -1
}
