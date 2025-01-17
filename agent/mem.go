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

//go:build !linux

package agent

var (
	totalMem float64
	usedMem  float64
)

// updateTotalMem is a noop on non linux systems
func updateTotalMem() {}

// updateUsedMem is a noop on non linux systems
func updateUsedMem() {}

// CurrentMemLoad returns 0 on non linux systems.
func CurrentMemLoad() float64 {
	return 0
}
