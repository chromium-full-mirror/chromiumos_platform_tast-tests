// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package timberslide

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RunningForEC,
		Desc: "Checks that the timberslide job instance for the EC is running",
		Contacts: []string{
			"chromeos-firmware@google.com",
			"hesling@chromium.org",
		},
		// ChromeOS > Platform > System > Firmware > EC
		BugComponent: "b:167114",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:cq-medium",
		},
	})
}

// RunningForEC checks if the timberslide job instance, responsible for
// collecting the logs from the EC, is running.
func RunningForEC(ctx context.Context, s *testing.State) {
	if err := upstart.CheckJob(
		ctx,
		"timberslide",
		upstart.WithArg("LOG_PATH", "/sys/kernel/debug/cros_ec/console_log"),
	); err != nil {
		s.Fatal("Failed to check timberslide(EC) is running: ", err)
	}
}
