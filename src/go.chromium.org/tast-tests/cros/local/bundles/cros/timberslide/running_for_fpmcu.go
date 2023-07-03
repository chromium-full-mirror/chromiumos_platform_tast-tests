// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package timberslide

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RunningForFPMCU,
		Desc: "Checks that the timberslide job instance for the fingerprint device is running",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"hesling@chromium.org",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:fingerprint-cq",
			"group:cq-medium",
		},
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
	})
}

// RunningForFPMCU checks if the timberslide job instance, responsible for
// collecting the logs from the fingerprint device (FPMCU), is running.
func RunningForFPMCU(ctx context.Context, s *testing.State) {
	if err := upstart.CheckJob(
		ctx,
		"timberslide",
		upstart.WithArg("LOG_PATH", "/sys/kernel/debug/cros_fp/console_log"),
	); err != nil {
		s.Fatal("Failed to check timberslide(FP) is running: ", err)
	}
}
