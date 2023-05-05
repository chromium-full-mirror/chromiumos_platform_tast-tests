// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/local/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type thermalStateDataSource struct {
	ignoreChargerType bool
	coolingDevices    []*power.CoolingDevice
	metric            perf.Metric
}

// NewThermalStateDataSource creates a new instance of thermalStateDataSource.
// Charger cooling device may report bogus thermal state when device is on
// battery. Use `ignoreChargerType` to suppress it from reporting.
func NewThermalStateDataSource(ignoreChargerType bool) *thermalStateDataSource {
	return &thermalStateDataSource{
		ignoreChargerType: ignoreChargerType,
	}
}

// Setup implements perf.TimelineDatasource.Setup.
func (ds *thermalStateDataSource) Setup(ctx context.Context, prefix, intervalName string) error {
	devices, err := power.ListingSysfsCoolingDevices(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		testing.ContextLog(ctx, "No cooling devices found")
		return nil
	}

	ds.coolingDevices = devices
	ds.metric = perf.Metric{
		Name:      prefix + "ThermalState",
		Unit:      "thermal_state",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	return nil
}

// Start implements perf.TimelineDatasource.Start.
func (ds *thermalStateDataSource) Start(ctx context.Context) error {
	return nil
}

// Snapshot implements perf.TimelineDatasource.Snapshot.
func (ds *thermalStateDataSource) Snapshot(ctx context.Context, values *perf.Values) error {
	thermalState := power.ThermalStateUnknown

	// Iterate through all cooling devices and reports the highest state.
	for _, dev := range ds.coolingDevices {
		if dev.Type == power.CoolingDevTypeCharger && ds.ignoreChargerType {
			continue
		}

		deviceState, err := dev.GetThermalState(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to read %s", dev.CurStatePath)
		}

		if deviceState > thermalState {
			thermalState = deviceState
		}
	}

	values.Append(ds.metric, float64(thermalState))
	return nil
}

// Stop does nothing.
func (ds *thermalStateDataSource) Stop(_ context.Context, values *perf.Values) error {
	return nil
}
