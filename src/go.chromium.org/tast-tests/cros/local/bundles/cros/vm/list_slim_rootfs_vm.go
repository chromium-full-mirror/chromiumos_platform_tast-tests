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
		Func:         ListSlimRootfsVM,
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

// ListSlimRootfsVM is used as one of the standard SW gates (go/pe-sw-gates).
// Please ask to crosvm-core@ if you want to modify or delete this test.
func ListSlimRootfsVM(ctx context.Context, s *testing.State) {
	user := s.FixtValue().(chrome.HasChrome).Chrome().NormalizedUser()
	concierge, err := vm.NewConcierge(ctx, user)
	if err != nil {
		s.Error("Failed to get concierge instance: ", err)
	}

	kernelAndRootfsFiles := slimrootfsutils.GetDataBasedOnBoards(vm.TargetArch())
	vmNames := []string{slimrootfsutils.DefaultVMName + "one", slimrootfsutils.DefaultVMName + "two"}
	kernel := s.DataPath(kernelAndRootfsFiles[0])
	rootfs := s.DataPath(kernelAndRootfsFiles[1])

	// Shorten timeout for the clean up
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	defer vm.TrySaveAllVMLogs(cleanupCtx, user, s.OutDir())

	var vms []*vm.VM
	for _, vmName := range vmNames {
		vm := vm.NewGenericVM(concierge, false, slimrootfsutils.StatefulDiskSizeBytes, kernel, rootfs, vmName)
		if err := vm.Start(ctx); err != nil {
			s.Fatal("Failed to start the VM: ", err)
		}
		vms = append(vms, vm)
	}

	runningVms, err := concierge.ListVms(ctx)
	if err != nil {
		s.Fatal("Failed to get info about the VM: ", err)
	}

	if runningVms == nil {
		s.Fatal("No running VMs found")
	}

	for _, vmName := range vmNames {
		foundVM := false
		for _, runningVM := range runningVms {
			if vmName == runningVM.Name {
				foundVM = true
				break
			}
		}

		if !foundVM {
			s.Fatalf("Failed to find VM %q in running VMs", vmName)
		}
	}
}
