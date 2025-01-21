// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package battery provides control to an DUT's battery.
package battery

import (
	"encoding/csv"
	"fmt"
	"os"
)

// DataPoint represents a single metric data point
type DataPoint struct {
	Timestamp            string
	BatteryPercent       float64
	ServoChargingVoltage float64
	ServoChargingCurrent float64
}

// MetricMonitor stores metric points
type MetricMonitor struct {
	DataPoints []DataPoint
}

// NewMetricMonitor provides initial MetricMonitor
func NewMetricMonitor() *MetricMonitor {
	return &MetricMonitor{
		DataPoints: []DataPoint{},
	}
}

// AddMetric save DataPoint
func (m *MetricMonitor) AddMetric(data DataPoint) {
	m.DataPoints = append(m.DataPoints, data)
}

// SaveCSV saves the collected metrics to a CSV file
func (m *MetricMonitor) SaveCSV(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Set the delimiter to semicolon (;)
	writer.Comma = ';'

	headers := []string{
		"Timestamp",
		"BatteryPercent",
		"ServoChargingVoltage",
		"ServoChargingCurrent",
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	for _, d := range m.DataPoints {
		row := []string{
			d.Timestamp, // Assuming Timestamp is already in the correct format
			fmt.Sprintf("%.3f", d.BatteryPercent),
			fmt.Sprintf("%.3f", d.ServoChargingVoltage),
			fmt.Sprintf("%.3f", d.ServoChargingCurrent),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
