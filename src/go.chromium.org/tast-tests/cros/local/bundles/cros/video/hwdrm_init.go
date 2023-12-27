// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: HWDRMInit,
		Desc: "Verifies that HWDRM can be initialized successfully",
		Contacts: []string{
			"chromeos-gfx-video@google.com",
			"jkardatzke@google.com",
		},
		BugComponent: "b:168352", // ChromeOS > Platform > Graphics > Video
		SoftwareDeps: []string{"protected_content", "hwdrm_stable"},
		Timeout:      5 * time.Minute,
		Attr:         []string{"group:mainline", "group:firmware", "firmware_bios", "firmware_level5"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Fixture:      "gpuWatchHangs",
	})
}

func HWDRMInit(ctx context.Context, s *testing.State) {
	// This is a marker file to indicate the FW version is incompatible with the
	// binary. Clear this if present before we start so we know it was created
	// from this test run and not left over from something else before.
	const fwInvalidMarkerFile = "/var/lib/oemcrypto/wv16_fw_version_invalid"
	if err := os.Remove(fwInvalidMarkerFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Fatal("Failed to remove FW mismatch indicator: ", err)
	}

	cmd := testexec.CommandContext(ctx, "oemcrypto_hw_init_test")
	testErr := cmd.Run(testexec.DumpLogOnError)
	// Check if the marker file is there to indicate FW version incompatibility
	// on Intel and then invoke the WV14 variant instead.
	if _, statErr := os.Stat(fwInvalidMarkerFile); statErr == nil {
		s.Log("Found indicator for FW mismatch, running WV14 variant")
		if err := os.Remove(fwInvalidMarkerFile); err != nil {
			s.Fatal("Failed to remove FW mismatch indicator: ", err)
		}
		cmd = testexec.CommandContext(ctx, "oemcrypto_hw_init_test-wv14")
		testErr = cmd.Run(testexec.DumpLogOnError)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		s.Fatal("Failed to check FW mismatch indicator: ", statErr)
	}

	if testErr != nil {
		s.Error("HWDRMInit failed: ", testErr)
	}
}
