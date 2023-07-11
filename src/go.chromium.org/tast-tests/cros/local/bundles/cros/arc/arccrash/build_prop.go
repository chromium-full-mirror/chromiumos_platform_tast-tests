// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arccrash provides utilities for tests of crash reporting.
package arccrash

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type buildProp struct {
	device      string
	board       string
	cpuAbi      string
	fingerprint string
}

func getProp(ctx context.Context, a *arc.ARC, key string) (string, error) {
	val, err := a.GetProp(ctx, key)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s", key)
	}
	if val == "" {
		return "", errors.Errorf("%s is empty", key)
	}
	return val, err
}

// GetBuildProp obtains the build property for ARC.
func GetBuildProp(ctx context.Context, a *arc.ARC) (*buildProp, error) {
	device, err := getProp(ctx, a, "ro.product.device")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device")
	}
	board, err := getProp(ctx, a, "ro.product.board")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get board")
	}
	cpuAbi, err := getProp(ctx, a, "ro.product.cpu.abi")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cpu_abi")
	}
	fingerprint, err := getProp(ctx, a, "ro.build.fingerprint")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get fingerprint")
	}

	return &buildProp{
		device:      device,
		board:       board,
		cpuAbi:      cpuAbi,
		fingerprint: fingerprint,
	}, nil
}

// UploadSystemBuildProp uploads /system/build.prop. This should be called to invetigate when GetPop failed. GetProp sometimes fails to get the device name
// even though the device name should always exists.
// See details in https://bugs.chromium.org/p/chromium/issues/detail?id=1039512#c16
func UploadSystemBuildProp(ctx context.Context, a *arc.ARC, outdir string) error {
	return a.PullFile(ctx, "/system/build.prop", filepath.Join(outdir, "build.prop"))
}

// FileContains checks that each value in |expectedValues| appears as a line in |filePath|.
func FileContains(ctx context.Context, filePath string, expectedValues []string) (bool, error) {
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return false, errors.Wrap(err, "failed to read meta file")
	}

	lines := strings.Split(string(b), "\n")
	for _, expectedValue := range expectedValues {
		found := false
		for _, l := range lines {
			if l == expectedValue {
				found = true
				break
			}
		}
		if !found {
			testing.ContextLogf(ctx, "Missing %q", expectedValue)
			return false, nil
		}
	}
	return true, nil
}

// ValidateBuildProp checks that given meta file for crash_sender contains the specified build properties.
func ValidateBuildProp(ctx context.Context, metafilePath string, bp *buildProp) (bool, error) {
	expectedValues := []string{
		"upload_var_device=" + bp.device,
		"upload_var_board=" + bp.board,
		"upload_var_cpu_abi=" + bp.cpuAbi,
		"upload_var_arc_version=" + bp.fingerprint,
	}

	ok, err := FileContains(ctx, metafilePath, expectedValues)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ValidateComputedSeverity checks that the given meta file contains the correct computed_severity and computed_product values.
func ValidateComputedSeverity(ctx context.Context, metafilePath string) (bool, error) {
	expectedValues := []string{
		"upload_var_client_computed_severity=ERROR",
		"upload_var_client_computed_product=Arc",
	}

	ok, err := FileContains(ctx, metafilePath, expectedValues)
	if err != nil {
		return false, err
	}
	return ok, nil
}
