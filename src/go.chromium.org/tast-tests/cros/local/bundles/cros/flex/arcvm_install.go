// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
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
		Attr:         []string{"group:flex_arcvm"},
		Fixture:      fixture.FakeDMSEnrolled,
		HardwareDeps: hwdep.D(hwdep.SkipDMIProductName("NUC11TNKv5"),
			hwdep.Model("reven")),
		SoftwareDeps: []string{"chrome"},
		Timeout:      20 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceFlexArcPreloadEnabled{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

func ARCVMInstall(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)

	// Start a Chrome instance and fetch policies from the login screen.
	cr, err := chrome.New(ctx,
		chrome.DMSPolicy(fdms.URL),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	defer func(ctx context.Context) {
		// Use cr as a reference to close the last started Chrome instance.
		if err := cr.Close(ctx); err != nil {
			s.Error("Failed to close Chrome connection: ", err)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	policies := []policy.Policy{
		&policy.DeviceFlexArcPreloadEnabled{Stat: policy.StatusSet, Val: true},
	}
	// Update policies from the login screen.
	if err := policyutil.ServeAndVerifyOnLoginScreen(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

	// Close the previous Chrome instance.
	if err := cr.Close(ctx); err != nil {
		s.Error("Failed to close Chrome connection: ", err)
	}

	// Restart Chrome to reload policies.
	cr, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	s.Log("Waiting for Android system image to appear")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat("/opt/google/vms/android/system.raw.img"); err == nil {
			return nil
		} else if os.IsNotExist(err) {
			return errors.New("system image is not present")
		} else {
			return errors.Errorf("failed to check system image: %q", err)
		}
	}, &testing.PollOptions{Timeout: 10 * time.Minute}); err != nil {
		s.Fatal("Android system image did not appear within the timeout: ", err)
	}

	s.Log("Cleaning up")
	err = testexec.CommandContext(ctx, "dlcservice_util", "--uninstall", "--id=android-vm-dlc").Run(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to uninstall the android-vm-dlc: ", err)
	}
	err = testexec.CommandContext(ctx, "umount", "/opt/google/vms/android").Run(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to unmount Android bind mount: ", err)
	}

}
