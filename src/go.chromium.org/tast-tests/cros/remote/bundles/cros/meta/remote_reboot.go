// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteReboot,
		Desc:         "Reboot a DUT to the tast streaming logs correctly",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		// Timeout is set to 8 minutes: 3 for reboot and 5 for sleep.
		Timeout: time.Minute * 8,
		Attr:    []string{"group:hw_agnostic"},
	})
}

func RemoteReboot(ctx context.Context, s *testing.State) {
	d := s.DUT()
	s.Log("Rebooting DUT")
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}
	s.Log("DUT is up now")
	// This sleep is for testing log streaming.
	testing.Sleep(ctx, time.Minute*5)
}
