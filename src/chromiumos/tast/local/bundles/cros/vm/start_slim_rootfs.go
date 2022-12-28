// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/vm"
	"chromiumos/tast/testing"
)

const (
	tatlKernelDataFile    = "tatl_slim_vm_kernel_amd64"
	tatlRootfsDataFile    = "tatl_slim_vm_rootfs_amd64"
	taelKernelDataFile    = "tael_slim_vm_kernel_amd64"
	taelRootfsDataFile    = "tael_slim_vm_rootfs_amd64"
	statefulDiskSizeBytes = 50 * 1024 * 1024
	vmName                = "slimvm"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StartSlimRootfs,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Starts a Linux VM with a slim rootfs",
		Contacts:     []string{"crosvm-core@google.com", "abhishekbh@google.com"},
		BugComponent: "b:256052459",
		SoftwareDeps: []string{"chrome", "vm_host"},
		Attr:         []string{"group:mainline", "informational"},
		Data:         getDataBasedOnBoards(vm.TargetArch()),
		Fixture:      "chromeLoggedIn",
	})
}

func getDataBasedOnBoards(board string) []string {
	if board == "amd64" {
		return []string{tatlKernelDataFile, tatlRootfsDataFile}
	}
	return []string{taelKernelDataFile, taelRootfsDataFile}
}

func StartSlimRootfs(ctx context.Context, s *testing.State) {
	concierge, err := vm.NewConcierge(ctx, s.FixtValue().(*chrome.Chrome).NormalizedUser())
	if err != nil {
		s.Error("Failed to get concierge instance: ", err)
	}

	kernelAndRootfsFiles := getDataBasedOnBoards(vm.TargetArch())
	kernel := s.DataPath(kernelAndRootfsFiles[0])
	rootfs := s.DataPath(kernelAndRootfsFiles[1])
	s.Log("Kernel path: ", kernel)
	s.Log("Rootfs path: ", rootfs)

	v := vm.NewGenericVM(concierge, false, statefulDiskSizeBytes, kernel, rootfs, vmName)
	err = v.Start(ctx)
	if err != nil {
		s.Error("Failed to start a VM: ", err)
	}
}
