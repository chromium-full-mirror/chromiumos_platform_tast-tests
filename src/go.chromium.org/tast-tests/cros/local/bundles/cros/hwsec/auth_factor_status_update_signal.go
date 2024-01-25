// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"encoding/hex"
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
	authFactorLabelPin      = "PIN"
	correctPin              = "123456"
	incorrectPin            = "000000"
	authFactorLabelPassword = "Password"
	correctPassword         = "p@ssword"
	testUser1               = "testUser11@example.com"
	numberOfLockoutAttempts = 4
	timeoutInMs             = 30000
	// Once the user is locked out, they are supposed to be locked out for |timeoutInMs| milliseconds. Because of the time it takes to run the command, we can not test the
	// actual time out because by the time the signal is received the timeout has probably changed a bit (time to run a few commands by the CPU). So we incorporate the
	// accepted delay in our calculations.
	delayBetweenLockoutAndSignalFetchInMs = 2000
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AuthFactorStatusUpdateSignal, LacrosStatus: testing.LacrosVariantUnneeded, Desc: "Checks that AuthFactorStatusUpdate signal is sent for the locked out users",
		Contacts: []string{
			"cryptohome-core@google.com",
			"behnoodm@chromium.org", // Test author
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"informational", "group:cryptohome", "group:mainline"},
		SoftwareDeps: []string{"pinweaver", "chrome"},
		Timeout:      100 * time.Second,
	})
}

func AuthFactorStatusUpdateSignal(ctx context.Context, s *testing.State) {
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
	if _, err := client.RemoveVault(ctx, testUser1); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}
	if setupErr := cryptochrome.WithModernPin(ctx, func() error {
		// Setup a new user with PIN.
		setupUserWithModernPIN(ctx, ctxForCleanup, testUser1, cmdRunner, helper)

		// Attempt to lockout the user so the signal could be sent.
		authSession, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUser1, false /* ephemeral */, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			s.Fatal("Failed to start auth session for PIN authentication: ", err)
		}
		// Make sure the authSession is invalidated even if the test fails.
		defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanup, authSessionID)
		// We need to attempt until one attempt before locking out, so we are ready to catch the signal once an attempt locks the user out.
		for i := 0; i < numberOfLockoutAttempts-1; i++ {
			_, err = cryptohomeHelper.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelPin, incorrectPin)
			if err == nil {
				s.Fatal("Authentication with wrong PIN succeeded unexpectedly: ", err)
			}
		}
		var authFactorStatusUpdateReply *uda.AuthFactorStatusUpdate
		authFactorStatusUpdateReply, err = cryptohomeHelper.FailAuthenticatePinAuthFactorAndFetchStatusUpdate(ctx, authSessionID, authFactorLabelPin, incorrectPin, authSession.BroadcastId)
		if err != nil {
			s.Fatal("Authentication succeeded while it was expected to fail or status update signal was not fetched properly: ", err)
		}
		// Check if the timeout is correct
		timeAvailableIn := authFactorStatusUpdateReply.AuthFactorWithStatus.StatusInfo.TimeAvailableIn
		if timeAvailableIn > timeoutInMs || timeAvailableIn <= 0 || timeoutInMs-timeAvailableIn > delayBetweenLockoutAndSignalFetchInMs {
			s.Fatal("time_available in is not set properly right after the lock out: ", timeAvailableIn)
		}

		// Check if the timeout delay between two signals is correct.
		_, err = cryptohomeHelper.FetchStatusUpdateSignal(ctx, authSession.BroadcastId)
		if err != nil {
			s.Fatal("StatusUpdateSignal was not fetched or its BroadcastID did not match that of the AuthSession: ", err)
		}
		cryptohomeHelper.InvalidateAuthSession(ctxForCleanup, authSessionID)
		// Test if the status update signal is sent for a new auth session for a locked out user.
		authSession, _, err = cryptohomeHelper.StartAuthSessionWithStatusUpdate(ctx, testUser1, false /* ephemeral */, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			s.Fatal("Failed to start auth session for PIN authentication: ", err)
		}
		// Make sure the authSession is invalidated even if the test fails.
		defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanup, hex.EncodeToString(authSession.AuthSessionId))
		return nil
	}); setupErr != nil {
		s.Fatal("Failed to setup with modern pin enabled: ", setupErr)
	}
}

// setupUserWithModernPIN sets up a user with a password and a PIN auth factor.
func setupUserWithModernPIN(ctx, ctxForCleanUp context.Context, userName string, cmdRunner *hwseclocal.CmdRunnerLocal, helper *hwseclocal.CmdHelperLocal) error {
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

	err = cryptohomeHelper.AddAuthFactor(ctx, authSessionID, authFactorLabelPassword, correctPassword)

	if err != nil {
		return errors.Wrap(err, "failed to add password auth factor")
	}
	// Add a PIN auth factor to the user.
	err = cryptohomeHelper.AddModernPinAuthFactor(ctx, authSessionID, authFactorLabelPin, correctPin)

	if err != nil {
		return errors.Wrap(err, "failed to add le credential")
	}
	return nil
}
