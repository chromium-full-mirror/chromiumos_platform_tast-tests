// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CaptureDeviceSnapshot gets a snapshot of performance values for the DUT
// at the time of the function call. This returns a list of perf.Values
// containing the following metrics:
// 1. CPU usage percentage,
// 2. CPU Temperature (in celsius),
// 3. RAM usage percentage,
// 4. Battery Level.
func CaptureDeviceSnapshot(ctx context.Context, prefix string) (*perf.Values, error) {
	testing.ContextLog(ctx, "Capturing initial device state: ")

	pv := perf.NewValues()

	cpuUsage, err := cpu.MeasureUsage(ctx, time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to measure CPU usage")
	}

	memInfo, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read memory metrics")
	}

	cpuTemp, _, err := cpu.Temperature(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the CPU temperature")
	}

	var batteryCapacity float64
	batteryPath, err := metrics.SysfsBatteryPath(ctx)
	if err != nil {
		// Skip reporting battery capacity on devices that might not have a
		// battery, like Chromeboxes.
		testing.ContextLog(ctx, "Skipping reporting initial battery percentage: ", err)
	} else {
		batteryCapacity, err = metrics.ReadBatteryCapacity(ctx, batteryPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read battery capacity")
		}
	}

	testing.ContextLog(ctx, "CPU (%): ", cpuUsage)
	pv.Set(perf.Metric{
		Name:      fmt.Sprintf("%s.%s", prefix, "CPU"),
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, cpuUsage)

	// Convert milli-celsius to celsius by dividing by 1000.
	testing.ContextLog(ctx, "CPU Temp (celsius): ", float64(cpuTemp)/1000)
	pv.Set(perf.Metric{
		Name:      fmt.Sprintf("%s.%s", prefix, "CPU.Temperature"),
		Unit:      "celsius",
		Direction: perf.SmallerIsBetter,
	}, float64(cpuTemp)/1000)

	testing.ContextLog(ctx, "RAM (%): ", memInfo.UsedPercent)
	pv.Set(perf.Metric{
		Name:      fmt.Sprintf("%s.%s", prefix, "RAM"),
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, memInfo.UsedPercent)

	if batteryCapacity > 0.1 {
		testing.ContextLog(ctx, "Battery Capacity (%): ", batteryCapacity)
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s.%s", prefix, "BatteryCapacity"),
			Unit:      "percent",
			Direction: perf.BiggerIsBetter,
		}, batteryCapacity)
	}
	return pv, nil
}
