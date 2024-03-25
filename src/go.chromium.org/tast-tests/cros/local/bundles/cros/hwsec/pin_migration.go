// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	cryptochrome "go.chromium.org/tast-tests/cros/local/cryptohome/chrome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	authFactorLabelOldPin = "testPin"
	correctOldPin         = "654321"
	wrongPin              = "000000"
	authFactorLabelPass   = "Password"
	correctPass           = "p@ssword"
	testUsername          = "testUser11@example.com"
	lockoutPeriod         = 30
	numberOfAttempts      = 5
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PinMigration, LacrosStatus: testing.LacrosVariantUnneeded, Desc: "Checks that the modern pin migration works properly",
		Contacts: []string{
			"cryptohome-core@google.com",
			"behnoodm@chromium.org", // Test author
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"informational", "group:cryptohome", "group:mainline"},
		SoftwareDeps: []string{"pinweaver", "chrome"},
		Timeout:      150 * time.Second,
	})
}

func PinMigration(ctx context.Context, s *testing.State) {
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)
	cryptohomeHelper := helper.CryptohomeClient()

	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	daemonController := helper.DaemonController()

	// Wait for cryptohomed to become available if needed.
	if err := daemonController.Ensure(ctx, hwsec.CryptohomeDaemon); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	supportsLE, err := client.SupportsLECredentials(ctx)
	if err != nil {
		s.Fatal("Failed to get supported policies: ", err)
	} else if !supportsLE {
		s.Fatal("Device does not support PinWeaver")
	}

	// Clean up obsolete state, in case there's any.
	cmdRunner.Run(ctx, "rm -rf /home/.shadow/low_entropy_creds")
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if _, err := client.RemoveVault(ctx, testUsername); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Setup a new user with PIN while the modern pin is disabled.
	if disableModernPinSetupErr := cryptochrome.WithModernPinDisabled(ctx, func() error {
		if err := setupUserWithLegacyPin(ctx, ctxForCleanup, testUsername, cmdRunner, helper); err != nil {
			s.Fatal("Failed to set up a user with a pin auth factor while modern pin is disabled: ", err)
		}
		return nil
	}); disableModernPinSetupErr != nil {
		s.Fatal("Failed to set up with modern pin disabled: ", disableModernPinSetupErr)
	}

	if migrationSetupErr := cryptochrome.WithMigrationPin(ctx, func() error {
		_, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUsername, false /* ephemeral */, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			s.Fatal("Failed to start auth session for PIN authentication: ", err)
		}
		// Make sure the authSession is invalidated even if the test fails.
		defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanup, authSessionID)
		// The difference between ModernPin and the old pin is that the
		// old pin will lockout indefinitely upon locking out, however
		// the ModernPin will lock out for a specified amount of time.
		// So we make sure the old pin gets locked out, then authenticate
		// with it so it migrates to the modern pin, lock it out again
		// and check if we can login with it after the |lockoutPeriod|.
		for i := 0; i < numberOfAttempts; i++ {
			_, err = cryptohomeHelper.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelOldPin, wrongPin)
			if err == nil {
				s.Fatal("Authentication with wrong PIN succeeded unexpectedly: ", err)
			}
		}
		// GoBigSleepLint: need to check lockout correctness
		testing.Sleep(ctx, lockoutPeriod*time.Second)
		_, err = cryptohomeHelper.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelOldPin, correctOldPin)
		if err == nil {
			s.Fatal("Authentication succeeded while it was expected to fail for old pin after lockout: ", err)
		}
		// Unlock the PIN
		_, err = client.AuthenticateAuthFactor(ctx, authSessionID, authFactorLabelPass, correctPass)
		if err != nil {
			s.Fatal("Authentication with Password failed: ", err)
		}
		// The migration should happen as soon as a successful pin login happens
		_, err = client.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelOldPin, correctOldPin)
		if err != nil {
			s.Fatal("Authentication or Migration failed for the set up pin: ", err)
		}

		for i := 0; i < numberOfAttempts; i++ {
			_, err = cryptohomeHelper.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelOldPin, wrongPin)
			if err == nil {
				s.Fatal("Authentication with wrong PIN succeeded unexpectedly: ", err)
			}
		}
		// GoBigSleepLint: need to check lockout correctness
		testing.Sleep(ctx, lockoutPeriod*time.Second)
		_, err = cryptohomeHelper.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelOldPin, correctOldPin)
		if err != nil {
			s.Fatal("Authentication failed while it was expected to succeed for modern pin after lockout: ", err)
		}
		return nil
	}); migrationSetupErr != nil {
		s.Fatal("Failed to setup with migration pin enabled: ", migrationSetupErr)
	}
}

// setupUserWithLegacyPin sets up a user with a password and a legacy pin.
func setupUserWithLegacyPin(ctx, ctxForCleanUp context.Context, userName string, cmdRunner *hwseclocal.CmdRunnerLocal, helper *hwseclocal.CmdHelperLocal) error {
	cryptohomeHelper := helper.CryptohomeClient()

	// Start an Auth session and get an authSessionID.
	_, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session for PIN authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctx, authSessionID)

	if err = cryptohomeHelper.CreatePersistentUser(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to create persistent user with auth session")
	}

	if _, err = cryptohomeHelper.PreparePersistentVault(ctx, authSessionID, false); err != nil {
		return errors.Wrap(err, "failed to prepare persistent user with auth session")
	}
	defer cryptohomeHelper.Unmount(ctx, userName)

	err = cryptohomeHelper.AddAuthFactor(ctx, authSessionID, authFactorLabelPass, correctPass)

	if err != nil {
		return errors.Wrap(err, "failed to add password auth factor")
	}
	// Add an old PIN auth factor to the user.
	err = cryptohomeHelper.AddPinAuthFactor(ctx, authSessionID, authFactorLabelOldPin, correctOldPin)

	if err != nil {
		return errors.Wrap(err, "failed to add le credential")
	}
	return nil
}
