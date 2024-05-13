// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/slimrootfsutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SyncTimeSlimRootfsVM,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Syncs time of a Linux VM with a slim rootfs",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "uekawa@google.com"},
		BugComponent: "b:1248538", // ChromeOS > Platform > Virtualization > Device and Guests
		SoftwareDeps: []string{"chrome", "vm_host"},
		Attr:         []string{"group:mainline"},
		Data:         slimrootfsutils.GetDataBasedOnBoards(vm.TargetArch()),
		Fixture:      "chromeLoggedIn",
	})
}

// SyncTimeSlimRootfsVM is used as one of the standard SW gates (go/pe-sw-gates).
// Please ask to crosvm-core@ if you want to modify or delete this test.
func SyncTimeSlimRootfsVM(ctx context.Context, s *testing.State) {
	concierge, err := vm.NewConcierge(ctx, s.FixtValue().(chrome.HasChrome).Chrome().NormalizedUser())
	if err != nil {
		s.Error("Failed to get concierge instance: ", err)
	}

	kernelAndRootfsFiles := slimrootfsutils.GetDataBasedOnBoards(vm.TargetArch())
	kernel := s.DataPath(kernelAndRootfsFiles[0])
	rootfs := s.DataPath(kernelAndRootfsFiles[1])

	vm := vm.NewGenericVM(concierge, false, slimrootfsutils.StatefulDiskSizeBytes, kernel, rootfs, slimrootfsutils.DefaultVMName)
	if err := vm.Start(ctx); err != nil {
		s.Fatal("Failed to start the VM: ", err)
	}

	if err := concierge.GetVMInfo(ctx, vm); err != nil {
		s.Fatal("Failed to get info about the VM: ", err)
	}

	if err := concierge.SyncTimes(ctx); err != nil {
		s.Fatal("Failed to sync time for the VM: ", err)
	}
}
