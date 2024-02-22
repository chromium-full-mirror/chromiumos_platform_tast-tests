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
		Func:         SecureBoot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks UEFI Secure Boot is enabled for bruschetta VM",
		Contacts:     []string{"clumptini+oncall@google.com"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Attr:         []string{"group:mainline", "group:bruschetta_cq"},
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixture,
	})
}

func SecureBoot(ctx context.Context, s *testing.State) {
	vm := s.FixtValue().(bruschetta.FixtureData).VM

	out, err := vm.Command(ctx, "mokutil", "--sb-state").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run command: ", err)
	}

	if string(out) != "SecureBoot enabled\n" {
		s.Fatalf("Secure Boot not enabled: %q", out)
	}
}
