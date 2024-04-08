// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debug

import (
	"context"
	"encoding/json"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/errors"
)

// Dump fetches the audio thread debug dump.
func Dump(ctx context.Context) (*AudioDebugInfo, error) {
	cmd := testexec.CommandContext(ctx, "cras_tests", "control", "dump_audio_debug_info", "--json")
	output, err := cmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "call cras_tests")
	}
	info := &AudioDebugInfo{}
	if err := json.Unmarshal(output, info); err != nil {
		return nil, errors.Wrap(err, "unmarshal debug info json")
	}
	return info, nil
}

// ListDevices returns the list of input or output device info.
func ListDevices(ctx context.Context, isInput bool) ([]IodevInfo, error) {
	listType := "list_output_devices"
	if isInput {
		listType = "list_input_devices"
	}

	var devices []IodevInfo
	cmd := testexec.CommandContext(ctx, "cras_tests", "control", listType, "--json")
	output, err := cmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return devices, errors.Wrap(err, "call cras_tests")
	}
	if err := json.Unmarshal(output, &devices); err != nil {
		return devices, errors.Wrapf(err, "unmarshal %s json", listType)
	}
	return devices, nil
}

// GetLastOpenResult fetches the last open result in string of the specified CRAS node.
func GetLastOpenResult(ctx context.Context, node *audio.CrasNode) (string, error) {
	devices, err := ListDevices(ctx, node.IsInput)
	if err != nil {
		return "", errors.Wrap(err, "get last_open_result")
	}

	// CrasNode.ID consists of upper 32-bit Device ID and lower 32-bit Node ID.
	// Only the former is needed here.
	devID := node.ID >> 32
	for _, dev := range devices {
		if dev.Index == devID {
			return dev.LastOpenResult, nil
		}
	}
	return "", errors.Errorf("no device is found for ID:%d", devID)
}
