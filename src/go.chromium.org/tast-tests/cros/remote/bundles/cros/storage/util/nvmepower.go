// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"regexp"
	"strconv"

	"go.chromium.org/tast/core/errors"
)

// APSTE defines an entry for an autonomous power state transitions
type APSTE struct {
	Delay  int
	Target int
}

// NvmePS defines supported power state
type NvmePS struct {
	Operational bool
	MaxPower    float64
	IdlePower   float64
	EntryLat    int
	ExitLat     int
}

func scaleNvmePower(unscaled, scale int) float64 {
	if scale == 1 {
		return float64(unscaled) * float64(0.0001)
	}
	if scale == 2 {
		return float64(unscaled) * float64(0.01)
	}
	return 0
}

func parseNvmePowerState(id []byte, psn int) NvmePS {
	// Bit field definitions are taken from NVMe 1.3 spec
	// Figures 109 and 113
	firstByte := 2048 + 32*psn

	ps := id[firstByte : firstByte+32]

	operational := getBit(ps, 25) == 0

	maxScale := 1
	if getBit(ps, 24) == 0 {
		maxScale = 2
	}
	maxPower := scaleNvmePower(swap2(ps[0:2]), maxScale)
	idleScale := (getBit(ps, 151) << 1) + getBit(ps, 150)
	idlePower := scaleNvmePower(swap2(ps[16:18]), idleScale)

	entryLat := swap4(ps[4:8])
	exitLat := swap4(ps[8:12])

	return NvmePS{
		Operational: operational,
		MaxPower:    maxPower,
		IdlePower:   idlePower,
		EntryLat:    entryLat,
		ExitLat:     exitLat,
	}
}

func readNvmePowerStates(ctx context.Context, disk *Disk) ([]NvmePS, error) {
	id, err := RunCmdWithOutput(ctx, disk.DUT, "nvme", "id-ctrl", "-b", disk.Path)
	if err != nil {
		return nil, errors.Wrap(err, "can't read controller identity")
	}

	var states []NvmePS

	npss := int(id[263])
	for ps := 0; ps <= npss; ps++ {
		states = append(states, parseNvmePowerState(id, ps))
	}

	return states, nil
}

func getNvmeFeatureCurrentValue(ctx context.Context, disk *Disk, featureHex string) (int64, error) {
	out, err := RunCmdWithStringOutput(ctx, disk.DUT, "nvme", "get-feature", "-f", featureHex, "-H", disk.Path)
	if err != nil {
		return -1, errors.Wrapf(err, "can't read feature %q for %q", featureHex, disk.Path)
	}

	valuePattern := `Current value:0x([0-9a-f]+)`
	re := regexp.MustCompile(valuePattern)
	match := re.FindStringSubmatch(out)
	if len(match) <= 1 {
		return -1, errors.New("can't parse feature value")
	}

	valueHex := match[1]
	value, err := strconv.ParseInt(valueHex, 16, 64)
	if err != nil {
		return -1, errors.Wrapf(err, "can't parse hex value %v", valueHex)
	}

	return value, nil
}

func getCurrentNvmePowerState(ctx context.Context, disk *Disk) (int64, error) {
	value, err := getNvmeFeatureCurrentValue(ctx, disk, "0x02")
	if err != nil {
		return -1, errors.Wrap(err, "can't get NVMe power state")
	}
	// Lower 5 bit define the power state, upper 3 bit - workload hint.
	return value & 0x1f, nil
}

func checkApstEnabled(ctx context.Context, disk *Disk) (bool, error) {
	// We need this function separately because binary format doesn't
	// report DWORD 0 - the enablement status of the feature.
	value, err := getNvmeFeatureCurrentValue(ctx, disk, "0x0c")
	if err != nil {
		return false, errors.Wrap(err, "can't get APST status")
	}
	return value > 0, nil
}

func readAPST(ctx context.Context, disk *Disk) ([]APSTE, error) {
	out, err := RunCmdWithOutput(ctx, disk.DUT, "nvme", "get-feature", "-f", "0x0c", "-b", disk.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "can't read APST for %q", disk.Path)
	}

	var apst []APSTE

	for ps := 0; ps < 32; ps++ {
		off := ps * 8
		target := int(out[off]) >> 3
		delay := swap4(append(out[off+1:off+4], 0))
		if target == 0 && delay == 0 {
			break
		}
		apst = append(apst, APSTE{Target: target, Delay: delay})
	}

	return apst, nil
}
