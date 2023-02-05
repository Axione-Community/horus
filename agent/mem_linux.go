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

//go:build linux

package agent

import (
	"io/ioutil"
	"strconv"
	"strings"
	"syscall"
	"time"

	"horus/log"
)

var (
	totalMem             float64
	totalMemSamplingFreq = 60 * time.Minute
	sysPageSize          = syscall.Getpagesize()

	usedMem             float64
	usedMemSamplingFreq = 10 * time.Second
)

func updateTotalMem() {
	ticker := time.NewTicker(totalMemSamplingFreq)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			totalMem = getTotalMem()
		}
	}
}

func updateUsedMem() {
	ticker := time.NewTicker(usedMemSamplingFreq)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			usedMem = getUsedMem()
		}
	}
}

func getUsedMem() float64 {
	log.Debug3("retrieving mem stats from /proc")
	data, err := ioutil.ReadFile("/proc/self/stat")
	if err != nil {
		log.Errorf("read self/stat: %v", err)
		return 0
	}
	fields := strings.Fields(string(data))
	rss, err := strconv.ParseInt(fields[23], 10, 64)
	if err != nil {
		log.Errorf("parse RSS fields: %v", err)
		return 0
	}
	return float64(uintptr(rss) * uintptr(sysPageSize))
}

func getTotalMem() float64 {
	log.Debug3("retrieving sys total mem from /proc")
	data, err := ioutil.ReadFile("/proc/meminfo")
	if err != nil {
		log.Errorf("read meminfo: %v", err)
		return 0
	}
	ln := strings.Split(string(data), "\n")[0]
	memTotalField := strings.Fields(ln)[1]
	totalMem, err := strconv.ParseInt(memTotalField, 10, 64)
	if err != nil {
		log.Errorf("parse totalMem %q: %v", memTotalField, err)
		return 0
	}
	return float64(totalMem * 1024)
}

// CurrentMemLoad returns the current relative memory usage of the agent.
func CurrentMemLoad() float64 {
	if totalMem == 0 {
		return -1
	}
	return usedMem / totalMem
}
