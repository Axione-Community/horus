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

package model

import (
	"encoding/json"
	"errors"

	"github.com/lib/pq"
)

// Device represents an snmp device.
type Device struct {
	// ID is the device id.
	ID int `db:"id" json:"id"`

	// Active tells whether the device can be polled.
	Active bool `db:"active" json:"active"`

	// Hostname is the device's FQDN.
	Hostname string `db:"hostname" json:"hostname"`

	// PollingFrequency is the device's snmp polling frequency.
	PollingFrequency int `db:"polling_frequency" json:"polling_frequency"`

	// PingFrequency is the device's ping frequency
	PingFrequency int `db:"ping_frequency" json:"ping_frequency"`

	// Tags is the influx tags (and prometheus labels) added to
	// each measurement of this device.
	Tags string `db:"tags" json:"tags,omitempty"`

	// SnmpParams is the device snmp config.
	SnmpParams

	// Profile is the device profile.
	Profile

	// AllowedAgentIDs is a list of the IDs of the only agents allowed to poll this device.
	// No restriction if empty.
	AllowedAgentIDs pq.Int64Array `db:"allowed_agent_ids" json:"-"`
}

// UnmarshalJSON implements the json Unmarshaler interface for Device type.
// Takes a flat json and builds a Device with embedded Profile and SnmpParams.
// Note: the standard Marshaler also outputs a flat json document.
func (dev *Device) UnmarshalJSON(data []byte) error {
	type D Device
	var d D

	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	if d.ID == 0 {
		return errors.New("invalid device: id cannot be empty")
	}
	if d.Hostname == "" {
		return errors.New("invalid device: hostname cannot be empty")
	}

	var t map[string]interface{}
	if d.Tags == "" {
		d.Tags = "{}"
	}
	if json.Unmarshal([]byte(d.Tags), &t) != nil {
		return errors.New("invalid device: tags must be a valid json map")
	}
	if err := json.Unmarshal(data, &d.Profile); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &d.SnmpParams); err != nil {
		return err
	}
	*dev = Device(d)
	return nil
}
