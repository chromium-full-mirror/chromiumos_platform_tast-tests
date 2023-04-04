// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uidetection

import (
	"context"

	pb "google.golang.org/genproto/googleapis/chromeos/uidetection/v1"

	"runtime/debug"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/lsbrelease"
	"chromiumos/tast/testing"
	"regexp"
)

// DeviceInfo describes the device under test.
type DeviceInfo struct {
	Board     string
	Model     string
	CrosBuild string
}

// TestMetadata returns metadata about the current test and test device.
func TestMetadata(ctx context.Context, uda *Context) *pb.TestMetadata {
	// Initialise DUT info.
	if uda.deviceInfo == nil {
		uda.deviceInfo = fetchDeviceInfo(ctx)
	}

	baseTestName, err := baseTestName()
	if err != nil {
		testing.ContextLog(ctx, "INFO: couldn't find the test name for uidetection analytics: ", err)
		baseTestName = ""
	}

	return &pb.TestMetadata{
		TestId:    baseTestName,
		Board:     uda.deviceInfo.Board,
		Model:     uda.deviceInfo.Model,
		CrosBuild: uda.deviceInfo.CrosBuild,
	}
}

func fetchDeviceInfo(ctx context.Context) *DeviceInfo {
	modelName, err := modelName(ctx)
	if err != nil {
		testing.ContextLog(ctx, "INFO: couldn't find the DUT model name for uidetection analytics: ", err)
		modelName = ""
	}

	lsb, err := lsbrelease.Load()
	if err != nil {
		testing.ContextLog(ctx, "INFO: couldn't find the DUT board and build for uidetection analytics: ", err)
	}

	return &DeviceInfo{
		Board:     lsb[lsbrelease.Board],
		Model:     modelName,
		CrosBuild: lsb[lsbrelease.BuilderPath],
	}
}

// baseTestName finds the name of the calling Tast test, excluding parameters.
// For example, 'tast.uidetection.BasicDetections'.
func baseTestName() (string, error) {
	// Search the stack trace for the test file, looking for a line similar to:
	// 'chromiumos/tast/local/bundles/bundlename/category/test_name.go:'.
	stack := string(debug.Stack())
	r := regexp.MustCompile(`bundles\/[^\/]+\/([^\/]+)\/([^\/]+)\.go:`)
	result := r.FindStringSubmatch(stack)
	if result == nil {
		return "", errors.New("found no matches searching the call stack for the test name")
	}
	if len(result) != 3 {
		return "", errors.New("found an incorrect number of matches searching the call stack for the test name")
	}

	// Join in the format 'tast.uidetection.BasicDetections'.
	testName := "tast." + result[1] + "." + snakeToPascalCase(result[2])
	return testName, nil
}

// modelName finds the model of the current DUT.
func modelName(ctx context.Context) (string, error) {
	stdout, err := testexec.CommandContext(ctx, "cros_config", "/", "name").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to query cros_config for model name")
	}
	return string(stdout), nil
}
