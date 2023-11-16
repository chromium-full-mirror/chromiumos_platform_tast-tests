// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CDMOEMCrypto,
		Desc: "Verifies that Widevine CE CDM and OEMCrypto tests run successfully",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"jkardatzke@google.com",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"protected_content"},
		Timeout:      25 * time.Minute,
		Attr:         []string{"group:graphics", "graphics_video", "graphics_nightly"},
		Fixture:      "gpuWatchHangs",
		Params: []testing.Param{{
			Name: "ce_cdm",
			Val:  "widevine_ce_cdm_hw_tests",
		}, {
			Name: "oemcrypto",
			Val:  "oemcrypto_hw_ref_tests",
		}},
	})
}

func createGTest(logdir, testExec string) *gtest.GTest {
	return gtest.New(
		testExec,
		gtest.Logfile(filepath.Join(logdir, testExec+".log")),
		gtest.Filter("-*Huge*:*VerifyUsageTimes*:*OEMCryptoUsageTableDefragTest*"),
	)
}

func CDMOEMCrypto(ctx context.Context, s *testing.State) {
	// This is a marker file to indicate the FW version is incompatible with the
	// binary. Clear this if present before we start so we know it was created
	// from this test run and not left over from something else before.
	const fwInvalidMarkerFile = "/var/lib/oemcrypto/wv16_fw_version_invalid"
	if err := os.Remove(fwInvalidMarkerFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Fatal("Failed to remove FW mismatch indicator: ", err)
	}

	s.Log("Asking powerd to disable screen dimming and keep the display on")
	cmd := testexec.CommandContext(ctx, "set_power_policy", "--dim_wake_lock=1", "--screen_wake_lock=1")
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to change power policy: ", err)
	}

	testExec := s.Param().(string)
	logdir := filepath.Join(s.OutDir(), "gtest")
	s.Log("Running ", testExec)
	_, gTestErr := createGTest(logdir, testExec).Run(ctx)
	// Check if the marker file is there to indicate FW version incompatibility
	// on Intel and then invoke the WV14 variant instead.
	if _, statErr := os.Stat(fwInvalidMarkerFile); statErr == nil {
		s.Log("Found indicator for FW mismatch, running WV14 variant")
		if err := os.Remove(fwInvalidMarkerFile); err != nil {
			s.Fatal("Failed to remove FW mismatch indicator: ", err)
		}
		testExec += "-wv14"
		_, gTestErr = createGTest(logdir, testExec).Run(ctx)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		s.Fatal("Failed to check FW mismatch indicator: ", statErr)
	}
	if gTestErr != nil {
		s.Errorf("%s failed: %v", testExec, gTestErr)
	}
}
