// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ArcBackupRestoreServiceEnabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test the behavior of ArcBackupRestoreServiceEnabled policy: check the Backup Manager state after setting the policy",
		Contacts: []string{
			"arc-commercial@google.com",
			"mhasank@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		SoftwareDeps: []string{"chrome"},
		// TODO(http://b/172073846): Test showed flakiness in the past. If
		// keeps failing it should be fixed or removed.
		Attr: []string{"group:golden_tier"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"group:hw_agnostic"},
		}},
		Timeout: 8 * time.Minute, // There is a need to start Chrome 4 times.
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcBackupRestoreServiceEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// ArcBackupRestoreServiceEnabled tests the ArcBackupRestoreServiceEnabled policy.
func ArcBackupRestoreServiceEnabled(ctx context.Context, s *testing.State) {
	// Start FakeDMS.
	fdms, err := fakedms.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to start FakeDMS: ", err)
	}
	defer fdms.Stop(ctx)

	for _, param := range []struct {
		name        string
		wantEnabled bool
		value       *policy.ArcBackupRestoreServiceEnabled
	}{
		{
			name:        "disabled",
			wantEnabled: false,
			value:       &policy.ArcBackupRestoreServiceEnabled{Val: 0},
		},
		{
			name:        "user_decides",
			wantEnabled: false,
			value:       &policy.ArcBackupRestoreServiceEnabled{Val: 1},
		},
		{
			name:        "enabled",
			wantEnabled: true,
			value:       &policy.ArcBackupRestoreServiceEnabled{Val: 2},
		},
		{
			name:        "unset",
			wantEnabled: false,
			value:       &policy.ArcBackupRestoreServiceEnabled{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Update the policy blob.
			pb := policy.NewBlob()
			pb.AddPolicies([]policy.Policy{param.value})
			if err := fdms.WritePolicyBlob(pb); err != nil {
				s.Fatal("Failed to write policies to FakeDMS: ", err)
			}

			// Start a Chrome instance that will fetch policies from the FakeDMS.
			// This policy must be updated before starting Chrome.
			cr, err := chrome.New(ctx,
				chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
				chrome.DMSPolicy(fdms.URL),
				chrome.ARCEnabled())
			if err != nil {
				s.Fatal("Chrome login failed: ", err)
			}
			defer cr.Close(ctx)

			a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
			if err != nil {
				s.Fatal("Failed to start ARC: ", err)
			}
			defer a.Close(ctx)

			// Get ARC Backup Manager state.
			var enabled bool
			if output, err := a.Command(ctx, "bmgr", "enabled").Output(); err != nil {
				s.Fatal("Failed to run adb command: ", err)
			} else if strings.Contains(string(output), "enabled") {
				enabled = true
			} else if strings.Contains(string(output), "disabled") {
				enabled = false
			} else {
				s.Fatalf("Invalid adb response: %q", string(output))
			}

			if enabled != param.wantEnabled {
				s.Errorf("Unexpected ARC backup restore service state: got %t; want %t", enabled, param.wantEnabled)
			}
		})
	}
}
