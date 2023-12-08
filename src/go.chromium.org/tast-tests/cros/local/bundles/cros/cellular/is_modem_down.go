// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IsModemDown,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that a modem label is correct and that the modem is not available",
		Contacts:     []string{"chromeos-cellular-team@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_run_isolated"},
		Timeout:      3 * time.Minute,
	})
}

// IsModemDown verifies that the cellular modem state label is correct and that the modem is not available.
func IsModemDown(ctx context.Context, s *testing.State) {
	if _, err := modemmanager.NewModem(ctx); err == nil {
		s.Fatal("Modem detected succesfully, expected failure")
	}
}
