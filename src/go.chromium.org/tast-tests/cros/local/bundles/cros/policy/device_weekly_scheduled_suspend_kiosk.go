// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/suspend"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DeviceWeeklyScheduledSuspendKiosk,
		Desc: "Checks that device sleeps on schedule with policy DeviceWeeklyScheduledSuspend in Kiosk session",
		Contacts: []string{
			"ghostbusters-reviews+TAST@google.com",
			"chromeos-kiosk-eng+TAST@google.com",
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		// This test can be limited to the golden tier, since it's only meant to cover the
		// integration of the policy with powerd, and powerd is well covered by other tests.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Attr: []string{
			"group:golden_tier_secondary",
			"group:release-health",
			"release-health_enterprise",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      suspend.SleepPollTimeout*2 + kioskmode.SetupDuration + 2*kioskmode.LaunchDuration + kioskmode.CleanupDuration,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceWeeklyScheduledSuspend{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func DeviceWeeklyScheduledSuspendKiosk(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		signinTestExtensionManifestKey,
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		// Enable device event log in order to observe powerd events.
		kioskmode.ExtraChromeOptions(chrome.ExtraArgs("--vmodule=device_event_log*=1")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close Kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	if err := suspend.VerifyScheduledSuspend(ctx, fdms, cr, kiosk.PolicyBlob(), true /* shouldSuspend */, false /* isLoginScreen */); err != nil {
		s.Fatal("Failed to verify scheduled suspend: ", err)
	}
}
