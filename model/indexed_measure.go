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
	"fmt"
	"regexp"

	"horus/log"
)

// IndexedMeasure is a group of tabular metrics indexed by the first one.
type IndexedMeasure struct {
	// ID is the measure db id.
	ID int `db:"id"`

	// Name is the name of the indexed measure.
	Name string `db:"name"`

	// Description is the description of the indexed measure.
	Description string `db:"description"`

	// Metrics is the list of metrics forming this measure.
	Metrics []Metric

	// IndexMetricID is the id of the metric used as index.
	IndexMetricID NullInt64 `db:"index_metric_id"`

	// IndexMetric is the name for the metric used as index.
	IndexMetric string `db:"-"`

	// IndexPos is the position of the index metric in the Metrics array.
	IndexPos int `db:"-"`

	// FilterPattern is the regex pattern used to filter the IndexedResults of this metric group.
	// It can be used to only keep results from interesting interfaces.
	FilterPattern string `db:"filter_pattern"`

	// FilterMetricID is the id of the metric on which the filter is applied.
	FilterMetricID NullInt64 `db:"filter_metric_id"`

	// FilterMetric is the name of the metric to filter against.
	FilterMetric string `db:"-"`

	// FilterPos is the index of the filter metric in the Metrics array.
	FilterPos int `db:"-"`

	// InvertFilterMatch negates the match result of the FilterPattern.
	InvertFilterMatch bool `db:"invert_filter_match"`

	// FilterRegex is the compiled FilterPattern pattern.
	FilterRegex *regexp.Regexp `db:"-" json:"-"`

	// UseAlternateCommunity tells wether to use the alternate community for all metrics of this measure.
	UseAlternateCommunity bool `db:"use_alternate_community"`

	// ToKafka is a flag telling if the results are exported to Kafka.
	ToKafka bool `db:"to_kafka"`

	// ToProm tells if the results are kept for Prometheus scraping.
	ToProm bool `db:"to_prometheus"`

	// ToInflux is a flag telling if the results are exported to InfluxDB.
	ToInflux bool `db:"to_influx"`

	// ToNats is a flag telling if the results are exported to NATS.
	ToNats bool `db:"to_nats"`

	// LabelsOnly tells wether this measure contains only labels.
	LabelsOnly bool `db:"-"`
}

// UnmarshalJSON unserializes data into an IndexedMetric.
// Checks specifically if the filter index and pattern are valid.
func (x *IndexedMeasure) UnmarshalJSON(data []byte) error {
	type IM IndexedMeasure
	var im IM

	if err := json.Unmarshal(data, &im); err != nil {
		return err
	}
	if im.IndexMetricID.Valid {
		for _, metric := range im.Metrics {
			if int64(metric.ID) == im.IndexMetricID.Int64 {
				im.IndexMetric = metric.Name
				break
			}
		}
		if im.IndexMetric == "" {
			return fmt.Errorf("indexed measure %s: IndexMetricID %d not found in metric list", im.Name, im.IndexMetricID.Int64)
		}
	}
	if im.FilterPattern != "" && !im.FilterMetricID.Valid {
		return fmt.Errorf("indexed measure %s: FilterMetricID cannot be null when FilterPattern is defined", im.Name)
	}
	if im.FilterPattern == "" && im.FilterMetricID.Valid {
		return fmt.Errorf("indexed measure %s: FilterPattern cannot be empty when FilterMetricID is defined", im.Name)
	}
	if im.FilterPattern != "" {
		for _, metric := range im.Metrics {
			if int64(metric.ID) == im.FilterMetricID.Int64 {
				im.FilterMetric = metric.Name
				break
			}
		}
		if im.FilterMetric == "" {
			return fmt.Errorf("indexed measure: invalid FilterMetricID %d, not in metric list", im.FilterMetricID.Int64)
		}
		var err error
		if im.FilterRegex, err = regexp.Compile(im.FilterPattern); err != nil {
			return fmt.Errorf("invalid filter regexp: %v", err)
		}
	}
	*x = IndexedMeasure(im)
	return nil
}

// RemoveInactive filters out all metrics of this indexed measure that are marked as inactive.
func (x *IndexedMeasure) RemoveInactive() {
	filtered := x.Metrics[:0]
	for _, metric := range x.Metrics {
		if metric.Active {
			filtered = append(filtered, metric)
		}
	}
	if x.IndexMetricID.Valid {
		// recompute index position
		for i, metric := range filtered {
			if int64(metric.ID) == x.IndexMetricID.Int64 {
				x.IndexPos = i
				break
			}
		}
	}
	log.Debug3f("metrics before filter: %v", Names(x.Metrics))
	x.Metrics = filtered
	log.Debug3f("metrics after inactive filter: %v", Names(filtered))
}
