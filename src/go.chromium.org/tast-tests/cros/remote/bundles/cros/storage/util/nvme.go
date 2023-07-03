// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// FindSingleHexInt64NvmeRegs parses out a register hex value out of the
// identity report of the controller, e.g.
// ver       : 10400
// TODO(dlunev) should it return error instead of bool?
func FindSingleHexInt64NvmeRegs(ctx context.Context, identity, key string) (int64, bool) {
	pattern := key + `\s+:\s+(?:0x)?([0-9a-fA-F]+)`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(identity)
	if len(match) <= 1 {
		return 0, false
	}
	valueHex := match[1]
	value, err := strconv.ParseInt(valueHex, 16, 64)
	if err != nil {
		testing.ContextLogf(ctx, "Can't parse hex value %v: %q", valueHex, err)
		return 0, false
	}
	return value, true
}

// GetTBWFromInfo finds and parses TBW in NVMe storage info.
func GetTBWFromInfo(ctx context.Context, smartInfo string) (int64, error) {
	pattern := `Data Units Written:\s+([0-9\,]+)`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(smartInfo)
	if len(match) <= 1 {
		return 0, errors.New("can't find TBW field")
	}

	duwStr := strings.ReplaceAll(match[1], ",", "")
	duw, err := strconv.ParseInt(duwStr, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "can't parse local int value %v", duwStr)
	}

	// SMART reports Data Units written 512,000 bytes units
	return duw * 512 * 1000, nil
}

// NvmePowerConfig contains Power Config of an NVMe device
type NvmePowerConfig struct {
	States      []NvmePS
	ApstEnabled bool
	Apst        []APSTE
}

func readNvmePowerConfig(ctx context.Context, disk *Disk) (*NvmePowerConfig, error) {
	config := &NvmePowerConfig{
		States:      []NvmePS{},
		ApstEnabled: false,
		Apst:        []APSTE{},
	}

	if disk.Type != NvmeDisk {
		return nil, errors.New("This function supports only NVMe")
	}

	states, err := readNvmePowerStates(ctx, disk)
	if err != nil {
		return nil, errors.Wrap(err, "can't read power states")
	}

	config.States = states

	apstEnabled, err := checkApstEnabled(ctx, disk)
	if err != nil {
		return nil, errors.Wrap(err, "can't check APST state")
	}

	config.ApstEnabled = apstEnabled
	if !config.ApstEnabled {
		return config, nil
	}

	apst, err := readAPST(ctx, disk)
	if err != nil {
		return nil, errors.Wrap(err, "can't read APST table")
	}
	config.Apst = apst
	return config, nil
}

func getNvmePowerState(ctx context.Context, disk *Disk) (DiskPowerState, error) {
	state, err := getCurrentNvmePowerState(ctx, disk)
	if err != nil {
		return DiskUnknownPowerState, errors.Wrap(err, "can't get NVMe current power state")
	}

	if disk.nvmePowerConfig.States[state].Operational {
		return DiskHighPowerState, nil
	}
	return DiskLowPowerState, nil
}

func debugPrintNvmePowerConfig(ctx context.Context, c *NvmePowerConfig) {
	testing.ContextLog(ctx, "NVMe power config")
	testing.ContextLog(ctx, "Power States")
	for id, ps := range c.States {
		testing.ContextLogf(
			ctx,
			"%d %v %f %f %d %d",
			id, ps.Operational, ps.MaxPower,
			ps.IdlePower, ps.EntryLat, ps.ExitLat)
	}
	testing.ContextLog(ctx, "APST Enabled: ", c.ApstEnabled)
	for id, apste := range c.Apst {
		testing.ContextLogf(ctx, "%d %d %d", id, apste.Target, apste.Delay)
	}
}
