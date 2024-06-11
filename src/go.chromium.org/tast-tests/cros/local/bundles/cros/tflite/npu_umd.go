// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tflite

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NPUUMD,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the test of Intel NPU UMD (User Mode Driver)",
		Contacts:     []string{"cros-odml-foundations-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:1140119", // ChromeOS > Platform > Technologies > Machine Learning > ML Accelerators > Intel
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"tflite_intel_openvino"},

		// TODO(b/345085042): The feature is supported by both software and
		// hardware, but the AP firmware (Coreboot) currently pinned on these
		// models is outdated. We will perform firmware qualification and
		// incrementally uprev the firmware.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("rex", "screebo", "karis", "ovis", "deku")),
	})
}

// NPUUMD runs unit tests for Intel NPU UMD.
// See https://github.com/intel/linux-npu-driver for more details.
func NPUUMD(ctx context.Context, s *testing.State) {
	gtestLogPath := filepath.Join(s.OutDir(), "gtest.log")

	// NPU was renamed from VPU, and the test binary is still using the old name.
	if report, err := gtest.New(
		"vpu-umd-test",
		gtest.Logfile(gtestLogPath),
		gtest.Filter("-Umd.ConfigurationCheck"),
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
		s.Error("Failed to pass NPU UMD test: ", err)
	}
}
