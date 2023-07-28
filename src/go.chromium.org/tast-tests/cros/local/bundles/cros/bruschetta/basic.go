// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bruschetta"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Basic,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests startup for the bruschetta VM",
		Contacts:     []string{"clumptini+oncall@google.com", "sidereal@google.com", "jamesye@google.com"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Attr:         []string{"group:mainline"},
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixture,
	})
}

func Basic(ctx context.Context, s *testing.State) {
	vm := s.FixtValue().(bruschetta.FixtureData).VM

	// Run a simple command in the VM using vsh to check that it works.
	_, err := vm.Command(ctx, "pwd").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run command: ", err)
	}
}
