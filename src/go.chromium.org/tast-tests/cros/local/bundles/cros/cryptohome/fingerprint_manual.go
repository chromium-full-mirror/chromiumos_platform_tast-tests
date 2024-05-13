// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"math"
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
		Func:         FingerprintManual,
		Desc:         "Checks that cryptohome fingerprint auth factor functions correctly through manual interaction with FP sensor",
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

func FingerprintManual(ctx context.Context, s *testing.State) {
	const (
		userName          = "foo@bar.baz"
		userPassword      = "secret"
		passwordLabel     = "gaia"
		indexFingerLabel  = "index-finger"
		indexFingerName   = "index finger"
		middleFingerLabel = "middle-finger"
		middleFingerName  = "middle finger"
		// This finger won't be enrolled
		thumbFingerName = "thumb"
		biodDir         = "/var/lib/biod/"
	)
	fingersToEnroll := []struct {
		label string
		name  string
	}{
		{indexFingerLabel, indexFingerName},
		{middleFingerLabel, middleFingerName},
	}
	var allFingerLabels []string
	for _, finger := range fingersToEnroll {
		allFingerLabels = append(allFingerLabels, finger.label)
	}

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)

	fpLoginCleanup, err := helper.EnableFingerprintLogin(ctx)
	if err != nil {
		s.Fatal("Failed to enable the FingerprintLogin feature in biod: ", err)
	}
	defer fpLoginCleanup(ctxForCleanUp)

	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}),
		chrome.ExtraArgs("--ignore-unknown-auth-factors"))
	if err != nil {
		s.Fatal("Failed to start Chrome at login screen: ", err)
	}
	vaultNeedsRemove := true
	defer func(ctx context.Context) {
		if vaultNeedsRemove {
			client.UnmountAndRemoveVault(ctx, userName)
		}
	}(ctxForCleanUp)
	defer cr.Close(ctxForCleanUp)

	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate auth session with password")
		}
		// Step 1: Enroll both index finger and middle finger.
		for _, finger := range fingersToEnroll {
			if err := internal.EnrollFinger(ctx, client, authSessionID, finger.label, finger.name); err != nil {
				return errors.Wrapf(err, "failed to enroll the %v", finger.name)
			}
		}
		return nil
	}); err != nil {
		s.Fatal("Failed test fingerprint functionalities: ", err)
	}
	// Start auth session again because we want to authenticate fingerprint with verify-only intent.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
		// Step 2: Test that both fingers authenticate successfully.
		for _, finger := range fingersToEnroll {
			if err := internal.MatchFinger(ctx, client, authSessionID, allFingerLabels, finger.name); err != nil {
				return errors.Wrapf(err, "failed to authenticate the %v", finger.name)
			}
		}
		// Step 3: Test that wrong finger fails and locks out fingerprint after 5 attempts.
		if err := internal.MatchWrongFingerUntilLockout(ctx, client, authSessionID, allFingerLabels, thumbFingerName); err != nil {
			return errors.Wrap(err, "failed to verify wrong finger fails authentication")
		}
		reply, _, err := client.StartAuthSession(ctx, userName, false, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			return errors.Wrap(err, "failed to start auth session")
		}
		configuredFingerprints := 0
		authFactors := reply.ConfiguredAuthFactorsWithStatus
		for _, authFactor := range authFactors {
			if authFactor.AuthFactor.Type == uda.AuthFactorType_AUTH_FACTOR_TYPE_FINGERPRINT {
				configuredFingerprints++
				if authFactor.StatusInfo.TimeAvailableIn != math.MaxUint64 {
					return errors.New("failed to verify fingerprint is locked out")
				}
			}
		}
		if configuredFingerprints != len(fingersToEnroll) {
			return errors.Errorf("incorrect number of configured fingerprints: got %v, expected %v", configuredFingerprints, len(fingersToEnroll))
		}
		// Step 4: Test that password authentication resets fingerprint lockout, and now fingerprint works again.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate auth session with password")
		}
		if err := internal.MatchFinger(ctx, client, authSessionID, allFingerLabels, fingersToEnroll[0].name); err != nil {
			return errors.Wrapf(err, "failed to authenticate the %v", fingersToEnroll[0].name)
		}
		return nil
	}); err != nil {
		s.Fatal("Failed test fingerprint functionalities: ", err)
	}

	// Check if the fingerprint directory of the user exists. This is mainly to ensure that we didn't get wrong
	// with the path name, such that checking this directory is properly removed below is more effective.
	userHash, err := client.GetUserHash(ctx, userName)
	if err != nil {
		s.Fatal("Failed to get user hash: ", err)
	}
	fingerprintDir := biodDir + userHash
	if _, err := os.ReadDir(fingerprintDir); err != nil {
		s.Fatal("Failed to ensure fingerprint directory exists: ", err)
	}

	if err := client.UnmountAndRemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to unmount and remove the user's vault: ", err)
	}
	vaultNeedsRemove = false

	// Check if fingerprint directory of the removed user is removed.
	if _, err := os.ReadDir(fingerprintDir); !os.IsNotExist(err) {
		if err == nil {
			s.Fatal("Failed to ensure fingerprint directory removed: directory exists")
		} else {
			s.Fatal("Failed to ensure fingerprint directory removed: ", err)
		}
	}
}
