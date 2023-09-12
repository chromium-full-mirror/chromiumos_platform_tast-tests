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
		Func:         StopAllSlimRootfsVMs,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Starts multiple Linux VMs and then stops all of them",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "uekawa@google.com"},
		BugComponent: "b:1248538", // ChromeOS > Platform > Virtualization > Device and Guests
		SoftwareDeps: []string{"chrome", "vm_host"},
		Attr:         []string{"group:mainline", "informational"},
		Data:         slimrootfsutils.GetDataBasedOnBoards(vm.TargetArch()),
		Fixture:      "chromeLoggedIn",
	})
}

func StopAllSlimRootfsVMs(ctx context.Context, s *testing.State) {
	concierge, err := vm.NewConcierge(ctx, s.FixtValue().(*chrome.Chrome).NormalizedUser())
	if err != nil {
		s.Error("Failed to get concierge instance: ", err)
	}

	kernelAndRootfsFiles := slimrootfsutils.GetDataBasedOnBoards(vm.TargetArch())
	kernel := s.DataPath(kernelAndRootfsFiles[0])
	rootfs := s.DataPath(kernelAndRootfsFiles[1])

	vmOne := vm.NewGenericVM(concierge, false, slimrootfsutils.StatefulDiskSizeBytes, kernel, rootfs, slimrootfsutils.DefaultVMName+"one")
	if err := vmOne.Start(ctx); err != nil {
		s.Fatal("Failed to start the first VM: ", err)
	}

	vmTwoName := slimrootfsutils.DefaultVMName + "two"
	vmTwo := vm.NewGenericVM(concierge, false, slimrootfsutils.StatefulDiskSizeBytes, kernel, rootfs, vmTwoName)
	if err := vmTwo.Start(ctx); err != nil {
		s.Fatal("Failed to start the second VM: ", err)
	}

	if err := concierge.StopAllVms(ctx); err != nil {
		s.Fatal("Failed to stop all VMs: ", err)
	}

	if err := concierge.GetVMInfo(ctx, vmOne); err == nil {
		s.Fatal("Failed to ensure the first VM was stopped: ", err)
	}

	if err := concierge.GetVMInfo(ctx, vmTwo); err == nil {
		s.Fatal("Failed to ensure then second VM was stopped: ", err)
	}
}
