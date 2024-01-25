// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"encoding/hex"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PINWeaver,
		Desc: "Checks that LE credentials work with AuthSession, AuthFactor and USS",
		Contacts: []string{
			"cryptohome-core@google.com",
			"hardikgoyal@chromium.org", // Test author
		},
		BugComponent: "b:1188704",
		Attr:         []string{"informational", "group:mainline"},
		SoftwareDeps: []string{"pinweaver", "reboot"},
	})
}

// Some constants used across the test.
const (
	authFactorLabelPIN       = "lecred"
	correctPINSecret         = "123456"
	incorrectPINSecret       = "000000"
	passwordAuthFactorLabel  = "fake_label"
	passwordAuthFactorSecret = "password"
	testUser1                = "testUser1@example.com"
	testUser2                = "testUser2@example.com"
)

func PINWeaver(ctx context.Context, s *testing.State) {
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	cryptohomeHelper := helper.CryptohomeClient()

	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}

	daemonController := helper.DaemonController()

	// Wait for cryptohomed becomes available if needed.
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
	if _, err := client.RemoveVault(ctx, testUser2); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	/**Initial User Setup. Test both user 1 and user 2 can login successfully.**/
	// Setup a user 1 for testing. This user will be locked out and re-authed to ensure the PIN is unlocked.
	// This user will be removed at last and the le_credential file will be checked.
	if err = setupUserWithPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run setupUserWithPIN with error: ", err)
	}
	defer cryptohomeHelper.UnmountAndRemoveVault(ctxForCleanUp, testUser1)

	// Ensure we can authenticate with correct pin.
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	// Ensure we can authenticate with correct password.
	if err = authenticateWithCorrectPassword(ctx, ctxForCleanUp, testUser1, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPassword with error: ", err)
	}

	// Setup a user 2 for testing. This user's pin will be removed and the le_credential file will be checked.
	if err = setupUserWithPIN(ctx, ctxForCleanUp, testUser2, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run setupUserWithPIN with error: ", err)
	}
	defer cryptohomeHelper.UnmountAndRemoveVault(ctxForCleanUp, testUser2)

	// Ensure we can authenticate with correct password for testUser2.
	if err = authenticateWithCorrectPassword(ctx, ctxForCleanUp, testUser2, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPassword with error: ", err)
	}

	// Ensure we can authenticate with correct pin for testUser2.
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser2, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	// Ensure that testUser1 still works with pin.
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	// Ensure that testUser1 still works with password.
	if err = authenticateWithCorrectPassword(ctx, ctxForCleanUp, testUser1, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPassword with error: ", err)
	}

	/** Running test where we try to almost lock out PIN with 4 attempts twice, but the user is able to log back in **/
	// Attempt four wrong PIN.
	if _, err = attemptWrongPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, 4 /*attempts*/); err != nil {
		s.Fatal("Failed to run attemptWrongPIN with error: ", err)
	}

	// Since the pin is not locked out yet, we should be able to log back in again.
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	// Attempt four wrong PIN again.
	replyWithError, err := attemptWrongPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, 4 /*attempts*/)
	if err != nil {
		s.Fatal("Failed to run attemptWrongPIN with error: ", err)
	}

	// Ensure AutheneticateAuthFactor error code relays TPM is not locked out.
	if ec := replyWithError.GetError(); ec != uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_AUTHORIZATION_KEY_FAILED {
		s.Fatal("TPM is locked out: ", ec)
	}
	if pa := replyWithError.GetErrorInfo().PrimaryAction; pa != uda.PrimaryAction_PRIMARY_INCORRECT_AUTH {
		s.Fatal("Incorrect pin should result in IncorrectAuth primary action, but got: ", pa)
	}

	// Since the pin is not locked out yet, we should be able to log back in again.
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	/** Test whether the attempt counter persists after reboot **/
	// Attempt four wrong PIN.
	if _, err = attemptWrongPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, 4 /*attempts*/); err != nil {
		s.Fatal("Failed to run attemptWrongPIN with error: ", err)
	}

	// Check to make sure that PIN AuthFactor appears in StartAuthSessionReply.
	reply, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUser1, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session when searching for PIN factor in reply: ", err)
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctx, authSessionID)
	// Search for PIN-based AuthFactor in reply.
	hasPinAuthFactor := false
	for _, authFactor := range reply.AuthFactors {
		if authFactor.Type == uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN {
			hasPinAuthFactor = true
		}
	}
	if !hasPinAuthFactor {
		s.Fatal("PIN-based AuthFactor was not found in StartAuthSessionReply")
	}

	// Because Cr50 stores state in the firmware, that persists across reboots, this test
	// needs to run before and after a reboot.
	if err = helper.Reboot(ctx); err != nil {
		s.Fatal("Failed to run helper with error: ", err)
	}

	// Lockout the PIN this time.
	_, err = attemptWrongPinToLockoutAndGetStatusUpdate(ctx, ctxForCleanUp, testUser1, helper)
	if err != nil {
		s.Fatal("Failed to run attemptWrongPinToLockoutAndGetStatusUpdate with error: ", err)
	}
	// Check if the signal is sent again after a new session is established.
	_, err = fetchStatusUpdateUponNewAuthSession(ctx, ctxForCleanUp, testUser1, helper)
	if err != nil {
		s.Fatal("Failed to fetch StatusUpdateSignal after a new AuthSession is established after the user is locked out with error: ", err)
	}
	if err = ensurePINLockedOut(ctx, testUser1, client); err != nil {
		s.Fatal("Failed to run ensurePINLockedOut with error: ", err)
	}

	// After the PIN lock out we should not be able to authenticate with correct PIN.
	if replyWithError, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, false /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}
	// Ensure AutheneticateAuthFactor error code relays TPM is locked out.
	if ec := replyWithError.GetError(); ec != uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_TPM_DEFEND_LOCK {
		s.Fatal("AuthenticateAuthFactor indicates that the TPM is not locked out: ", ec)
	}
	if pa := replyWithError.GetErrorInfo().PrimaryAction; pa != uda.PrimaryAction_PRIMARY_LE_LOCKED_OUT {
		s.Fatal("Incorrect pin should result in LeLockedOut primary action, but got: ", pa)
	}

	// Check to make sure that PIN AuthFactor does not appear in StartAuthSessionReply.
	reply, authSessionID, err = cryptohomeHelper.StartAuthSession(ctx, testUser1, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session when searching for PIN factor in reply: ", err)
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctx, authSessionID)
	// Search for PIN-based AuthFactor in reply.
	for _, authFactor := range reply.AuthFactors {
		if authFactor.Type == uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN {
			s.Fatal("PIN-based AuthFactor was found in StartAuthSessionReply")
		}
	}

	/** Ensure that testUser2 can still use PIN **/
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser2, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	/** Unlock PIN **/
	if err = authenticateWithCorrectPassword(ctx, ctxForCleanUp, testUser1, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPassword with error: ", err)
	}

	// Ensure pin login now works again for testUser1.
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	// Remove the added PIN and check to see if le_credential file was updated.
	if err = removeLeCredential(ctx, ctxForCleanUp, testUser2, authFactorLabelPIN, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run removeLeCredential with error: ", err)
	}

	/** Ensure test user 1 can still login with PIN**/
	if _, err = authenticateWithCorrectPIN(ctx, ctxForCleanUp, testUser1, cmdRunner, helper, true /*shouldAuthenticate*/); err != nil {
		s.Fatal("Failed to run authenticateWithCorrectPIN with error: ", err)
	}

	// Remove test user 1 and check to see if le_credential file was updated.
	if err = removeUser(ctx, ctxForCleanUp, testUser1, cmdRunner, helper); err != nil {
		s.Fatal("Failed to run removeUser with error: ", err)
	}
}

// getLeCredsFromDisk gets the LE Credential file from disk.
func getLeCredsFromDisk(ctx context.Context, r *hwsecremote.CmdRunnerRemote) ([]string, error) {
	output, err := r.Run(ctx, "/bin/ls", "/home/.shadow/low_entropy_creds")
	if err != nil {
		return nil, err
	}

	labels := strings.Split(string(output), "\n")
	sort.Strings(labels)
	return labels, nil
}

// setupUserWithPIN sets up a user with a password and a PIN auth factor.
func setupUserWithPIN(ctx, ctxForCleanUp context.Context, userName string, cmdRunner *hwsecremote.CmdRunnerRemote, helper *hwsecremote.CmdHelperRemote) error {
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

	err = cryptohomeHelper.AddAuthFactor(ctx, authSessionID, passwordAuthFactorLabel, passwordAuthFactorSecret)

	if err != nil {
		return errors.Wrap(err, "failed to add password auth factor")
	}

	leCredsBeforeAdd, err := getLeCredsFromDisk(ctx, cmdRunner)
	if err != nil {
		return errors.Wrap(err, "failed to get le creds from disk before add")
	}

	// Add a PIN auth factor to the user.
	err = cryptohomeHelper.AddPinAuthFactor(ctx, authSessionID, authFactorLabelPIN, correctPINSecret)

	if err != nil {
		return errors.Wrap(err, "failed to add le credential")
	}
	leCredsAfterAdd, err := getLeCredsFromDisk(ctx, cmdRunner)
	if err != nil {
		return errors.Wrap(err, "failed to get le creds from disk after add")
	}

	if diff := cmp.Diff(leCredsAfterAdd, leCredsBeforeAdd); diff == "" {
		return errors.Wrap(err, "le cred file did not change after add")
	}
	return nil
}

// attemptWrongPIN attempts to try wrong PIN for authentication for given number of attempts.
func attemptWrongPIN(ctx, ctxForCleanUp context.Context, testUser string, r *hwsecremote.CmdRunnerRemote, helper *hwsecremote.CmdHelperRemote, numberOfWrongAttempts int) (hwsec.UserDataAuthReplyWithError, error) {
	cryptohomeHelper := helper.CryptohomeClient()

	// Authenticate a new auth session via the new added PIN auth factor.
	_, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start auth session for PIN authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanUp, authSessionID)

	var reply hwsec.UserDataAuthReplyWithError
	reply = &uda.AuthenticateAuthFactorReply{}
	// Supply invalid credentials five times to trigger firmware lockout of the credential.
	for i := 0; i < numberOfWrongAttempts; i++ {
		reply, err = cryptohomeHelper.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelPIN, incorrectPINSecret)
		if err == nil {
			return nil, errors.Wrap(err, "authentication with wrong PIN succeeded unexpectedly")
		}
	}
	return reply, nil
}

// attemptWrongPinToLockoutAndGetStatusUpdate should be called only when the next attempt is expected to lock the user out. This function will attempt the last attempt with the wrong pin and attaches
// itself to the upcoming AuthFactorStatusUpdate signal.
func attemptWrongPinToLockoutAndGetStatusUpdate(ctx, ctxForCleanUp context.Context, testUser string, helper *hwsecremote.CmdHelperRemote) (*uda.AuthFactorStatusUpdate, error) {
	cryptohomeHelper := helper.CryptohomeClient()

	// Authenticate a new auth session via the new added PIN auth factor.
	authSession, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start auth session for PIN authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanUp, authSessionID)

	var reply *uda.AuthFactorStatusUpdate
	reply, err = cryptohomeHelper.FailAuthenticatePinAuthFactorAndFetchStatusUpdate(ctx, authSessionID, authFactorLabelPIN, incorrectPINSecret, authSession.BroadcastId)
	if err != nil {
		return nil, errors.Wrap(err, "the authentication succeeded with pin although we were expecting it to fail or failed to get the status update signal")
	}
	return reply, nil
}

// fetchStatusUpdateUponNewAuthSession attempts to fetch the signal after a user is locked out and a new AuthSession is established for them.
func fetchStatusUpdateUponNewAuthSession(ctx, ctxForCleanUp context.Context, testUser string, helper *hwsecremote.CmdHelperRemote) (*uda.AuthFactorStatusUpdate, error) {
	cryptohomeHelper := helper.CryptohomeClient()

	// Authenticate a new auth session via the new added PIN auth factor.
	authSession, authFactorStatusUpdate, err := cryptohomeHelper.StartAuthSessionWithStatusUpdate(ctx, testUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start auth session for PIN authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanUp, hex.EncodeToString(authSession.AuthSessionId))
	return authFactorStatusUpdate, nil
}

// authenticateWithCorrectPIN authenticates a given user with the correct PIN.
func authenticateWithCorrectPIN(ctx, ctxForCleanUp context.Context, testUser string, r *hwsecremote.CmdRunnerRemote, helper *hwsecremote.CmdHelperRemote, shouldAuthenticate bool) (hwsec.UserDataAuthReplyWithError, error) {
	cryptohomeHelper := helper.CryptohomeClient()

	// Authenticate a new auth session via the new added PIN auth factor.
	_, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start auth session for PIN authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanUp, authSessionID)

	reply := &uda.AuthenticateAuthFactorReply{}
	reply, err = cryptohomeHelper.AuthenticatePinAuthFactor(ctx, authSessionID, authFactorLabelPIN, correctPINSecret)
	if (err == nil) != shouldAuthenticate {
		return reply, errors.Wrapf(err, "failed to authenticated auth factor with correct PIN. got %v, want %v", (err == nil), shouldAuthenticate)
	}
	return reply, nil
}

// authenticateWithCorrectPassword authenticates a given user with the correct password.
func authenticateWithCorrectPassword(ctx, ctxForCleanUp context.Context, testUser string, r *hwsecremote.CmdRunnerRemote, helper *hwsecremote.CmdHelperRemote) error {
	cryptohomeHelper := helper.CryptohomeClient()

	// Authenticate a new auth session via the new password auth factor and mount the user.
	_, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session for password authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanUp, authSessionID)

	// Authenticate with correct password.
	reply, err := cryptohomeHelper.AuthenticateAuthFactor(ctx, authSessionID, passwordAuthFactorLabel, passwordAuthFactorSecret)
	if err != nil {
		return errors.Wrap(err, "failed to authenticate auth factor")
	}
	if err := cryptohomecommon.ExpectAuthIntents(reply.AuthorizedFor, []uda.AuthIntent{
		uda.AuthIntent_AUTH_INTENT_DECRYPT,
		uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY,
	}); err != nil {
		return errors.Wrap(err, "unexpected AuthSession authorized intents")
	}

	return nil
}

// removeLeCredential removes testUser and checks to see if the leCreds on disk was updated.
func removeLeCredential(ctx, ctxForCleanUp context.Context, testUser, label string, r *hwsecremote.CmdRunnerRemote, helper *hwsecremote.CmdHelperRemote) error {
	cryptohomeHelper := helper.CryptohomeClient()

	_, authSessionID, err := cryptohomeHelper.StartAuthSession(ctx, testUser, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session for authentication")
	}
	defer cryptohomeHelper.InvalidateAuthSession(ctxForCleanUp, authSessionID)

	// Authenticate with correct password.
	reply, err := cryptohomeHelper.AuthenticateAuthFactor(ctx, authSessionID, passwordAuthFactorLabel, passwordAuthFactorSecret)
	if err != nil {
		return errors.Wrap(err, "failed to authenticate auth factor")
	}
	if err := cryptohomecommon.ExpectAuthIntents(reply.AuthorizedFor, []uda.AuthIntent{
		uda.AuthIntent_AUTH_INTENT_DECRYPT,
		uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY,
	}); err != nil {
		return errors.Wrap(err, "unexpected AuthSession authorized intents")
	}

	leCredsBeforeRemove, err := getLeCredsFromDisk(ctx, r)
	if err != nil {
		return errors.Wrap(err, "failed to get le creds from disk")
	}

	if err := cryptohomeHelper.RemoveAuthFactor(ctx, authSessionID, label); err != nil {
		return errors.Wrap(err, "failed to remove auth factor")
	}

	leCredsAfterRemove, err := getLeCredsFromDisk(ctx, r)
	if err != nil {
		return errors.Wrap(err, "failed to get le creds from disk")
	}

	if diff := cmp.Diff(leCredsAfterRemove, leCredsBeforeRemove); diff == "" {
		return errors.Wrap(err, "LE cred not cleaned up successfully")
	}
	return nil
}

func ensurePINLockedOut(ctx context.Context, testUser string, cryptohomeClient *hwsec.CryptohomeClient) error {
	output, err := cryptohomeClient.ListAuthFactors(ctx, testUser)
	if err != nil {
		return errors.Wrap(err, "failed to list auth factors")
	}

	// Search for the first PIN-based AuthFactor, and parse if it is locked out.
	var pinAuthFactor *uda.AuthFactorWithStatus
	for _, authFactor := range output.ConfiguredAuthFactorsWithStatus {
		if authFactor.AuthFactor.Type == uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN {
			pinAuthFactor = authFactor
			break
		}
	}

	if pinAuthFactor != nil {
		for _, authIntent := range pinAuthFactor.AvailableForIntents {
			if authIntent == uda.AuthIntent_AUTH_INTENT_DECRYPT {
				return errors.New("PIN not locked when it should have been")
			}
		}

		var statusInfo = pinAuthFactor.StatusInfo
		if statusInfo.TimeAvailableIn != math.MaxUint64 {
			return errors.New("Pin not locked for indefinite amount of time, available in " + strconv.FormatUint(statusInfo.TimeAvailableIn, 10))
		}
		return nil
	}
	return errors.New(testUser + " does not have any PIN-based AuthFactors.")
}

// removeUser removes testUser and checks to see if the leCreds on disk was updated.
func removeUser(ctx, ctxForCleanUp context.Context, testUser string, r *hwsecremote.CmdRunnerRemote, helper *hwsecremote.CmdHelperRemote) error {
	cryptohomeHelper := helper.CryptohomeClient()

	leCredsBeforeRemove, err := getLeCredsFromDisk(ctx, r)
	if err != nil {
		return errors.Wrap(err, "failed to get le creds from disk")
	}

	if err := cryptohomeHelper.UnmountAndRemoveVault(ctx, testUser); err != nil {
		return errors.Wrap(err, "failed to remove the user")
	}

	leCredsAfterRemove, err := getLeCredsFromDisk(ctx, r)
	if err != nil {
		return errors.Wrap(err, "failed to get le creds from disk")
	}

	if diff := cmp.Diff(leCredsAfterRemove, leCredsBeforeRemove); diff == "" {
		return errors.Wrap(err, "LE cred not cleaned up successfully")
	}
	return nil
}
