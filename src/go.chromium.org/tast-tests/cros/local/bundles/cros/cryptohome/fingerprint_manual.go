// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
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
		// have logged-in a user so we need to reboot.
		Fixture: "rebootFixture",
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
		biodDir           = "/var/lib/biod/"
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
			if err := enrollFinger(ctx, client, authSessionID, finger.label, finger.name); err != nil {
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
			if err := authFinger(ctx, client, authSessionID, allFingerLabels, finger.name); err != nil {
				return errors.Wrapf(err, "failed to authenticate the %v", finger.name)
			}
		}
		// Step 3: Test that wrong finger fails and locks out fingerprint after 5 attempts.
		if err := authFingerLockout(ctx, client, authSessionID, allFingerLabels); err != nil {
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
		if err := authFinger(ctx, client, authSessionID, allFingerLabels, fingersToEnroll[0].name); err != nil {
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

func enrollFinger(ctx context.Context, client *hwsec.CryptohomeClient, authSessionID, fingerLabel, fingerName string) error {
	// Fingerprint enrollment might take up to 10 touches. Reserve 30 seconds.
	ctxWatcher, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	watcher, err := cryptohome.NewFingerprintEnrollmentWatcher(ctxWatcher)
	if err != nil {
		return errors.Wrap(err, "failed to create FingerprintEnrollmentWatcher")
	}
	defer watcher.Close(ctx)
	if _, err := client.PrepareAddFpAuthFactor(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to prepare fingerprint auth factor for add")
	}
	defer client.TerminateFpAuthFactor(ctx, authSessionID)

	promptFingerTouch(ctx, fingerName)
	for {
		select {
		case sig, ok := <-watcher.Signals:
			if !ok {
				return errors.New("enrollment signal channel closed unexpectedly")
			}
			showEnrollmentProgress(ctx, *sig)
			if sig.Done {
				if err := client.AddFingerprintAuthFactor(ctx, authSessionID, fingerLabel); err != nil {
					return errors.Wrap(err, "failed to add fingerprint auth factor")
				}
				return nil
			}
			// There's a special error code that signals the enrollment session can't be continued.
			if sig.ScanResult == uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_FATAL_ERROR {
				return errors.New("fingerprint enrollment failed with internal error")
			}
			promptFingerTouch(ctx, fingerName)
		case <-ctxWatcher.Done():
			return errors.New("fingerprint enrollment timed out")
		}
	}
}

func showEnrollmentProgress(ctx context.Context, sig cryptohome.FingerprintEnrollmentSignal) {
	var scanStatusString string
	if sig.ScanResult == uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_SUCCESS {
		scanStatusString = "Scan success"
	} else {
		// Note that the failure might just be a hint, and percent-complete might still increase
		// even if the scan status isn't success.
		scanStatusString = fmt.Sprintf("Scan failed: %v", sig.ScanResult)
	}
	testing.ContextLogf(ctx, "%s, progress: %v%%", scanStatusString, sig.PercentComplete)
}

func authFinger(ctx context.Context, client *hwsec.CryptohomeClient, authSessionID string, fingerLabels []string, fingerName string) error {
	// Fingerprint authentication might take up to 5 touches due to false negatives (5th touch will lock it out anyway).
	// Reserve 5 seconds for each touch, 25 seconds in total.
	ctxWatcher, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	watcher, err := cryptohome.NewFingerprintAuthenticationWatcher(ctxWatcher)
	if err != nil {
		return errors.Wrap(err, "failed to create FingerprintAuthenticationWatcher")
	}
	defer watcher.Close(ctx)
	if _, err := client.PrepareAuthFpAuthFactor(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to prepare fingerprint auth factor for auth")
	}
	defer client.TerminateFpAuthFactor(ctx, authSessionID)

	promptFingerTouch(ctx, fingerName)
	for {
		select {
		case sig, ok := <-watcher.Signals:
			if !ok {
				return errors.New("authentication signal channel closed unexpectedly")
			}
			// Scan should always succeed if there's no internal errors, as match is a later step.
			if sig.ScanResult != uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_SUCCESS {
				return errors.New("fingerprint authentication failed with internal error")
			}
			if reply, err := client.AuthenticateFingerprintAuthFactor(ctx, authSessionID, fingerLabels); err != nil {
				if reply.ErrorInfo.PrimaryAction != uda.PrimaryAction_PRIMARY_INCORRECT_AUTH {
					// This error isn't retryable, return error.
					return errors.Wrap(err, "failed to authenticate fingerprint auth factor")
				}
				testing.ContextLog(ctx, "Fingerprint auth failed, please retry")
			} else {
				return nil
			}
			promptFingerTouch(ctx, fingerName)
		case <-ctxWatcher.Done():
			return errors.New("fingerprint authentication timed out")
		}
	}
}

func authFingerLockout(ctx context.Context, client *hwsec.CryptohomeClient, authSessionID string, fingerLabels []string) error {
	const (
		// This finger isn't enrolled. It's used for testing failure cases.
		wrongFingerName      = "thumb"
		lockoutWrongAttempts = 5
	)

	// Fingerprint authentication takes 5 touches to get locked.
	// Reserve 5 seconds for each touch, 25 seconds in total.
	ctxWatcher, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	watcher, err := cryptohome.NewFingerprintAuthenticationWatcher(ctxWatcher)
	if err != nil {
		return errors.Wrap(err, "failed to create FingerprintAuthenticationWatcher")
	}
	defer watcher.Close(ctx)
	if _, err := client.PrepareAuthFpAuthFactor(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to prepare fingerprint auth factor for auth")
	}
	defer client.TerminateFpAuthFactor(ctx, authSessionID)

	promptFingerTouch(ctx, wrongFingerName)
	for i := 1; i <= lockoutWrongAttempts; i++ {
		select {
		case sig, ok := <-watcher.Signals:
			if !ok {
				return errors.New("authentication signal channel closed unexpectedly")
			}
			// Scan should always succeed if there's no internal errors, as match is a later step.
			if sig.ScanResult != uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_SUCCESS {
				return errors.New("fingerprint authentication failed with internal error")
			}
			if reply, err := client.AuthenticateFingerprintAuthFactor(ctx, authSessionID, fingerLabels); err != nil {
				var expectedAction uda.PrimaryAction
				if i == lockoutWrongAttempts {
					expectedAction = uda.PrimaryAction_PRIMARY_LE_LOCKED_OUT
				} else {
					expectedAction = uda.PrimaryAction_PRIMARY_INCORRECT_AUTH
				}
				if reply.ErrorInfo.PrimaryAction != expectedAction {
					return errors.Wrapf(err, "authenticate fingerprint did not fail with primary action %v", expectedAction)
				}
				testing.ContextLog(ctx, "Fingerprint auth failed as expected")
			} else {
				return errors.New("fingerprint authentication succeeded with a wrong finger")
			}
			promptFingerTouch(ctx, wrongFingerName)
		case <-ctxWatcher.Done():
			return errors.New("fingerprint authentication timed out")
		}
	}
	return nil
}

func promptFingerTouch(ctx context.Context, fingerName string) {
	// This will look like:
	// ******************************
	// * Please press your X finger *
	// ******************************
	promptString := "* Please press your " + fingerName + " *"
	asterisks := strings.Repeat("*", len(promptString))
	testing.ContextLogf(ctx, "%s", asterisks)
	testing.ContextLogf(ctx, "%s", promptString)
	testing.ContextLogf(ctx, "%s", asterisks)
}
