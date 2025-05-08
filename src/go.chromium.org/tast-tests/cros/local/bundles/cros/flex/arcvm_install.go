// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/flex"
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
		Fixture:      flex.ARCVMEnrolled,
		HardwareDeps: hwdep.D(hwdep.SkipDMIProductName("NUC11TNKv5"), hwdep.Model("reven")),
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Timeout:      30 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceFlexArcPreloadEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func ARCVMInstall(ctx context.Context, s *testing.State) {
	// The "test code" for this test was moved to the flex.ARCVMEnrolled fixture
	// by crrev/c/6527449, so other tests could enable/disable ARCVM on Flex.
	//
	// We still want to be able to track whether the enablement process works
	// independent of other tests, so please do not delete this.
}
