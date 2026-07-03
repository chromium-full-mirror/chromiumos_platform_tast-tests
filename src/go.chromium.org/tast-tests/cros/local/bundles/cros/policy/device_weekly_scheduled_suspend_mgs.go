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
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DeviceWeeklyScheduledSuspendMGS,
		Desc: "Checks that device sleeps on schedule with policy DeviceWeeklyScheduledSuspend in MGS session",
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
		Timeout:      suspend.SleepPollTimeout*2 + 2*chrome.LoginTimeout + chrome.ResetTimeout,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceWeeklyScheduledSuspend{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.Served),
			pci.SearchFlag(&policy.DeviceLocalAccountAutoLoginId{}, pci.Served),
			pci.SearchFlag(&policy.DeviceLocalAccountAutoLoginDelay{}, pci.Served),
		},
	})
}

func DeviceWeeklyScheduledSuspendMGS(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, chrome.ResetTimeout)
	defer cancel()

	m, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
		// Enable device event log in order to observe powerd events.
		mgs.ExtraChromeOptions(chrome.ExtraArgs("--vmodule=device_event_log*=1"), chrome.EnableFeatures("DeviceWeeklyScheduledSuspendMgs")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in MGS: ", err)
	}
	defer func(ctx context.Context) {
		if err := m.Close(ctx); err != nil {
			s.Error("Failed to close MGS: ", err)
		}
	}(cleanupCtx)

	pb := policy.NewBlob()
	pb.AddPolicies([]policy.Policy{
		&policy.DeviceLocalAccounts{
			Val: []policy.DeviceLocalAccountInfo{
				{
					AccountID:   &mgs.MgsAccountID,
					AccountType: &mgs.AccountType,
				},
			},
		},
		&policy.DeviceLocalAccountAutoLoginId{
			Val: mgs.MgsAccountID,
		},
		&policy.DeviceLocalAccountAutoLoginDelay{
			Val: 0,
		},
	})

	if err := suspend.VerifyScheduledSuspend(ctx, fdms, cr, pb, true /* shouldSuspend */, false /* isLoginScreen */); err != nil {
		s.Fatal("Failed to verify scheduled suspend: ", err)
	}
}
