// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tflite

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AndroidHardwareBuffer,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the test of AHardwareBuffer shim used in TFLite",
		Contacts:     []string{"cros-odml-foundations-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:1445284", // ChromeOS > Platform > Technologies > Machine Learning > On-Device ML
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"ml_service"},
	})
}

// AndroidHardwareBuffer runs android_hardware_buffer_test, which exercises the
// AHardwareBuffer shim in ChromeOS platform/tflite repo.
func AndroidHardwareBuffer(ctx context.Context, s *testing.State) {
	gtestLogPath := filepath.Join(s.OutDir(), "gtest.log")

	if report, err := gtest.New(
		"android_hardware_buffer_test",
		gtest.Logfile(gtestLogPath),
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
		s.Error("Failed to pass AHardwareBuffer test: ", err)
	}
}
