// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/policy/nebraska"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/lsbrelease"
	"chromiumos/tast/testing"
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
		BugComponent: "b:263361362",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"igorcov@chromium.org", // Test author
		},
		Attr:         []string{"group:mainline", "informational"},
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
		lsb, err := lsbrelease.Load()
		if err != nil {
			s.Fatal("Failed to load lsbrelease: ", err)
		}

		// Generate contents of update_payload.json based on appID from the device.
		appID := lsb[lsbrelease.ReleaseAppID]

		// The data have been generated using documentation from
		// go/remote-management/docs/version-management/development
		payloadJSONContents := nebraska.UpdatePayload{
			Appid:             appID,
			IsDelta:           false,
			MetadataSignature: nil,
			MetadataSize:      74204,
			Sha256Hex:         "/gu3P+FBIioa5ILaBNKysJ/uWQiIId/mNEn9qCT/0/k=",
			Size:              1401355452,
			TargetVersion:     "99999.0.0",
			Version:           2,
		}

		// Start nebraska with the generated JSON contents.
		updateServer, err := nebraska.StartWithMetadata(ctx, &payloadJSONContents)
		if err != nil {
			s.Fatal("Failed to start nebraska: ", err)
		}
		defer updateServer.Stop(ctx)
		if err := updateServer.ConfigureStatefulLSBRelease(); err != nil {
			s.Fatal("Failed to configure lsb-release: ", err)
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

			updateServerLog, err := updateServer.ReadLog(ctx)
			if err != nil {
				s.Fatal("Failed to read nebraska logs: ", err)
			}

			if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "unset_nebraska_log.txt"), updateServerLog, 0644); err != nil {
				s.Error("Failed to dump nebraska logs: ", err)
			}

			return nil
		}, &testing.PollOptions{
			Timeout: 59 * time.Second,
		}); err != nil {
			s.Error("Could not find expected values: ", err)
		}
	})
}
