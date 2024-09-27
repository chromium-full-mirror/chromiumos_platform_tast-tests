// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IsModemFirmwareKnown,
		Desc:         "Verifies that the modem FW version is known",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active"},
	})
}

// IsModemFirmwareKnown Test
func IsModemFirmwareKnown(ctx context.Context, s *testing.State) {
	if err := cellular.IsModemFirmwareKnown(ctx); err != nil {
		s.Fatal("Modem FW is not known: ", err)
	}
}
