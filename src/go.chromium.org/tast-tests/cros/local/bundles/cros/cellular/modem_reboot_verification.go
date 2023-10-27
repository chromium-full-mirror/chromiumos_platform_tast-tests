// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ModemRebootVerification,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that the modem can be rebooted using GPIO",
		Contacts:       []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:        "cellular",
		SoftwareDeps:   []string{"modemfwd"},
		HardwareDeps:   hwdep.D(hwdep.SkipOnCellularModemType(cellularconst.ModemTypeFM350)),
		Timeout:        3 * time.Minute,
	})
}

// ModemRebootVerification checks if the modem can be rebooted using GPIO.
func ModemRebootVerification(ctx context.Context, s *testing.State) {
	if _, err := cellular.RestartModemWithHelper(ctx); err != nil {
		s.Fatal("Failed to reboot modem using GPIO: ", err)
	}
}
