// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type autoUpdateRestrictionsTestParam struct {
	// policyValues are the policies that need to be set.
	policyValues []policy.Policy
	// policyShouldBlockUpdate is the expected result whether provided policyValues should result
	// in the update being blocked.
	policyShouldBlockUpdate bool
}

var allWeekInterval = []*policy.DeviceAutoUpdateTimeRestrictionsValue{
	{
		Start: &policy.DeviceAutoUpdateTimeRestrictionsValueStart{DayOfWeek: "Monday", Hours: 0, Minutes: 0},
		End:   &policy.RefDisallowedTimeInterval{DayOfWeek: "Monday", Hours: 0, Minutes: 0},
	},
}

var allWeekInTwoIntervals = []*policy.DeviceAutoUpdateTimeRestrictionsValue{
	{
		Start: &policy.DeviceAutoUpdateTimeRestrictionsValueStart{DayOfWeek: "Monday", Hours: 0, Minutes: 0},
		End:   &policy.RefDisallowedTimeInterval{DayOfWeek: "Thursday", Hours: 12, Minutes: 37},
	},
	{
		Start: &policy.DeviceAutoUpdateTimeRestrictionsValueStart{DayOfWeek: "Thursday", Hours: 12, Minutes: 37},
		End:   &policy.RefDisallowedTimeInterval{DayOfWeek: "Monday", Hours: 0, Minutes: 0},
	},
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceAutoUpdateTimeRestrictions,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that update engine requests updates according to DeviceAutoUpdateTimeRestrictions policy",
		BugComponent: "b:1031231", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Version Control
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"igorcov@chromium.org", // Test author
		},
		Attr:         []string{"group:mainline", "informational", "group:cq-medium"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromeEnrolledLoggedIn,
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "device_auto_update_time_restrictions_always",
				Val: &autoUpdateRestrictionsTestParam{
					policyValues:            []policy.Policy{&policy.DeviceAutoUpdateTimeRestrictions{Val: allWeekInterval}},
					policyShouldBlockUpdate: true,
				},
			},
			{
				Name: "device_auto_update_time_restrictions_always_in_two_sections",
				Val: &autoUpdateRestrictionsTestParam{
					policyValues:            []policy.Policy{&policy.DeviceAutoUpdateTimeRestrictions{Val: allWeekInTwoIntervals}},
					policyShouldBlockUpdate: true,
				},
			},
			{
				Name: "device_auto_update_time_restrictions_never_empty",
				Val: &autoUpdateRestrictionsTestParam{
					policyValues:            []policy.Policy{},
					policyShouldBlockUpdate: false,
				},
			}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceAutoUpdateTimeRestrictions{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// As an IT leader/manager, I want to control the details of how my managed devices update in order to minimize disruption to my business operations.
				// COM_FOUND_CUJ12_TASK2_WF1
				Value: "screenplay-dc821156-53dc-47ba-b9e3-9702fe1c99e4",
			},
		},
	})
}

func DeviceAutoUpdateTimeRestrictions(ctx context.Context, s *testing.State) {
	const (
		localUpdateEngineLog       = "/var/log/update_engine.log"
		blockedByPolicyLogEntry    = "finished OmahaResponseHandlerAction with code ErrorCode::kOmahaUpdateDeferredPerPolicy"
		notBlockedByPolicyLogEntry = "Allowing update to be applied."
		prefsFileIntervalTimeout   = "/var/lib/update_engine/prefs/test-update-check-interval-timeout"
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	param := s.Param().(*autoUpdateRestrictionsTestParam)

	// Restart update-engine after clearing policies.
	defer upstart.RestartJob(ctx, "update-engine")

	s.Run(ctx, "checkRestrictions", func(ctx context.Context, s *testing.State) {
		// Start nebraska with the generated JSON contents.
		updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
		if err != nil {
			s.Fatal("Failed to start nebraska: ", err)
		}
		defer updateServer.Close(ctx)

		if err := updateServer.SetFakedMetadata(ctx); err != nil {
			s.Fatal("Failed to configure Nebraska with faked update metadata: ", err)
		}

		// Set the pref test-update-check-interval-timeout
		// which makes update engine process events on startup even on a test image.
		if err := ioutil.WriteFile(prefsFileIntervalTimeout, []byte("10"), 0666); err != nil {
			s.Fatal("Failed to set the prefs file for interval timeout: ", err)
		}

		// Set the policy and check that the attribute is set.
		if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.policyValues); err != nil {
			s.Fatal("Failed to update policies: ", err)
		}

		if err := upstart.RestartJob(ctx, "update-engine"); err != nil {
			s.Fatal("Failed to trigger update request: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			linkToLog, err := os.Readlink(localUpdateEngineLog)
			if err != nil {
				s.Error("Failed to find update_engine.log on device: ", err)
			}

			realLog, err := ioutil.ReadFile(linkToLog)
			if err != nil {
				s.Error("Failed to find on device: ", linkToLog)
			}

			// Look up for corresponding strings in the update_engine.log depending on whether the test expects the policy to block the update or not.
			if param.policyShouldBlockUpdate {
				if !strings.Contains(string(realLog), blockedByPolicyLogEntry) || strings.Contains(string(realLog), notBlockedByPolicyLogEntry) {
					return errors.New("failed to find proper logs in update_engine.log. Should block: true")
				}
			} else {
				if strings.Contains(string(realLog), blockedByPolicyLogEntry) || !strings.Contains(string(realLog), notBlockedByPolicyLogEntry) {
					return errors.New("failed to find proper logs in update_engine.log. Should block: false")
				}
			}
			return nil
		}, &testing.PollOptions{
			Timeout: 59 * time.Second,
		}); err != nil {
			s.Error("Could not find expected values: ", err)
		}
	})
}
