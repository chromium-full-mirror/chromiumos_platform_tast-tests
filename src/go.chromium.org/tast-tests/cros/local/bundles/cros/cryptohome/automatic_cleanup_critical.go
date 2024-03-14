// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/cryptohome/cleanup"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutomaticCleanupCritical,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test critical automatic disk cleanup",
		BugComponent: "b:1264602",
		Contacts: []string{
			"chromeos-disk-cleanup@google.com",
			"vsavu@google.com",     // Test author
			"gwendal@chromium.org", // Lead for ChromeOS Storage
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Fixture:      fixture.FakeDMSEnrolled,
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		SoftwareDeps: []string{"reboot", "chrome", "tpm_clear_allowed", "vpd"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceRunAutomaticCleanupOnLogin{}, pci.VerifiedFunctionalityOS),
		},
		HardwareDeps: hwdep.D(hwdep.MinStorage(16)),
	})
}

func AutomaticCleanupCritical(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)

	for _, param := range []struct {
		name      string                                   // name is the subtest name.
		policy    *policy.DeviceRunAutomaticCleanupOnLogin // policy is the policy we test.
		shouldRun bool                                     // shouldRun indicates whether the cleanup should run.
	}{
		{
			name:      "unset",
			policy:    &policy.DeviceRunAutomaticCleanupOnLogin{Stat: policy.StatusUnset},
			shouldRun: true,
		},
		{
			name:      "false",
			policy:    &policy.DeviceRunAutomaticCleanupOnLogin{Val: false},
			shouldRun: false,
		},
		{
			name:      "true",
			policy:    &policy.DeviceRunAutomaticCleanupOnLogin{Val: true},
			shouldRun: true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Start a Chrome instance that will fetch policies from the FakeDMS.
			cr, err := chrome.New(ctx,
				chrome.NoLogin(),
				chrome.DMSPolicy(fdms.URL),
				chrome.KeepEnrollment(),
				chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
			)
			if err != nil {
				s.Fatal("Chrome start failed: ", err)
			}
			defer cr.Close(ctx)

			tconn, err := cr.SigninProfileTestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to open tconn: ", err)
			}

			// Update policies.
			pb := policy.NewBlob()
			pb.AddPolicies([]policy.Policy{param.policy})
			if err := fdms.WritePolicyBlob(pb); err != nil {
				s.Fatal("Failed to write policies: ", err)
			}

			if err := policyutil.Refresh(ctx, tconn); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			if err := policyutil.Verify(ctx, tconn, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to verify policies: ", err)
			}

			const (
				homedirSize = 100 * cleanup.MiB // 100 Mib, used for testing

				temporaryUser = "tmp-user"
				user1         = "critical-cleanup-user1"
				user2         = "critical-cleanup-user2"
				password      = "1234"
			)

			// Start cryptohomed and wait for it to be available.
			if err := cryptohome.CheckService(ctx); err != nil {
				s.Fatal("Failed to start cryptohomed: ", err)
			}

			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			freeSpace, err := disk.FreeSpace(cleanup.UserHome)
			if err != nil {
				s.Fatal("Failed to get the amount of free space")
			}

			cleanupThreshold := freeSpace + 50*1024*1024
			cleanupThresholdsArgs := fmt.Sprintf("--cleanup_threshold=%d --aggressive_cleanup_threshold=%d --critical_cleanup_threshold=%d --target_free_space=%d", cleanupThreshold, cleanupThreshold, cleanupThreshold, cleanupThreshold)

			// Restart with higher thresholds. Restart also needed to make sure policies are applied.
			if err := upstart.RestartJob(ctx, "cryptohomed", upstart.WithArg("VMODULE_ARG", "*=1"), upstart.WithArg("CRYPTOHOMED_ARGS", cleanupThresholdsArgs)); err != nil {
				s.Fatal("Failed to restart cryptohome: ", err)
			}

			if err := cleanup.RunOnExistingUsers(ctx); err != nil {
				s.Fatal("Failed to perform initial cleanup: ", err)
			}

			// Create users with contents to fill up disk space.
			_, err = cleanup.CreateFilledUserHomedir(ctx, user1, password, "MyFiles/Downloads", homedirSize)
			if err != nil {
				s.Fatal("Failed to create user with content: ", err)
			}
			defer cryptohome.RemoveVault(ctx, user1)

			fillFile2, err := cleanup.CreateFilledUserHomedir(ctx, user2, password, "MyFiles/Downloads", homedirSize)
			if err != nil {
				s.Fatal("Failed to create user with content: ", err)
			}
			defer cryptohome.RemoveVault(ctx, user2)
			// Unmount all users before removal.
			defer cryptohome.UnmountAll(ctx)

			// Make sure to unmount the second user.
			if err := cryptohome.UnmountVault(ctx, user2); err != nil {
				s.Fatal("Failed to unmount user vault: ", err)
			}

			// Remount the second user. Since space is very low, other user should be cleaned up.
			if err := cryptohome.MountVault(ctx, user2, password); err != nil {
				s.Fatal("Failed to remount user vault: ", err)
			}

			if err := cryptohome.WaitForUserMount(ctx, user2); err != nil {
				s.Fatal("Failed to remount user vault: ", err)
			}

			// Check if the users are correctly present.
			if _, err := os.Stat(fillFile2); err != nil {
				s.Error("Data for user2 lost: ", err)
			}

			shouldExist := !param.shouldRun
			if exists, err := cleanup.UserHomeExists(ctx, user1); err != nil {
				s.Fatal("Failed to dermine if user vault exists: ", err)
			} else if exists != shouldExist {
				s.Errorf("User vault unexpectedly exists: got %t; want %t", exists, shouldExist)
			}
		})
	}
}
