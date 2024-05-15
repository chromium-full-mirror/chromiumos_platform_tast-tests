// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeOSReleaseChannelDelegated,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check of ChromeOsReleaseChannelDelegated policy by checking the settings page and update_engine",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test author
		},
		BugComponent: "b:1031231", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Version Control
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.FakeDMSUpdateEngineEnrolled,
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ChromeOsReleaseChannelDelegated{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.ChromeOsReleaseChannel{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.ChromeOsReleaseChannelDelegated{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// As an IT leader/manager, I want to specify the release
				// channel that my organization's devices are receiving updates
				// from based on my preferences balancing stability and access
				// to new features.
				// COM_FOUND_CUJ11_TASK4_WF1
				Value: "screenplay-37893d3d-3f48-4541-896c-f70c3cf52271",
			},
		},
	})
}

var affiliationID []string = []string{"affiliation_id"}

func updateDeviceAffiliationIDs(ctx context.Context, fdms *fakedms.FakeDMS, signinProfileTestExtensionManifestKey string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// ChromeOsReleaseChannelDelegated only works for affiliated users.
	pb := policy.NewBlob()
	pb.DeviceAffiliationIds = affiliationID
	pb.UserAffiliationIds = affiliationID

	// Update affiliation.
	if err := fdms.WritePolicyBlob(pb); err != nil {
		return errors.Wrap(err, "failed to write policy blob with device affiliation IDs")
	}

	// Restart Chrome to read the updated device affiliation IDs.
	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.LoadSigninProfileExtension(signinProfileTestExtensionManifestKey),
		chrome.KeepEnrollment())
	if err != nil {
		return errors.Wrap(err, "failed to start initial Chrome")
	}
	defer func() {
		if err := cr.Close(cleanupCtx); err != nil {
			retErr = errors.Wrap(err, "failed to close initial Chrome connection")
		}
	}()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a test connection to the login screen")
	}

	if err := policyutil.Refresh(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to refresh device affiliation IDs")
	}

	return nil
}

// updateEngineTargetChannel reads target channel from update_engine.
func updateEngineTargetChannel(ctx context.Context) (string, error) {
	output, err := testexec.CommandContext(ctx, "update_engine_client", "--show_channel").CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}

	targetChannelRegex := regexp.MustCompile(`Target Channel.*: ([a-z]+)-channel`)
	result := targetChannelRegex.FindStringSubmatch(string(output))

	if len(result) == 0 {
		return "", errors.Errorf("could not extract target channel name from %q", string(output))
	}

	return result[1], nil
}

// waitForTargetChannel waits for update_engine to report the requested target channel.
func waitForTargetChannel(ctx context.Context, channel string) error {
	testing.ContextLogf(ctx, "Waiting for channel to be set to %q", channel)
	return testing.Poll(ctx, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if targetChannel, err := updateEngineTargetChannel(ctx); err != nil {
			return err
		} else if targetChannel != channel {
			return errors.Errorf("unexpected channel: got %q, want %q", targetChannel, channel)
		}

		return nil
	}, &testing.PollOptions{
		Timeout: 30 * time.Second,
	})
}

// applyPoliciesAndCheckSettingsPage sets policies and restarts Chrome to apply
// them, then checks the settings page for the restriction on channel
// selection.
func applyPoliciesAndCheckSettingsPage(ctx context.Context, s *testing.State, fdms *fakedms.FakeDMS, policies []policy.Policy, expectedRestriction restriction.Restriction, dumpPrefix string) (retChrome *chrome.Chrome, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// ChromeOsReleaseChannelDelegated only works for affiliated users.
	pb := policy.NewBlob()
	pb.DeviceAffiliationIds = affiliationID
	pb.UserAffiliationIds = affiliationID
	pb.AddPolicies(policies)

	// Update affiliation and policies.
	if err := fdms.WritePolicyBlob(pb); err != nil {
		return nil, err
	}

	// Restart to apply the affiliation and policies.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		return nil, err
	}

	defer func(ctx context.Context) {
		if cr == nil {
			testing.ContextLog(ctx, "cr is nil, skipping screenshot")
			return
		}

		faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), func() bool {
			return retErr != nil
		}, cr, dumpPrefix+"_ui_tree_settings_page")
	}(cleanupCtx)

	defer func(ctx context.Context) {
		logTarget := filepath.Join(s.OutDir(), dumpPrefix+"_update_engine_log.txt")
		if err := fsutil.CopyFile("/var/log/update_engine.log", logTarget); err != nil {
			testing.ContextLogf(ctx, "Failed to copy update engine log to %q", logTarget)
		}
	}(cleanupCtx)

	// Restart update-engine to force reload policies.
	if err := upstart.RestartJob(ctx, "update-engine"); err != nil {
		return nil, err
	}

	channelButton := nodewith.Name("Change channel").Role(role.Button)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Open the channel settings page and check if the channel selection is disabled.
		if err := policyutil.OSSettingsPage(ctx, cr, "help/details").
			SelectNode(ctx, channelButton).Restriction(expectedRestriction).
			Verify(); err != nil {

			return err
		}

		return nil
	}, &testing.PollOptions{
		Timeout: 10 * time.Second,
	}); err != nil {
		return nil, err
	}

	// Updating the target channel by policy only happens when an update is triggered.
	// The result of the update check is not relevant, the logic is executed while triggering.
	// --status will return an error, but there's no other way to not block here. Ignoring the error
	testexec.CommandContext(ctx, "update_engine_client", "--update", "--status").Run()

	return cr, nil
}

func ChromeOSReleaseChannelDelegated(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	signinProfileTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	if err := updateDeviceAffiliationIDs(ctx, fdms, signinProfileTestExtensionManifestKey); err != nil {
		s.Fatal("Failed to set up device affiliation IDs: ", err)
	}

	s.Run(ctx, "delegated_first", func(ctx context.Context, s *testing.State) {
		// Set the channel user choice to stable.
		if err := testexec.CommandContext(ctx, "update_engine_client", "--channel=stable-channel", "--nopowerwash").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to set the channel to stable: ", err)
		}

		cr, err := applyPoliciesAndCheckSettingsPage(ctx, s, fdms, []policy.Policy{
			&policy.ChromeOsReleaseChannelDelegated{Val: true},
			&policy.ChromeOsReleaseChannel{Val: "beta-channel"},
		}, restriction.None, "delegated_first")
		if err != nil {
			s.Fatal("Failed to check settings page: ", err)
		}
		defer cr.Close(ctx)

		// The target channel should be stable as initially configured (policy is ignored).
		if err := waitForTargetChannel(ctx, "stable"); err != nil {
			s.Error("Channel not set: ", err)
		}

		// Move to dev channel using the UI.

		// Connect to Test API to use it with the UI library.
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create Test API connection: ", err)
		}

		channelButton := nodewith.Name("Change channel").Role(role.Button)
		channelAccept := channelButton.Ancestor(nodewith.Name("Change channel").Role(role.Dialog))

		ui := uiauto.New(tconn)
		if err := uiauto.Combine("select dev channel",
			ui.LeftClick(channelButton),
			ui.LeftClick(nodewith.Name("Developer - unstable").First()),
			ui.LeftClick(channelAccept),
		)(ctx); err != nil {
			s.Fatal("Failed to select dev channel: ", err)
		}

		// Check that the channel has actually been set.
		if err := waitForTargetChannel(ctx, "dev"); err != nil {
			s.Error("Channel not set: ", err)
		}
	})

	s.Run(ctx, "policy_override", func(ctx context.Context, s *testing.State) {
		cr, err := applyPoliciesAndCheckSettingsPage(ctx, s, fdms, []policy.Policy{
			&policy.ChromeOsReleaseChannelDelegated{Val: false},
			&policy.ChromeOsReleaseChannel{Val: "beta-channel"},
		}, restriction.Disabled, "policy_override")
		if err != nil {
			s.Fatal("Failed to check settings page: ", err)
		}
		defer cr.Close(ctx)

		if err := waitForTargetChannel(ctx, "beta"); err != nil {
			s.Error("Channel not set: ", err)
		}
	})

	s.Run(ctx, "delegated_again", func(ctx context.Context, s *testing.State) {
		cr, err := applyPoliciesAndCheckSettingsPage(ctx, s, fdms, []policy.Policy{
			&policy.ChromeOsReleaseChannelDelegated{Val: true},
			&policy.ChromeOsReleaseChannel{Val: "stable-channel"},
		}, restriction.None, "delegated_again")
		if err != nil {
			s.Fatal("Failed to check settings page: ", err)
		}
		defer cr.Close(ctx)

		// Channel is coming from the previously set policy, not the new one.
		if err := waitForTargetChannel(ctx, "beta"); err != nil {
			s.Error("Channel not set: ", err)
		}
	})
}
