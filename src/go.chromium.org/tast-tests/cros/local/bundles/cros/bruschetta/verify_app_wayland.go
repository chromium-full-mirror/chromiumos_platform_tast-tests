// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/guestos/verifyapp"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyAppWayland,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs a Wayland bruschetta application from the terminal and verifies that it renders",
		Contacts:     []string{"clumptini+oncall@google.com", "jamesye@google.com"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Attr:         []string{"group:mainline", "group:bruschetta_cq"},
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixture,
	})
}

func VerifyAppWayland(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(bruschetta.FixtureData).Chrome
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM

	if err := verifyapp.RunTest(ctx, s.OutDir(), cr, bru, guestos.WaylandDemoConfig()); err != nil {
		s.Fatal("Failed to run test: ", err)
	}
}
