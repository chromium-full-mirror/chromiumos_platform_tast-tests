// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ARCVMInstall,
		Desc: "Tests that the ARCVM DLC is automatically installed and activated",
		Contacts: []string{
			"chromeos-flex-eng+oncall@google.com",
			"josephsussman@google.com", // Test author
		},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		Attr:         []string{"group:mainline"},
		Fixture:      "flexARCVM",
		HardwareDeps: hwdep.D(hwdep.SkipDMIProductName("NUC11TNKv5"), hwdep.Model("reven")),
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Timeout:      30 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceFlexArcPreloadEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func ARCVMInstall(ctx context.Context, s *testing.State) {
	s.Log("Waiting for Android system image to appear")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat("/opt/google/vms/android/system.raw.img"); err == nil {
			return nil
		} else if os.IsNotExist(err) {
			return errors.New("system image is not present")
		} else {
			return errors.Errorf("failed to check system image: %q", err)
		}
	}, &testing.PollOptions{Timeout: 20 * time.Minute}); err != nil {
		s.Fatal("Android system image did not appear within the timeout: ", err)
	}

	s.Log("Cleaning up")
	err := testexec.CommandContext(ctx, "dlcservice_util", "--uninstall", "--id=android-vm-dlc").Run(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to uninstall the android-vm-dlc: ", err)
	}
	err = testexec.CommandContext(ctx, "umount", "/opt/google/vms/android").Run(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to unmount Android bind mount: ", err)
	}

}
