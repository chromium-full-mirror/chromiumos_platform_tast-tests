// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"os"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/cryptohome/internal"
	"go.chromium.org/tast-tests/cros/local/chrome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FingerprintMigrationManual,
		Desc:         "Checks that cryptohome fingerprint migration functions correctly through manual interaction with FP sensor",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"cryptohome-core@google.com",
			"chromeos-fingerprint@google.com",
			"hcyang@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		SoftwareDeps: []string{"pinweaver", "biometrics_daemon", "chrome"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		// This needs to be run after a fresh reboot because PinWeaver's trust-on-first-use
		// protocol is only allowed before a user is logged-in in a boot cycle. Previous tests might
		// have logged-in a user so we might need to reboot.
		Fixture: "rebootIfPwBlockedFixture",
		Timeout: 5 * time.Minute,
	})
}

func FingerprintMigrationManual(ctx context.Context, s *testing.State) {
	const (
		userName             = "foo@bar.baz"
		userPassword         = "secret"
		passwordLabel        = "gaia"
		indexFingerLabel     = "index-finger"
		indexFingerName      = "index finger"
		middleFingerLabel    = "middle-finger"
		middleFingerName     = "middle finger"
		thumbName            = "thumb"
		thumbLabel           = "thumb"
		biodDir              = "/var/lib/biod/"
		pairingKeyFile       = "/var/lib/biod/CrosFpAuthStackManager/wrapped_pk"
		migrationFeatureName = "CrOSLateBootMigrateLegacyFingerprint"
		maxFailureAttempts   = 3
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec helper: ", err)
	}
	biodClient := hwsec.NewBiodClient(cmdRunner)

	// Cleanup vault at the end of the test.
	defer client.UnmountAndRemoveVault(ctxForCleanUp, userName)

	sanitizedUsername, err := client.GetSanitizedUsername(ctx, userName, false)
	if err != nil {
		s.Fatalf("Failed to get sanitized username for %s: %v", userName, err)
	}

	// Setup: Although we're going to do the actual setup in legacy fingerprint protocol first,
	// we need to get the pairing key required by the modern protocol set up beforehand. This
	// is because user login blocks further pairing key establishment for security reasons, and
	// we don't want to reboot in this test.
	func() {
		fpLoginCleanup, err := helper.EnableFingerprintLogin(ctx)
		if err != nil {
			s.Fatal("Failed to enable the FingerprintLogin feature in biod: ", err)
		}
		defer fpLoginCleanup(ctxForCleanUp)
		if testing.Poll(ctx, func(ctx context.Context) error {
			_, err := os.ReadFile(pairingKeyFile)
			return err
		}, &testing.PollOptions{Timeout: time.Second * 10, Interval: time.Millisecond * 100}); err != nil {
			s.Fatal("Failed to establish the pairing key")
		}
	}()

	// Setup: Set up legacy fingerprints to migrate later.
	func() {
		cr, err := chrome.New(ctx,
			chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}))
		if err != nil {
			s.Fatal("Failed to start Chrome at login screen: ", err)
		}
		defer cr.Close(ctxForCleanUp)

		// Step 1: Enroll the index finger.
		internal.PromptFingerEnroll(ctx, indexFingerName)
		if err := biodClient.Enroll(ctx, sanitizedUsername, indexFingerLabel); err != nil {
			s.Fatal("Enroll failed: ", err)
		}
		internal.PromptFingerLift(ctx)

		// Step 2: Enroll the middle finger.
		internal.PromptFingerEnroll(ctx, middleFingerName)
		if err := biodClient.Enroll(ctx, sanitizedUsername, middleFingerLabel); err != nil {
			s.Fatal("Enroll failed: ", err)
		}
		internal.PromptFingerLift(ctx)
	}()

	// Migration: Migrate legacy fingerprints to fingerprint factors, and enroll 1 more fingerprint.
	func() {
		// Enable fingerprint login feature.
		fpLoginCleanup, err := helper.EnableFingerprintLogin(ctx)
		if err != nil {
			s.Fatal("Failed to enable the FingerprintLogin feature in biod: ", err)
		}
		defer fpLoginCleanup(ctxForCleanUp)

		cr, err := chrome.New(ctx,
			chrome.KeepState(),
			chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}),
			chrome.EnableFeatures(migrationFeatureName),
			chrome.ExtraArgs("--ignore-unknown-auth-factors"))
		if err != nil {
			s.Fatal("Failed to start Chrome at login screen: ", err)
		}
		defer cr.Close(ctxForCleanUp)

		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate auth session with password")
			}

			// Step 1: Migrate fingerprints.
			if _, err := client.MigrateLegacyFingerprints(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to migrate legacy fingerprints")
			}
			return nil
		}); err != nil {
			s.Fatal("Failed to test fingerprint functionalities: ", err)
		}

		// Start auth session again because we want to authenticate fingerprint with verify-only intent.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			// Step 2: Test that both migrated fingers authenticate successfully.
			legacyFingerprintLabels := []string{"legacy-fp-1", "legacy-fp-2"}
			for _, fingerName := range []string{indexFingerName, middleFingerName} {
				if err := internal.MatchFinger(ctx, client, authSessionID, legacyFingerprintLabels, fingerName); err != nil {
					return errors.Wrapf(err, "failed to authenticate the %v", fingerName)
				}
			}
			return nil
		}); err != nil {
			s.Fatal("Failed to test fingerprint functionalities: ", err)
		}

		// Start auth session again to add more fingers.
		// We'll delete one of the legacy fingerprints, but we can't be sure which one it is.
		// Record the remaining fingerprint factor's label.
		remainingLegacyFpLabel := ""
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate auth session with password")
			}

			// Step 3: Add an extra finger: the thumb.
			if err := internal.EnrollFinger(ctx, client, authSessionID, thumbLabel, thumbName); err != nil {
				return errors.Wrapf(err, "failed to enroll the %v", thumbName)
			}

			// Step 4: Delete a migrated finger. Here it's a bit tricky because we want to delete
			// with the user specified name. Let's first ListAuthFactor to find out the label
			// we want to delete (might be legacy-fp-1 or legacy-fp-2).
			labelToDelete := ""
			reply, err := client.ListAuthFactors(ctx, userName)
			if err != nil {
				return errors.Wrap(err, "failed to list auth factors")
			}
			for _, factor := range reply.GetConfiguredAuthFactorsWithStatus() {
				if factor.AuthFactor.CommonMetadata.UserSpecifiedName == indexFingerLabel {
					labelToDelete = factor.AuthFactor.Label
				} else if factor.AuthFactor.CommonMetadata.UserSpecifiedName == middleFingerLabel {
					remainingLegacyFpLabel = factor.AuthFactor.Label
				}
			}
			if labelToDelete == "" {
				return errors.New("failed to find auth factor with name 'index finger'")
			}
			if remainingLegacyFpLabel == "" {
				return errors.New("failed to find auth factor with name 'middle finger'")
			}
			if err := client.RemoveAuthFactor(ctx, authSessionID, labelToDelete); err != nil {
				return errors.Wrapf(err, "failed to remove auth factor %s", labelToDelete)
			}
			return nil
		}); err != nil {
			s.Fatal("Failed to test fingerprint functionalities: ", err)
		}

		// Start auth session again because we want to authenticate fingerprint with verify-only intent.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			allFingerLabels := []string{remainingLegacyFpLabel, thumbLabel}
			// Step 5: Test that middle finger and thumb authenticate successfully.
			for _, fingerName := range []string{thumbName, middleFingerName} {
				if err := internal.MatchFinger(ctx, client, authSessionID, allFingerLabels, fingerName); err != nil {
					return errors.Wrapf(err, "failed to authenticate the %v", fingerName)
				}
			}
			// Step 6: Test that wrong finger fails and locks out fingerprint after 5 attempts.
			if err := internal.MatchWrongFingerUntilLockout(ctx, client, authSessionID, allFingerLabels, indexFingerName); err != nil {
				return errors.Wrap(err, "failed to verify wrong finger fails authentication")
			}
			// Step 7: Test that password authentication resets fingerprint lockout, and now fingerprint works again.
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate auth session with password")
			}
			if err := internal.MatchFinger(ctx, client, authSessionID, allFingerLabels, middleFingerName); err != nil {
				return errors.Wrapf(err, "failed to authenticate the %v", middleFingerName)
			}
			return nil
		}); err != nil {
			s.Fatal("Failed to test fingerprint functionalities: ", err)
		}
	}()

	// Rollback: If the fingerprint login feature is disabled, the available fingerprints
	// will revert back to the set of legacy fingerprints before migration. Note
	// deletion/relabelling of the migrated legacy fingerprints don't affect the
	// actual legacy fingerprint records.
	// Therefore, index finger and middle finger are available now, and thumb isn't.
	func() {
		cr, err := chrome.New(ctx,
			chrome.KeepState(),
			chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}),
			chrome.ExtraArgs("--ignore-unknown-auth-factors"))
		if err != nil {
			s.Fatal("Failed to start Chrome at login screen: ", err)
		}
		defer cr.Close(ctxForCleanUp)

		// Step 1: Ensure index finger authenticates successfully within 3 touches.
		internal.PromptFingerMatch(ctx, indexFingerName)
		if err := biodClient.MatchExpectSuccess(ctx, maxFailureAttempts); err != nil {
			s.Fatal("Match failed: ", err)
		}
		internal.PromptFingerLift(ctx)

		// Step 2: Ensure middle finger authenticates successfully within 3 touches.
		internal.PromptFingerMatch(ctx, middleFingerName)
		if err := biodClient.MatchExpectSuccess(ctx, maxFailureAttempts); err != nil {
			s.Fatal("Match failed: ", err)
		}
		internal.PromptFingerLift(ctx)

		// Step 3: Ensure thumb can't match successfully.
		internal.PromptFingerMatch(ctx, thumbName)
		if err := biodClient.MatchExpectFailure(ctx); err != nil {
			s.Fatal("Failed to expect a match failure: ", err)
		}
	}()
}
