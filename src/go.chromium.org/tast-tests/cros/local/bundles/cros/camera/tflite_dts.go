// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TFLiteDTS,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the TFLite stable delegate test suite",
		Contacts:     []string{"chromeos-camera-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"ml_service"},
		Vars:         []string{"settings", "accel_config"},
	})
}

// This should be sync with StableDelegateLoaderSettings in
// https://github.com/tensorflow/tensorflow/blob/-/tensorflow/lite/acceleration/configuration/configuration.proto
type stableDelegateLoaderSettings struct {
	DelegatePath string `json:"delegate_path"`
	DelegateName string `json:"delegate_name"`
}

type stableDelegateSettings struct {
	StableDelegateLoaderSettings stableDelegateLoaderSettings `json:"stable_delegate_loader_settings"`
}

func localLibraryDirectory() string {
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		return "/usr/local/lib64/"
	}
	return "/usr/local/lib/"
}

// TODO(shik): Run DTS with vendor provided stable delegates automatically.
var sampleSettings = stableDelegateSettings{
	StableDelegateLoaderSettings: stableDelegateLoaderSettings{
		DelegatePath: localLibraryDirectory() + "libtensorflowlite_cros_sample_delegate.so",
		DelegateName: "cros_sample_delegate",
	},
}

// The config format comment is adapted from
// https://github.com/tensorflow/tensorflow/blob/d55ac19510f7f80b0bb92aa1c63c531ec8e64605/tensorflow/lite/delegates/utils/experimental/stable_delegate/kernel_test_main.cc
const sampleAccelConfig = `
## Config Format
# Every Test can be allowlisted or denylisted using a regexp on its test_id.
# To denylist an element simply add - before the test_id regex.
# Acceleration validation will only be checked on allowlisted tests.

## test_id
#
# The test_id is "test_suite_name/test_name", this differs from the
# name used by the build because of the / separator instead of .

## Rules Evaluation
#
# Rules are checked in order, the first matching wins.
# Put more specific rules first and generic default ones below.

# The sample stable delegate supports static-sized addition and subtraction.
FloatAddOpModel/NoActivation
FloatAddOpModel/VariousInputShapes
FloatSubOpModel/NoActivation
FloatSubOpModel/VariousInputShapes
`

// Disable MultiDimBroadcast related tests for now since it takes ~3 minutes,
// and the operation is not supported by sample stable delegate.
// TODO(shik): Consider enable it for vendor delegate if it's supported.
const gtestFilter = "-*MultiDimBroadcastSubshard*"

// TFLiteDTS runs the Tensorflow Lite Stable Delegate Test Suite (DTS).
func TFLiteDTS(ctx context.Context, s *testing.State) {
	settingsPath := filepath.Join(s.OutDir(), "settings.json")
	accelConfigPath := filepath.Join(s.OutDir(), "accel.conf")
	gtestLogPath := filepath.Join(s.OutDir(), "gtest.log")

	settingsVar, ok := s.Var("settings")
	if ok {
		s.Log("Use provided settings file at ", settingsVar)
		if err := fsutil.CopyFile(settingsVar, settingsPath); err != nil {
			s.Fatal("Failed to copy settings file: ", err)
		}
	} else {
		jsonSettings, err := json.Marshal(sampleSettings)
		if err != nil {
			s.Fatal("Failed to marshal stable delegate settings")
		}
		if err := os.WriteFile(settingsPath, jsonSettings, 0644); err != nil {
			s.Fatal("Failed to write settings.json: ", err)
		}
	}

	accelConfigVar, ok := s.Var("accel_config")
	if ok {
		s.Log("Use provided accel_config file at ", accelConfigVar)
		if err := fsutil.CopyFile(accelConfigVar, accelConfigPath); err != nil {
			s.Fatal("Failed to copy accel_config file: ", err)
		}
	} else {
		if err := os.WriteFile(accelConfigPath, []byte(sampleAccelConfig), 0644); err != nil {
			s.Fatal("Failed to write accel.conf: ", err)
		}
	}

	if report, err := gtest.New(
		"/usr/local/bin/stable_delegate_test_suite",
		gtest.Logfile(gtestLogPath),
		gtest.Filter(gtestFilter),
		gtest.ExtraArgs(
			"--stable_delegate_settings_file="+settingsPath,
			"--acceleration_test_config_path="+accelConfigPath,
		),
	).Run(ctx); err != nil {
		if report != nil {
			failed := report.FailedTestNames()
			for _, name := range failed {
				s.Log("Failed test: ", name)
			}

			numFailed := len(failed)
			if numFailed == 1 {
				s.Errorf("%s failed", failed[0])
			} else if numFailed > 1 {
				s.Errorf("%s and %d more tests failed", failed[0], numFailed-1)
			}
		}
		s.Error("Failed to pass DTS: ", err)
	}
}
