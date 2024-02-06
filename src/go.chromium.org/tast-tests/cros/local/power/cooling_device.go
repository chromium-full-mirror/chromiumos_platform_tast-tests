// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"math"
	"os"
	"path"
	"strings"

	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
)

// CoolingDevType denotes the type of cooling devices.
type CoolingDevType int

// Cooling device types.
const (
	CoolingDevTypeProcessor CoolingDevType = iota
	CoolingDevTypeFan
	CoolingDevTypeCharger
	CoolingDevTypeOther
)

type thresholdScale struct {
	fair     float64
	serious  float64
	critical float64
}

func deviceTypeFromString(s string) CoolingDevType {
	if s == "Processor" {
		return CoolingDevTypeProcessor
	}
	if s == "thermal-cpu-freq" {
		return CoolingDevTypeProcessor
	}

	if strings.Index(s, "TFN1") >= 0 {
		return CoolingDevTypeFan
	}

	if strings.Index(s, "TCHG") >= 0 {
		return CoolingDevTypeCharger
	}

	return CoolingDevTypeOther
}

// CoolingDevice reads state from underlying cooling devices and calculates
// ThermalState.
type CoolingDevice struct {
	CurStatePath string
	Type         CoolingDevType

	thresholdFair     float64
	thresholdSerious  float64
	thresholdCritical float64

	maxState int64
}

// NewCoolingDevice creates a CoolingDevice for the given path.
func NewCoolingDevice(ctx context.Context, devPath string) (*CoolingDevice, error) {
	if _, err := os.Stat(devPath); err != nil {
		return nil, errors.New("cooling device " + devPath + " does not exist")
	}

	maxState, err := util.ReadInt64(ctx, path.Join(devPath, "max_state"))
	if err != nil {
		return nil, errors.New("failed to read max_state for cooling device " + devPath)
	}

	curStatePath := path.Join(devPath, "cur_state")
	_, err = util.ReadInt64(ctx, curStatePath)
	if err != nil {
		return nil, errors.New("failed to read cur_state for cooling device " + devPath)
	}

	devTypeString, err := util.ReadFirstLine(ctx, path.Join(devPath, "type"))
	if err != nil {
		return nil, errors.New("failed to read type for cooling device " + devPath)
	}

	thresholdScaleByType := map[CoolingDevType]*thresholdScale{
		CoolingDevTypeProcessor: &thresholdScale{
			fair:     0.1,
			serious:  0.5,
			critical: 0.8,
		},
		CoolingDevTypeFan: &thresholdScale{
			fair:     0.5,
			serious:  1.0,
			critical: 2.0,
		},
		CoolingDevTypeCharger: &thresholdScale{
			fair:     0.7,
			serious:  1.0,
			critical: 2.0,
		},
		CoolingDevTypeOther: &thresholdScale{
			fair:     0.5,
			serious:  0.8,
			critical: 1.0,
		},
	}

	devType := deviceTypeFromString(devTypeString)
	scale := thresholdScaleByType[devType]
	return &CoolingDevice{
		CurStatePath:      curStatePath,
		Type:              devType,
		thresholdFair:     math.Ceil(float64(maxState) * scale.fair),
		thresholdSerious:  math.Ceil(float64(maxState) * scale.serious),
		thresholdCritical: math.Ceil(float64(maxState) * scale.critical),
		maxState:          maxState,
	}, nil
}

// GetThermalState returns ThermalState derived from the sysfs reading.
func (c *CoolingDevice) GetThermalState(ctx context.Context) (ThermalState, error) {
	curState, err := util.ReadInt64(ctx, c.CurStatePath)
	if err != nil {
		return ThermalStateUnknown, errors.New("failed to read " + c.CurStatePath)
	}

	if curState < 0 || curState > c.maxState {
		return ThermalStateUnknown, errors.Errorf("invalid curState: %v", curState)
	}

	if c.maxState == 0 {
		return ThermalStateUnknown, nil
	}

	if float64(curState) >= c.thresholdCritical {
		return ThermalStateCritical, nil
	}
	if float64(curState) >= c.thresholdSerious {
		return ThermalStateSerious, nil
	}
	if float64(curState) >= c.thresholdFair {
		return ThermalStateFair, nil
	}
	return ThermalStateNominal, nil
}
