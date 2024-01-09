// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hardwareprobe contains hardware_probe related functions for local tests.
package hardwareprobe

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	hardwareProbeBinary = "/usr/local/graphics/hardware_probe"
)

type pciDevice struct {
	BDF     string `json:"BDF"`
	Name    string `json:"Name"`
	BootVGA bool   `json:"BootVGA"`
}

type gpuInfo struct {
	Family string `json:"Family"`
	Vendor string `json:"GPUVendor"`
}

type disk struct {
	SizeGb int `json:"size_gb"`
}

type hardwareProbeResult struct {
	VGADevice []pciDevice `json:"VGA_Devices"`
	GPUInfo   []gpuInfo   `json:"GPU_Family"`
	CPUFamily string      `json:"CPU_SOC_Family"`
	Memory    int         `json:"Memory"`
	Disk      *disk       `json:"Disk"`
}

// GetHardwareProbeResult saves the information to path and returns detailed information gathered by hardware_probe binaries in the DUT.
func GetHardwareProbeResult(ctx context.Context) (hardwareProbeResult, error) {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return hardwareProbeResult{}, errors.New("failed to get outDir")
	}
	file := filepath.Join(outDir, "hardware_probe.json")
	out, err := testexec.CommandContext(ctx, hardwareProbeBinary, "-output", file).CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return hardwareProbeResult{}, errors.Wrapf(err, "failed to run %v, output: %v", hardwareProbeBinary, string(out))
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return hardwareProbeResult{}, errors.Wrap(err, "failed to read json")
	}
	var result hardwareProbeResult
	if err := json.Unmarshal(b, &result); err != nil {
		return hardwareProbeResult{}, errors.Wrapf(err, "failed to unmarshal data: %v", string(out))
	}
	return result, nil
}

// CPUFamily returns the CPU SOC family on the host. e.g. intel, amd, qualcomm, etc.
func CPUFamily(ctx context.Context) (string, error) {
	result, err := GetHardwareProbeResult(ctx)
	if err != nil {
		return "", err
	}
	return result.CPUFamily, nil
}

// GPUFamilies returns the GPU families on the host. e.g. qualcomm, broadwell, kabylake, cezanne, etc.
func GPUFamilies(ctx context.Context) ([]string, error) {
	result, err := GetHardwareProbeResult(ctx)
	if err != nil {
		return nil, err
	}
	var families []string
	for _, info := range result.GPUInfo {
		families = append(families, info.Family)
	}
	return families, nil
}
