// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CommandVim,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test vim in Terminal window",
		Contacts:     []string{"jamesye@google.com", "clumptini+oncall@google.com"},
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaMinDiskSize,
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixture,
	})
}

func CommandVim(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	keyboard := s.FixtValue().(bruschetta.FixtureData).KB
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM

	// Open Terminal app.
	terminal, err := terminalapp.LaunchBruschetta(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Terminal app: ", err)
	}

	if err := guestos.CommandVim(ctx, terminal, keyboard, bru); err != nil {
		s.Fatal("Vim test failed: ", err)
	}
}
