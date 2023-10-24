// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FlashromWriteProtect,
		Desc: "Checks that flashrom supports writeprotect commands on the device's flash IC",
		Contacts: []string{
			"chromeos-faft@google.com",
			"nartemiev@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"flashrom", "chromeos_firmware"},
		Timeout:      3 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func FlashromWriteProtect(ctx context.Context, s *testing.State) {
	// WP support is all-or-nothing, just run a simple status command
	// to check the flash IC is supported

	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, out, err := flashromConfig.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerHost, "").
		Probe(ctx)

	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()

	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
	}

	wpEnabled, out, err := flashromInstance.SoftwareWriteProtectStatus(ctx)

	if err != nil {
		s.Logf("Software WP status check output: %s", string(out))
		s.Fatalf("Software write-protect status check failed: %s", err)
	}

	wpStatusMessage := "disabled"
	if wpEnabled {
		wpStatusMessage = "enabled"
	}
	testing.ContextLog(ctx, "Software WP status check result: ", wpStatusMessage)
}
