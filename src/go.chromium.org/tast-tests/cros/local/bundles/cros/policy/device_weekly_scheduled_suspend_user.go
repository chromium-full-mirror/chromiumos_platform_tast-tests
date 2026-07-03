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
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DeviceWeeklyScheduledSuspendUser,
		Desc: "Checks that device sleeps on schedule with policy DeviceWeeklyScheduledSuspend in user session",
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
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      suspend.SleepPollTimeout*2 + chrome.LoginTimeout + chrome.ResetTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceWeeklyScheduledSuspend{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func DeviceWeeklyScheduledSuspendUser(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, chrome.ResetTimeout)
	defer cancel()

	cr, err := chrome.New(
		ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		// Enable device event log in order to observe powerd events.
		chrome.ExtraArgs("--vmodule=device_event_log*=1"),
		chrome.EnableFeatures("DeviceWeeklyScheduledSuspendMgs"),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in user session: ", err)
	}
	defer cr.Close(cleanupCtx)

	if err := suspend.VerifyScheduledSuspend(ctx, fdms, cr, policy.NewBlob(), false /* shouldSuspend */, false /* isLoginScreen */); err != nil {
		s.Fatal("Failed to verify scheduled suspend: ", err)
	}
}
