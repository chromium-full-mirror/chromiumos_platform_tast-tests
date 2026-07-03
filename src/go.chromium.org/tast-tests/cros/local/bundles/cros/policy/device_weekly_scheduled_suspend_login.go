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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DeviceWeeklyScheduledSuspendLogin,
		Desc: "Checks that device sleeps on schedule with policy DeviceWeeklyScheduledSuspend on Login screen",
		Contacts: []string{
			"ghostbusters-reviews+TAST@google.com",
			"chromeos-kiosk-eng+TAST@google.com",
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Attr: []string{
			"group:golden_tier_secondary",
			"group:release-health",
			"release-health_enterprise",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      suspend.SleepPollTimeout*2 + chrome.LoginTimeout + chrome.ResetTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceWeeklyScheduledSuspend{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func DeviceWeeklyScheduledSuspendLogin(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, chrome.ResetTimeout)
	defer cancel()

	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		// Enable device event log in order to observe powerd events.
		chrome.ExtraArgs("--vmodule=device_event_log*=1"),
		chrome.EnableFeatures("DeviceWeeklyScheduledSuspendMgs"),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome on login screen: ", err)
	}
	defer cr.Close(cleanupCtx)

	if err := suspend.VerifyScheduledSuspend(ctx, fdms, cr, policy.NewBlob(), true /* shouldSuspend */, true /* isLoginScreen */); err != nil {
		s.Fatal("Failed to verify scheduled suspend: ", err)
	}
}
