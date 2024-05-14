// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/slimrootfsutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
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
		Attr:         []string{"group:mainline", "group:sw_gates_virt", "sw_gates_virt_enabled"},
		Data:         slimrootfsutils.GetDataBasedOnBoards(vm.TargetArch()),
		Fixture:      "chromeLoggedIn",
	})
}

// StopAllSlimRootfsVMs is used as one of the standard SW gates (go/pe-sw-gates).
// Please ask to crosvm-core@ if you want to modify or delete this test.
func StopAllSlimRootfsVMs(ctx context.Context, s *testing.State) {
	user := s.FixtValue().(chrome.HasChrome).Chrome().NormalizedUser()
	concierge, err := vm.NewConcierge(ctx, user)
	if err != nil {
		s.Error("Failed to get concierge instance: ", err)
	}

	kernelAndRootfsFiles := slimrootfsutils.GetDataBasedOnBoards(vm.TargetArch())
	kernel := s.DataPath(kernelAndRootfsFiles[0])
	rootfs := s.DataPath(kernelAndRootfsFiles[1])

	// Shorten timeout for the clean up
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	defer vm.TrySaveAllVMLogs(cleanupCtx, user, s.OutDir())

	vmOne := vm.NewGenericVM(concierge, false, slimrootfsutils.StatefulDiskSizeBytes, kernel, rootfs, slimrootfsutils.DefaultVMName+"one")
	if err := vmOne.Start(ctx); err != nil {
		s.Fatal("Failed to start the first VM: ", err)
	}

	vmTwo := vm.NewGenericVM(concierge, false, slimrootfsutils.StatefulDiskSizeBytes, kernel, rootfs, slimrootfsutils.DefaultVMName+"two")
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
