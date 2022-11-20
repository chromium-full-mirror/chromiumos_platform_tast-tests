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
	kernelDataFile        = "slim_vm_kernel_amd64"
	rootfsDataFile        = "slim_vm_rootfs_amd64"
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
		Data:         []string{kernelDataFile, rootfsDataFile},
		Fixture:      "chromeLoggedIn",
	})
}

func StartSlimRootfs(ctx context.Context, s *testing.State) {
	concierge, err := vm.NewConcierge(ctx, s.FixtValue().(*chrome.Chrome).NormalizedUser())
	if err != nil {
		s.Error("Failed to get concierge instance: ", err)
	}

	kernel := s.DataPath(kernelDataFile)
	rootfs := s.DataPath(rootfsDataFile)
	s.Log("Kernel path: ", kernel)
	s.Log("Rootfs path: ", rootfs)

	v := vm.NewGenericVM(concierge, false, statefulDiskSizeBytes, kernel, rootfs, vmName)
	err = v.Start(ctx)
	if err != nil {
		s.Error("Failed to start a VM: ", err)
	}
}
