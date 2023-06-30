// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cpu

import (
	"context"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
)

const (
	c0C1Key             = "C0_C1"
	aggregateNonC0C1Key = "non-C0_C1"
)

// FetchIntelCPUUarch gets the name of the x86 microarchitecture.
func FetchIntelCPUUarch() (string, error) {
	intelUarchTable := map[string]string{
		"06_9A": "Alder Lake",
		"06_BE": "Alder Lake",
		"06_4C": "Airmont",
		"06_1C": "Atom",
		"06_26": "Atom",
		"06_27": "Atom",
		"06_35": "Atom",
		"06_36": "Atom",
		"06_3D": "Broadwell",
		"06_47": "Broadwell",
		"06_4F": "Broadwell",
		"06_56": "Broadwell",
		"06_A5": "Comet Lake",
		"06_A6": "Comet Lake",
		"06_0D": "Dothan",
		"06_5C": "Goldmont",
		"06_7A": "Goldmont",
		"06_3C": "Haswell",
		"06_45": "Haswell",
		"06_46": "Haswell",
		"06_3F": "Haswell-E",
		"06_7D": "Ice Lake",
		"06_7E": "Ice Lake",
		"06_3A": "Ivy Bridge",
		"06_3E": "Ivy Bridge-E",
		"06_8E": "Kaby Lake",
		"06_9E": "Kaby Lake",
		"06_0F": "Merom",
		"06_16": "Merom",
		"06_AA": "Meteor Lake",
		"06_17": "Nehalem",
		"06_1A": "Nehalem",
		"06_1D": "Nehalem",
		"06_1E": "Nehalem",
		"06_1F": "Nehalem",
		"06_2E": "Nehalem",
		"0F_03": "Prescott",
		"0F_04": "Prescott",
		"0F_06": "Presler",
		"06_BA": "Raptor Lake",
		"06_2A": "Sandy Bridge",
		"06_2D": "Sandy Bridge",
		"06_37": "Silvermont",
		"06_4A": "Silvermont",
		"06_4D": "Silvermont",
		"06_5A": "Silvermont",
		"06_5D": "Silvermont",
		"06_4E": "Skylake",
		"06_5E": "Skylake",
		"06_55": "Skylake",
		"06_8C": "Tiger Lake",
		"06_8D": "Tiger Lake",
		"06_86": "Tremont",
		"06_96": "Tremont",
		"06_9C": "Tremont",
		"06_25": "Westmere",
		"06_2C": "Westmere",
		"06_2F": "Westmere",
	}
	out, err := ioutil.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", errors.Wrap(err, "failed to read cpuinfo")
	}

	family := int64(0)
	familyRegex := regexp.MustCompile(`(?m)^cpu family\s+:\s+([0-9]+)$`)
	matches := familyRegex.FindAllStringSubmatch(string(out), -1)
	if matches != nil {
		family, err = strconv.ParseInt(matches[0][1], 10, 64)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse family")
		}
	}

	model := int64(0)
	modelRegex := regexp.MustCompile(`(?m)^model\s+:\s+([0-9]+)$`)
	matches = modelRegex.FindAllStringSubmatch(string(out), -1)
	if matches != nil {
		model, err = strconv.ParseInt(matches[0][1], 10, 64)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse model")
		}
	}

	return intelUarchTable[fmt.Sprintf("%02X_%02X", family, model)], nil
}

// FetchPackageStates gets a map of the package C-states to msr addresses.
func FetchPackageStates() (map[string]int64, error) {
	atomStates := map[string]int64{"C2": 0x3F8, "C4": 0x3F9, "C6": 0x3FA}
	nehalemStates := map[string]int64{"C3": 0x3F8, "C6": 0x3F9, "C7": 0x3FA}
	sandyBridgeStates := map[string]int64{"C2": 0x60D, "C3": 0x3F8, "C6": 0x3F9, "C7": 0x3FA}
	silvermontStates := map[string]int64{"C6": 0x3FA}
	goldmontStates := map[string]int64{"C2": 0x60D, "C3": 0x3F8, "C6": 0x3F9, "C10": 0x632}
	broadwellStates := map[string]int64{"C2": 0x60D, "C3": 0x3F8, "C6": 0x3F9, "C7": 0x3FA,
		"C8": 0x630, "C9": 0x631, "C10": 0x632}
	// model groups pulled from Intel SDM, volume 4
	// Group same package cstate using the older uarch name
	fullStateMap := map[string]map[string]int64{
		"Airmont":      silvermontStates,
		"Alder Lake":   broadwellStates,
		"Atom":         atomStates,
		"Broadwell":    broadwellStates,
		"Comet Lake":   broadwellStates,
		"Goldmont":     goldmontStates,
		"Haswell":      sandyBridgeStates,
		"Ice Lake":     broadwellStates,
		"Ivy Bridge":   sandyBridgeStates,
		"Ivy Bridge-E": sandyBridgeStates,
		"Kaby Lake":    broadwellStates,
		"Meteor Lake":  broadwellStates,
		"Nehalem":      nehalemStates,
		"Raptor Lake":  broadwellStates,
		"Sandy Bridge": sandyBridgeStates,
		"Silvermont":   silvermontStates,
		"Skylake":      broadwellStates,
		"Tiger Lake":   broadwellStates,
		"Tremont":      goldmontStates,
		"Westmere":     nehalemStates,
	}

	uarch, err := FetchIntelCPUUarch()
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine uarch")
	}

	return fullStateMap[uarch], nil
}

// FindCPUPerPackage returns a slice that contains 1 CPU from each package.
func FindCPUPerPackage(ctx context.Context) ([]int, error) {
	packages := make(map[int]int)
	cpuInfos, err := ioutil.ReadDir("/dev/cpu")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read /dev/cpu")
	}
	for _, cpuInfo := range cpuInfos {
		cpu, err := strconv.ParseInt(cpuInfo.Name(), 10, 32)
		if err != nil {
			// Skip misc files in the directory
			continue
		}
		path := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/topology/physical_package_id", cpu)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(err, "failed to determine package")
		}

		pkgStr, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read package")
		}
		pkg, err := strconv.ParseInt(strings.TrimSpace(string(pkgStr)), 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse package")
		}
		packages[int(pkg)] = int(cpu)
	}
	perPackageCPUs := make([]int, len(packages))
	for i, cpu := range packages {
		perPackageCPUs[i] = cpu
	}
	return perPackageCPUs, nil
}

func readMSR(addr int64, cpu int) (uint64, error) {
	file, err := os.Open(fmt.Sprintf("/dev/cpu/%d/msr", cpu))
	if err != nil {
		return 0, errors.Wrap(err, "failed to open msrs")
	}
	defer file.Close()
	data := make([]byte, 8)
	if _, err := file.ReadAt(data, addr); err != nil {
		return 0, errors.Wrap(err, "failed to read msrs")
	}
	return binary.LittleEndian.Uint64(data), nil
}

// ReadPackageCStates takes a list of CPUs which correspond to the device's
// packages and the package C-states, returns a map containing how long was
// spent in each state.
func ReadPackageCStates(perPackageCPUs []int, pCStates map[string]int64) (map[string]uint64, error) {
	ret := make(map[string]uint64)
	ret[c0C1Key] = 0
	ret[aggregateNonC0C1Key] = 0

	for _, cpu := range perPackageCPUs {
		tsc, err := readMSR(0x10, cpu)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read tsc value")
		}
		ret[c0C1Key] += tsc
		for pcstate, addr := range pCStates {
			val, err := readMSR(addr, cpu)
			if err != nil {
				return nil, errors.Wrap(err, "failed to read pcstate msr")
			}
			ret[pcstate] = val
			ret[c0C1Key] -= val
			ret[aggregateNonC0C1Key] += val
		}
	}
	return ret, nil
}
