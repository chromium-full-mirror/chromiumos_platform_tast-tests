// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	cpb "go.chromium.org/chromiumos/system_api/cryptohome_proto"
	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	cryptohomeWrappedKeysetString = "TPM_WRAPPED"
	shadowHome                    = "/home/.shadow"
)

// userHashRegexp extracts the hash from a cryptohome dir's path.
// Example: "/home/.shadow/118c4648065f5cd3660e17a53533ec7bc924d01f"
var userHashRegexp = regexp.MustCompile("^/home/user/([[:xdigit:]]+)$")

// parseDelimitedBinaryProtos parses the binaryMsg that is written in delimited-binary-protobuf format. These types of messages are formatted as |4-byte size||data||4-byte size||data|....
func parseDelimitedBinaryProtos(ctx context.Context, binaryMsg []byte) [][]byte {
	var res [][]byte
	var index, size uint32
	for index = 0; index+4 < uint32(len(binaryMsg)); index += size {
		size = binary.LittleEndian.Uint32(binaryMsg[index : index+4])
		index += 4
		res = append(res, binaryMsg[index:index+size])
	}
	return res
}

// UserDataAuthReplyWithError is an interface type that represent common UserDataAuth API protobuf reply that contains error in
// the form of CryptohomeErrorCode and CryptohomeErrorInfo.
type UserDataAuthReplyWithError interface {
	// GetError returns the legacy CryptohomeErrorCode.
	GetError() uda.CryptohomeErrorCode

	// GetErrorInfo() returns the CryptohomeErrorInfo struct that contains various error related info.
	GetErrorInfo() *uda.CryptohomeErrorInfo
}

// CryptohomeClient wraps and the functions of cryptohomeBinary and parses the outputs to
// structured data.
type CryptohomeClient struct {
	runner               CmdRunner
	binary               *cryptohomeBinary
	cryptohomePathBinary *cryptohomePathBinary
	daemonController     *DaemonController
}

// NewCryptohomeClient creates a new CryptohomeClient.
func NewCryptohomeClient(r CmdRunner) *CryptohomeClient {
	return &CryptohomeClient{
		runner:               r,
		binary:               newCryptohomeBinary(r),
		cryptohomePathBinary: newCryptohomePathBinary(r),
		daemonController:     NewDaemonController(r),
	}
}

// IsMounted checks if any vault is mounted.
func (u *CryptohomeClient) IsMounted(ctx context.Context) (bool, error) {
	out, err := u.binary.isMounted(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to check if mounted")
	}
	result, err := strconv.ParseBool(strings.TrimSuffix(string(out), "\n"))
	if err != nil {
		return false, errors.Wrap(err, "failed to parse output from cryptohome")
	}
	return result, nil
}

// Unmount unmounts the vault for username.
func (u *CryptohomeClient) Unmount(ctx context.Context, username string) (bool, error) {
	if err := u.UnmountAll(ctx); err != nil {
		return false, errors.Wrap(err, "failed to unmount all")
	}
	return true, nil
}

// UnmountAll unmounts all vault.
func (u *CryptohomeClient) UnmountAll(ctx context.Context) error {
	goal, _, _, err := u.daemonController.Status(ctx, UIDaemon)
	if err != nil {
		return errors.Wrap(err, "failed to get the status of ui")
	}
	if goal == startGoal {
		// Restart the UI to make sure nothing is still using cryptohome and unmount the mount point.
		if err := u.daemonController.Restart(ctx, UIDaemon); err != nil {
			return errors.Wrap(err, "failed to restart the UI")
		}
	} else {
		// Running the ui-post-stop directly if UI doesn't start.
		if _, err := u.runner.Run(ctx, "/usr/share/cros/init/ui-post-stop"); err != nil {
			return errors.Wrap(err, "failed to run ui-post-stop")
		}
	}
	return nil
}

// VaultConfig specifies the extra options to Mounting/Creating a vault.
type VaultConfig struct {
	// Ephemeral is set to true if the vault is ephemeral, that is, the vault is erased after the user logs out.
	Ephemeral bool

	// Ecryptfs is set to true if the vault should be backed by eCryptfs.
	Ecryptfs bool

	// CreateEmptyLabel is set to true if vault should be created with no label.
	CreateEmptyLabel bool

	// KioskUser is set to true if the vault should be mounted as a Vault.
	KioskUser bool
}

// NewVaultConfig creates a default vault config.
func NewVaultConfig() *VaultConfig {
	return &VaultConfig{}
}

const (
	// PassAuth is the constant for AuthConfig.AuthType, representing password authentication.
	PassAuth = iota
	// ChallengeAuth is the constant for AuthConfig.AuthType, representing challenge-response authenticating.
	ChallengeAuth = iota
)

// AuthConfig represents the data required to authenticate a user.
// It could be password authentication or challenge-response authentication.
type AuthConfig struct {
	// AuthType is the type of authentication.
	AuthType int

	// Username is the username for authentication.
	Username string

	// Password is the user's password.
	// Used only when AuthType is PassAuth
	Password string

	// KeyDelegateName is the dbus service name for the authentication delegate.
	// Used only when AuthType is ChallengeAuth
	KeyDelegateName string

	// KeyDelegatePath is the dbus service path for the authentication delegate.
	// Used only when AuthType is ChallengeAuth
	KeyDelegatePath string

	// ChallengeSPKI is the SPKI that contains the public key for challenge response. It's in DER format.
	// Used only when AuthType is ChallengeAuth
	ChallengeSPKI []byte

	// ChallengeAlgs is the list of cryptographic algorithms to use for challenge response.
	// Used only when AuthType is ChallengeAuth
	ChallengeAlgs []cpb.ChallengeSignatureAlgorithm
}

// NewPassAuthConfig creates an AuthConfig for Password Authentication.
func NewPassAuthConfig(username, password string) *AuthConfig {
	config := &AuthConfig{}
	config.AuthType = PassAuth
	config.Username = username
	config.Password = password
	return config
}

// NewChallengeAuthConfig creates an AuthConfig for Challenge-Response Authentication.
func NewChallengeAuthConfig(username, keyDelegateName, keyDelegatePath string, challengeSPKI []byte, challengeAlgs []cpb.ChallengeSignatureAlgorithm) *AuthConfig {
	config := &AuthConfig{}
	config.AuthType = ChallengeAuth
	config.Username = username
	config.KeyDelegateName = keyDelegateName
	config.KeyDelegatePath = keyDelegatePath
	config.ChallengeSPKI = challengeSPKI
	config.ChallengeAlgs = challengeAlgs
	return config
}

// authConfigToExtraFlags converts AuthConfig to flags accepted by the cryptohome command line.
func authConfigToExtraFlags(config *AuthConfig) []string {
	var extraFlags []string
	if config.AuthType == PassAuth {
		extraFlags = append(extraFlags, "--password="+config.Password)
	} else if config.AuthType == ChallengeAuth {
		var algs = []string{}
		for _, a := range config.ChallengeAlgs {
			algs = append(algs, a.String())
		}
		extraFlags = append(extraFlags, "--challenge_alg="+strings.Join(algs, ","))
		extraFlags = append(extraFlags, "--challenge_spki="+hex.EncodeToString(config.ChallengeSPKI))
		extraFlags = append(extraFlags, "--key_delegate_name="+config.KeyDelegateName)
		extraFlags = append(extraFlags, "--key_delegate_path="+config.KeyDelegatePath)
	} else {
		panic("Invalid AuthType in CryptohomeClient")
	}
	return extraFlags
}

func (u *CryptohomeClient) mountVaultWithAuthFactor(ctx context.Context, label string, authConfig *AuthConfig, create bool, vaultConfig *VaultConfig) error {
	// Start an Auth session and get an authSessionID.
	_, authSessionID, err := u.StartAuthSession(ctx, authConfig.Username, vaultConfig.Ephemeral, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start Auth session")
	}
	// PrepareEphemeralVault handles both authentication and vault preparation for an ephemeral vault.
	if vaultConfig.Ephemeral {
		if err := u.PrepareEphemeralVault(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to prepare ephemeral vault")
		}
		return nil
	}

	if create {
		if err := u.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if err := u.AddAuthFactor(ctx, authSessionID, label, authConfig.Password); err != nil {
			return errors.Wrap(err, "failed to add credentials with AuthSession")
		}
	} else {
		// Authenticate the same AuthSession using authSessionID.
		// If we cannot authenticate, do not proceed with mount and unmount.
		if _, err := u.AuthenticateAuthFactor(ctx, authSessionID, label, authConfig.Password); err != nil {
			return errors.Wrap(err, "failed to authenticate AuthFactor")
		}
	}
	if _, err := u.PreparePersistentVault(ctx, authSessionID, vaultConfig.Ecryptfs); err != nil {
		return errors.Wrap(err, "failed to prepare persistent vault")
	}
	return nil
}

// MountVault mounts the vault for username; creates a new vault if no vault yet if create is true. error is nil if the operation completed successfully.
func (u *CryptohomeClient) MountVault(ctx context.Context, label string, authConfig *AuthConfig, create bool, vaultConfig *VaultConfig) error {
	return u.mountVaultWithAuthFactor(ctx, label, authConfig, create, vaultConfig)
}

// MountGuest creates a mount point for a guest user; error is nil if the operation completed successfully.
func (u *CryptohomeClient) MountGuest(ctx context.Context) error {
	if _, err := u.binary.prepareGuestVault(ctx); err != nil {
		return errors.Wrap(err, "failed to mount guest")
	}
	return nil
}

// GetSanitizedUsername computes the sanitized username for the given user.
// If useDBus is true, the sanitized username will be computed by cryptohome (through dbus). Otherwise, it'll be computed directly by libbrillo (without dbus).
func (u *CryptohomeClient) GetSanitizedUsername(ctx context.Context, username string, useDBus bool) (string, error) {
	out, err := u.binary.getSanitizedUsername(ctx, username, useDBus)
	if err != nil {
		return "", errors.Wrap(err, "failed to call cryptohomeBinary.GetSanitizedUsername")
	}
	outs := strings.TrimSpace(string(out))
	exp := regexp.MustCompile("^[a-f0-9]{40}$")
	// A proper sanitized username should be a hex string of length 40.
	if !exp.MatchString(outs) {
		return "", errors.Errorf("invalid sanitized username %q", outs)
	}
	return outs, nil
}

// GetSystemSalt retrieves the system salt and return the hex encoded version of it.
// If useDBus is true, the system salt will be retrieved from cryptohome (through dbus). Otherwise, it'll be loaded directly by libbrillo (without dbus).
func (u *CryptohomeClient) GetSystemSalt(ctx context.Context, useDBus bool) (string, error) {
	out, err := u.binary.getSystemSalt(ctx, useDBus)
	if err != nil {
		return "", errors.Wrap(err, "failed to call cryptohomeBinary.GetSystemSalt")
	}
	outs := strings.TrimSpace(string(out))
	exp := regexp.MustCompile("^[a-f0-9]+$")
	// System salt should be a non-empty hex string.
	if !exp.MatchString(outs) {
		return "", errors.Errorf("invalid system salt %q", outs)
	}
	return outs, nil
}

func (u *CryptohomeClient) checkVaultWithAuthFactor(ctx context.Context, label string, authConfig *AuthConfig, pinAuth bool) (bool, error) {
	// Start an Auth session and get an authSessionID.
	_, authSessionID, err := u.StartAuthSession(ctx, authConfig.Username, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY)
	if err != nil {
		return false, errors.Wrap(err, "failed to start Auth session")
	}
	defer u.InvalidateAuthSession(ctx, authSessionID)

	var result *uda.AuthenticateAuthFactorReply
	var expectedResult []uda.AuthIntent
	if pinAuth {
		// Lightweight Authentication does not exist for PIN AuthFactors - should go through full decryption in AuthenticateAuthFactor.
		result, err = u.AuthenticatePinAuthFactor(ctx, authSessionID, label, authConfig.Password)
		expectedResult = []uda.AuthIntent{uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, uda.AuthIntent_AUTH_INTENT_DECRYPT}
	} else {
		result, err = u.AuthenticateAuthFactor(ctx, authSessionID, label, authConfig.Password)
		expectedResult = []uda.AuthIntent{uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY}
	}
	if err != nil {
		return false, errors.Wrap(err, "failed to authenticate AuthFactor")
	}
	// Check that reply authenticatied with correct AuthIntent.
	less := func(a, b uda.AuthIntent) bool { return a < b }
	diff := cmp.Diff(result.AuthProperties.AuthorizedFor, expectedResult, cmpopts.SortSlices(less))
	if diff != "" {
		return false, errors.Errorf("authenticated with incorrect AuthIntent: %q", result.AuthProperties.AuthorizedFor)
	}
	return true, nil
}

// CheckVault checks the vault via AuthenticateAuthFactor, using lightweight verification.
func (u *CryptohomeClient) CheckVault(ctx context.Context, label string, authConfig *AuthConfig) (bool, error) {
	return u.checkVaultWithAuthFactor(ctx, label, authConfig, false /*pinAuth*/)
}

// CheckPinVault checks the vault via AuthenticatePinAuthFactor, using lightweight verification.
func (u *CryptohomeClient) CheckPinVault(ctx context.Context, label string, authConfig *AuthConfig) (bool, error) {
	return u.checkVaultWithAuthFactor(ctx, label, authConfig, true /*pinAuth*/)
}

// RemoveVault remove the vault for username.
func (u *CryptohomeClient) RemoveVault(ctx context.Context, username string) (bool, error) {
	out, err := u.binary.remove(ctx, username)
	if err != nil {
		return false, errors.Wrapf(err, "failed to remove vault, got: %s", string(out))
	}
	return true, nil
}

// UnmountAndRemoveVault attempts to unmount all vaults and remove the vault for username.
// This is a simple helper, and it's created because this is a commonly used combination.
func (u *CryptohomeClient) UnmountAndRemoveVault(ctx context.Context, username string) error {
	// Note: Vault must be unmounted to be removed.
	if err := u.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount all")
	}

	if _, err := u.RemoveVault(ctx, username); err != nil {
		return errors.Wrap(err, "failed to remove vault post unmount")
	}

	return nil
}

// LockToSingleUserMountUntilReboot will block users other than the specified from logging in if the call succeeds, and in that case, nil is returned.
func (u *CryptohomeClient) LockToSingleUserMountUntilReboot(ctx context.Context, username string) error {
	const successMessage = "Login disabled."
	binaryOutput, err := u.binary.lockToSingleUserMountUntilReboot(ctx, username)
	if err != nil {
		return errors.Wrap(err, "failed to call lock_to_single_user_mount_until_reboot")
	}
	output := strings.TrimSuffix(string(binaryOutput), "\n")
	// Note that we are checking the output message again because we want to
	// catch cases that return 0 but wasn't successful.
	if !strings.Contains(output, successMessage) {
		return errors.Errorf("incorrect message from LockToSingleUserMountUntilReboot; got %q, want %q", output, successMessage)
	}
	return nil
}

// IsTPMWrappedKeySet checks if the current user vault is TPM-backed.
func (u *CryptohomeClient) IsTPMWrappedKeySet(ctx context.Context, username string) (bool, error) {
	out, err := u.binary.dumpKeyset(ctx, username)
	if err != nil {
		return false, errors.Wrap(err, "failed to dump keyset")
	}
	return strings.Contains(string(out), cryptohomeWrappedKeysetString), nil
}

// CheckTPMWrappedUserKeyset checks if the given user's keyset is backed by TPM.
// Returns an error if the keyset is not TPM-backed or if there's anything wrong.
func (u *CryptohomeClient) CheckTPMWrappedUserKeyset(ctx context.Context, user string) error {
	if keysetTPMBacked, err := u.IsTPMWrappedKeySet(ctx, user); err != nil {
		return errors.Wrap(err, "failed to check user keyset")
	} else if !keysetTPMBacked {
		return errors.New("user keyset is not TPM-backed")
	}

	return nil
}

// parseTokenStatus parse the output of cryptohome --action=pkcs11_system_token_status or cryptohome --action=pkcs11_token_status and return the label, pin, slot and error (in that order).
func parseTokenStatus(cmdOutput string) (returnedLabel, returnedPin string, returnedSlot int, returnedErr error) {
	arr := strings.Split(cmdOutput, "\n")

	labels := []string{"Label", "Pin", "Slot"}
	params := make(map[string]string)
	for _, str := range arr {
		for _, label := range labels {
			labelPrefix := label + " = "
			if strings.HasPrefix(str, labelPrefix) {
				params[label] = str[len(labelPrefix):]
			}
		}
	}

	// Check that we've got all the parameters
	if len(params) != len(labels) {
		return "", "", -1, errors.Errorf("missing parameters in token status output, got: %v", params)
	}

	// Slot should be an integer
	slot, err := strconv.Atoi(params["Slot"])
	if err != nil {
		return "", "", -1, errors.Wrap(err, "token slot not integer")
	}

	// Fill up the return values
	return params["Label"], params["Pin"], slot, nil
}

// GetTokenInfoForUser retrieve the token label, pin and slot for the user token if username is non-empty, or system token if username is empty.
func (u *CryptohomeClient) GetTokenInfoForUser(ctx context.Context, username string) (returnedLabel, returnedPin string, returnedSlot int, returnedErr error) {
	cmdOutput := ""
	if username == "" {
		// We want the system token.
		out, err := u.binary.pkcs11SystemTokenInfo(ctx)
		cmdOutput = string(out)
		if err != nil {
			return "", "", -1, errors.Wrapf(err, "failed to get system token info %q", cmdOutput)
		}
	} else {
		// We want the user token.
		out, err := u.binary.pkcs11UserTokenInfo(ctx, username)
		cmdOutput = string(out)
		if err != nil {
			return "", "", -1, errors.Wrapf(err, "failed to get user token info %q", cmdOutput)
		}
	}
	label, pin, slot, err := parseTokenStatus(cmdOutput)
	if err != nil {
		return "", "", -1, errors.Wrapf(err, "failed to parse token status %q", cmdOutput)
	}
	return label, pin, slot, nil
}

// GetTokenForUser retrieve the token slot for the user token if username is non-empty, or system token if username is empty.
func (u *CryptohomeClient) GetTokenForUser(ctx context.Context, username string) (int, error) {
	_, _, slot, err := u.GetTokenInfoForUser(ctx, username)
	return slot, err
}

// WaitForUserToken wait until the user token for the specified user is ready. Otherwise, return an error if the token is still unavailable.
func (u *CryptohomeClient) WaitForUserToken(ctx context.Context, username string) error {
	const waitForUserTokenTimeout = 15 * time.Second

	if username == "" {
		// This method is for user token, not system token.
		// Note: For those who want to wait for system token, system token is always ready, no need to wait.
		return errors.New("empty username in WaitForUserToken")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		slot, err := u.GetTokenForUser(ctx, username)
		if err != nil {
			// This is unexpected and shouldn't usually happen.
			return testing.PollBreak(errors.Wrapf(err, "failed to get slot ID for username %q", username))
		}
		if slot <= 0 {
			return errors.Wrapf(err, "invalid slot ID %d for username %q", slot, username)
		}
		return nil
	}, &testing.PollOptions{Timeout: waitForUserTokenTimeout}); err != nil {
		return errors.Wrap(err, "failed waiting for user token")
	}
	return nil
}

// FWMPError is a custom error type that conveys the error as well as parsed
// ErrorCode from cryptohome API.
type FWMPError struct {
	*errors.E

	// ErrorCode is the error code from FWMP methods.
	ErrorCode string
}

// GetAccountDiskUsage returns the disk space (in bytes) used by the username.
func (u *CryptohomeClient) GetAccountDiskUsage(ctx context.Context, username string) (diskUsage int64, returnedError error) {
	binaryMsg, err := u.binary.getAccountDiskUsage(ctx, username)
	msg := string(binaryMsg)
	if err != nil {
		testing.ContextLogf(ctx, "Failure to call GetAccountDiskUsage, got %q", msg)
		return -1, errors.Wrap(err, "failed to call GetAccountDiskUsage")
	}

	var result int64
	for _, s := range strings.Split(strings.TrimSpace(msg), "\n") {
		if n, err := fmt.Sscanf(s, "Account Disk Usage in bytes: %d", &result); err == nil && n == 1 {
			// We've found the output that we need.
			return result, nil
		}
	}

	testing.ContextLogf(ctx, "Unexpected GetAccountDiskUsage output, got %q", msg)
	return -1, errors.New("failed to parse GetAccountDiskUsage output")
}

// GetHomeUserPath retrieves the user home path, which contains a salted hash of the username.
func (u *CryptohomeClient) GetHomeUserPath(ctx context.Context, username string) (string, error) {
	binaryMsg, err := u.cryptohomePathBinary.userPath(ctx, username)
	msg := string(binaryMsg)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to call cryptohome-path user, got %q", msg)
		return "", errors.Wrap(err, "failed to call cryptohome-path user")
	}
	return strings.TrimSpace(msg), nil
}

// GetRootUserPath retrieves the user root path, which contains a salted hash of the username.
func (u *CryptohomeClient) GetRootUserPath(ctx context.Context, username string) (string, error) {
	binaryMsg, err := u.cryptohomePathBinary.systemPath(ctx, username)
	msg := string(binaryMsg)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to call cryptohome-path system, got %q", msg)
		return "", errors.Wrap(err, "failed to call cryptohome-path system")
	}
	return strings.TrimSpace(msg), nil
}

// GetUserHash returns user's cryptohome hash.
func (u *CryptohomeClient) GetUserHash(ctx context.Context, username string) (string, error) {
	binaryMsg, err := u.cryptohomePathBinary.userPath(ctx, username)
	msg := string(binaryMsg)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to call cryptohome-path user, got %q", msg)
		return "", errors.Wrap(err, "failed to call cryptohome-path user")
	}
	p := strings.TrimSpace(msg)
	m := userHashRegexp.FindStringSubmatch(p)
	if m == nil {
		return "", errors.Errorf("didn't find hash in path %q", p)
	}
	return m[1], nil
}

// GetUserShadowRoot returns the shadow root of the user.
func (u *CryptohomeClient) GetUserShadowRoot(ctx context.Context, username string) (string, error) {
	hash, err := u.GetUserHash(ctx, username)
	if err != nil {
		return "", errors.Wrap(err, "failed to retrieve the user hash")
	}
	return filepath.Join(shadowHome, hash), nil
}

// SupportsLECredentials calls GetSupportedKeyPolicies and parses the output for low entropy credential support.
func (u *CryptohomeClient) SupportsLECredentials(ctx context.Context) (bool, error) {
	binaryMsg, err := u.binary.getSupportedKeyPolicies(ctx)
	if err != nil {
		return false, errors.Wrap(err, "GetSupportedKeyPolicies failed")
	}

	// TODO(crbug.com/1187192): Parsing human readable output is error prone.
	// Parse the text output which looks something like:
	// low_entropy_credentials_supported: true
	// GetSupportedKeyPolicies success.
	return strings.Contains(string(binaryMsg), "low_entropy_credentials_supported: true"), nil
}

// StartAuthSession starts an AuthSession for a user, returning both the StartAuthSessionReply proto and the generated Auth Session ID.
func (u *CryptohomeClient) StartAuthSession(ctx context.Context, user string, isEphemeral bool, authIntent uda.AuthIntent) (*uda.StartAuthSessionReply, string, error) {
	reply := &uda.StartAuthSessionReply{}
	binaryMsg, err := u.binary.startAuthSession(ctx, user, isEphemeral, authIntent)
	if err != nil {
		return reply, "", err
	}

	if err := proto.Unmarshal(binaryMsg, reply); err != nil {
		return reply, "", err
	}

	authSessionID := reply.AuthSessionId
	if authSessionID == nil {
		return reply, "", errors.New("didn't find auth session in output")
	}

	return reply, hex.EncodeToString(authSessionID), nil
}

// AuthenticateAuthFactor authenticates an AuthSession with a given authSessionID via an auth factor.
func (u *CryptohomeClient) AuthenticateAuthFactor(ctx context.Context, authSessionID, label, password string) (*uda.AuthenticateAuthFactorReply, error) {
	binaryMsg, err := u.binary.authenticateAuthFactor(ctx, authSessionID, label, password)

	// Attempt to parse the binaryMsg anyway, we need them to check for the correct error code.
	reply := &uda.AuthenticateAuthFactorReply{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthenticateAuthFactor reply")
	}

	if err != nil {
		return reply, errors.Wrap(err, "AuthenticateAuthFactor failed")
	}
	return reply, nil
}

// RemoveAuthFactor removes an auth factor with provided label.
func (u *CryptohomeClient) RemoveAuthFactor(ctx context.Context, authSessionID, label string) error {
	_, err := u.binary.removeAuthFactor(ctx, authSessionID, label)
	return err
}

// FetchStatusUpdateSignal fetches the status update signal that gets sent priodically after a user is locked out of an auth factor.
func (u *CryptohomeClient) FetchStatusUpdateSignal(ctx context.Context, broadcastID []byte) (*uda.AuthFactorStatusUpdate, error) {
	binaryMsg, err := u.binary.fetchStatusUpdateSignal(ctx, broadcastID)
	// Unmarshall the reply even if there was an error.
	reply := &uda.AuthFactorStatusUpdate{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthFactorStatusUpdate reply")
	}
	if err != nil {
		return reply, errors.Wrap(err, "failed to receive AuthFactorStatusUpdate Signal")
	}
	if !bytes.Equal(reply.BroadcastId, broadcastID) {
		return nil, errors.Wrap(err, "the broadcast id doesn't match between auth_session and AuthFactorStatusUpdateSignal")
	}
	return reply, nil
}

// StartAuthSessionWithStatusUpdate starts an AuthSession for a user and fetches the StatusUpdate signal that is sent. It returns the StartAuthSessionReply proto and the
// generated Auth Session ID as well as the AuthFactorStatusUpdate.
func (u *CryptohomeClient) StartAuthSessionWithStatusUpdate(ctx context.Context, user string, isEphemeral bool, authIntent uda.AuthIntent) (*uda.StartAuthSessionReply, *uda.AuthFactorStatusUpdate, error) {
	startAuthSessionReply := &uda.StartAuthSessionReply{}
	statusUpdateReply := &uda.AuthFactorStatusUpdate{}
	binaryMsg, err := u.binary.startAuthSessionWithStatusUpdate(ctx, user, isEphemeral, authIntent)
	replies := parseDelimitedBinaryProtos(ctx, binaryMsg)
	if len(replies) < 2 {
		return nil, nil, errors.New("only one reply was found while two was expected, either the signal was not received or the reply could not be parsed")
	}
	if err != nil {
		return nil, nil, err
	}
	if err := proto.Unmarshal(replies[0], startAuthSessionReply); err != nil {
		return nil, nil, err
	}
	authSessionID := startAuthSessionReply.AuthSessionId
	if authSessionID == nil {
		return nil, nil, errors.New("didn't find auth session in output")
	}
	if unmarshErr := proto.Unmarshal(replies[1], statusUpdateReply); unmarshErr != nil {
		return startAuthSessionReply, nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthFactorStatusUpdate reply")
	}
	if !bytes.Equal(statusUpdateReply.BroadcastId, startAuthSessionReply.BroadcastId) {
		return startAuthSessionReply, nil, errors.Wrap(err, "the broadcast id doesn't match between auth_session and AuthFactorStatusUpdateSignal")
	}
	return startAuthSessionReply, statusUpdateReply, nil
}

// FailAuthenticatePinAuthFactorAndFetchStatusUpdate authenticates an AuthSession with a given authSessionID via pin and intercepts the AuthFactorStatusUpdate signal.
// As we are intending to catch the status update signal, we intend the authentication to fail. So on a failed authentication, we will return nil, because that is the expected
// behavior. We will return error if the output could not be unmarshalled or if the status update signal is not fetched properly or if the authentication has succeeded.
func (u *CryptohomeClient) FailAuthenticatePinAuthFactorAndFetchStatusUpdate(ctx context.Context, authSessionID, label, pin string, broadcastID []byte) (*uda.AuthFactorStatusUpdate, error) {

	binaryMsg, err := u.binary.authenticatePinAuthFactorWithStatusUpdate(ctx, authSessionID, label, pin, broadcastID)
	replies := parseDelimitedBinaryProtos(ctx, binaryMsg)
	if len(replies) < 2 {
		return nil, errors.New("Only one reply was found while two was expected, either the signal was not received or the reply couldn't be parsed")
	}
	authenticateReply := &uda.AuthenticateAuthFactorReply{}
	if unmarshErr := proto.Unmarshal(replies[0], authenticateReply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthenticateAuthFactor reply")
	}
	statusUpdateReply := &uda.AuthFactorStatusUpdate{}
	if unmarshErr := proto.Unmarshal(replies[1], statusUpdateReply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthFactorStatusUpdate reply")
	}
	if !bytes.Equal(statusUpdateReply.BroadcastId, broadcastID) {
		return nil, errors.Wrap(err, "the broadcast id doesn't match between auth_session and AuthFactorStatusUpdateSignal")
	}
	if err == nil {
		return nil, errors.Wrap(err, "the authentication was expeceted to fail but it succeeded")
	}
	return statusUpdateReply, nil
}

// AuthenticatePinAuthFactor authenticates an AuthSession with a given authSessionID via pin.
func (u *CryptohomeClient) AuthenticatePinAuthFactor(ctx context.Context, authSessionID, label, pin string) (*uda.AuthenticateAuthFactorReply, error) {
	binaryMsg, err := u.binary.authenticatePinAuthFactor(ctx, authSessionID, label, pin)

	// Unmarshal proto first, even if there was an error.
	reply := &uda.AuthenticateAuthFactorReply{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthenticateAuthFactor reply")
	}
	if err != nil {
		return reply, errors.Wrap(err, "AuthenticateAuthFactor failed")
	}

	return reply, nil
}

// AuthenticateKioskAuthFactor authenticates an AuthSession with a given authSessionID via a kiosk authfactor.
func (u *CryptohomeClient) AuthenticateKioskAuthFactor(ctx context.Context, authSessionID string) error {
	_, err := u.binary.authenticateKioskAuthFactor(ctx, authSessionID, "public_mount")
	return err
}

// AuthenticateKioskAuthFactorWithLabel authenticates an AuthSession with a given authSessionID via a
// kiosk factor with a specific label. You usually would prefer to use the unlabelled version but
// legacy kiosks can require a non-standard label.
func (u *CryptohomeClient) AuthenticateKioskAuthFactorWithLabel(ctx context.Context, authSessionID, label string) error {
	_, err := u.binary.authenticateKioskAuthFactor(ctx, authSessionID, label)
	return err
}

// AuthenticateRecoveryAuthFactor authenticates an AuthSession with a given authSessionID via recovery auth factor.
func (u *CryptohomeClient) AuthenticateRecoveryAuthFactor(ctx context.Context, authSessionID, label, epochResponseHex,
	recoveryResponseHex, recoveryLedgerName, recoveryLedgerPubKeyHash, recoveryLedgerPubKey string) error {
	_, err := u.binary.authenticateRecoveryAuthFactor(ctx, authSessionID, label, epochResponseHex,
		recoveryResponseHex, recoveryLedgerName, recoveryLedgerPubKeyHash, recoveryLedgerPubKey)
	return err
}

// FetchRecoveryIDs authenticates an AuthSession with a given authSessionID via recovery auth factor.
func (u *CryptohomeClient) FetchRecoveryIDs(ctx context.Context, username, label string) ([]string, error) {
	binaryMsg, err := u.binary.fetchRecoveryIDs(ctx, username, label)

	// Unmarshal proto first, even if there was an error.
	reply := &uda.RecoveryExtendedInfoReply{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal RecoveryExtendedInfoReply")
	}
	if err != nil {
		return reply.RecoveryIds, errors.Wrap(err, "FetchRecoveryIDs failed")
	}

	return reply.RecoveryIds, nil
}

// AuthenticateSmartCardAuthFactor authenticates an AuthSession with a given authSessionID via smart card.
func (u *CryptohomeClient) AuthenticateSmartCardAuthFactor(ctx context.Context, authSessionID, label string, authConfig *AuthConfig) (*uda.AuthenticateAuthFactorReply, error) {
	extraFlags := authConfigToExtraFlags(authConfig)
	binaryMsg, err := u.binary.authenticateSmartCardAuthFactor(ctx, authSessionID, label, extraFlags)

	// Unmarshal proto first, even if there was an error.
	reply := &uda.AuthenticateAuthFactorReply{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthenticateAuthFactor reply")
	}
	if err != nil {
		return reply, errors.Wrap(err, "AuthenticateAuthFactor failed")
	}

	return reply, nil
}

// AuthenticateFingerprintAuthFactor authenticates an AuthSession with a given authSessionID via fingerprint.
func (u *CryptohomeClient) AuthenticateFingerprintAuthFactor(ctx context.Context, authSessionID string, labels []string) (*uda.AuthenticateAuthFactorReply, error) {
	binaryMsg, err := u.binary.authenticateFingerprintAuthFactor(ctx, authSessionID, labels)

	// Unmarshal proto first, even if there was an error.
	reply := &uda.AuthenticateAuthFactorReply{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal AuthenticateAuthFactor reply")
	}
	if err != nil {
		return reply, errors.Wrap(err, "AuthenticateAuthFactor failed")
	}

	return reply, nil
}

// AddAuthFactor creates an auth factor for the user with given password.
func (u *CryptohomeClient) AddAuthFactor(ctx context.Context, authSessionID, label, password string) error {
	_, err := u.binary.addAuthFactor(ctx, authSessionID, label, password, nil)
	return err
}

// AddPinAuthFactor creates a pin auth factor for the user.
func (u *CryptohomeClient) AddPinAuthFactor(ctx context.Context, authSessionID, label, pin string) error {
	_, err := u.binary.addPinAuthFactor(ctx, authSessionID, label, pin, nil)
	return err
}

// AddModernPinAuthFactor creates a modern pin auth factor for the user.
func (u *CryptohomeClient) AddModernPinAuthFactor(ctx context.Context, authSessionID, label, pin string) error {
	_, err := u.binary.addModernPinAuthFactor(ctx, authSessionID, label, pin)
	return err
}

// AddFingerprintAuthFactor creates a fingerprint auth factor for the user.
func (u *CryptohomeClient) AddFingerprintAuthFactor(ctx context.Context, authSessionID, label string) error {
	_, err := u.binary.addFingerprintAuthFactor(ctx, authSessionID, label)
	return err
}

// AddRecoveryAuthFactor creates a recovery auth factor for the user.
func (u *CryptohomeClient) AddRecoveryAuthFactor(ctx context.Context, authSessionID, label, mediatorPubKeyHex, userGaiaID, deviceUserID string) error {
	_, err := u.binary.addRecoveryAuthFactor(ctx, authSessionID, label, mediatorPubKeyHex, userGaiaID, deviceUserID)
	return err
}

// AddKioskAuthFactor creates an auth factor for kiosk user.
func (u *CryptohomeClient) AddKioskAuthFactor(ctx context.Context, authSessionID string) error {
	_, err := u.binary.addKioskAuthFactor(ctx, authSessionID)
	return err
}

// AddSmartCardAuthFactor creates an auth factor for the user with given smart card.
func (u *CryptohomeClient) AddSmartCardAuthFactor(ctx context.Context, authSessionID, label string, authConfig *AuthConfig) error {
	extraFlags := authConfigToExtraFlags(authConfig)
	_, err := u.binary.addSmartCardAuthFactor(ctx, authSessionID, label, extraFlags)
	return err
}

// UpdatePasswordAuthFactor updates an auth factor for the user with given password.
func (u *CryptohomeClient) UpdatePasswordAuthFactor(ctx context.Context, authSessionID, label, password string) error {
	_, err := u.binary.updatePasswordAuthFactor(ctx, authSessionID, label, password, nil)
	return err
}

// UpdateRecoveryAuthFactor updates the recovery auth factor for the user.
func (u *CryptohomeClient) UpdateRecoveryAuthFactor(
	ctx context.Context,
	authSessionID, label, mediatorPubKeyHex, userGaiaID, deviceUserID string,
	ensureFreshRecoveryID bool) error {
	_, err := u.binary.updateRecoveryAuthFactor(ctx, authSessionID, label, mediatorPubKeyHex, userGaiaID, deviceUserID, ensureFreshRecoveryID)
	return err
}

// LockRecoveryFactorUntilReboot locks the recovery auth factor for all users for the authentication operation until system reboots.
// Remove the flag file `/run/cryptohome/crd_detected_on_login_screen` after calling this function.
func (u *CryptohomeClient) LockRecoveryFactorUntilReboot(
	ctx context.Context) error {
	_, err := u.binary.lockRecoveryFactorUntilReboot(ctx)
	return err
}

// UpdatePinAuthFactor updates the pin auth factor for the user.
func (u *CryptohomeClient) UpdatePinAuthFactor(ctx context.Context, authSessionID, label, pin string) error {
	_, err := u.binary.updatePinAuthFactor(ctx, authSessionID, label, pin)
	return err
}

// RelabelAuthFactor relabels an auth factor for the user.
func (u *CryptohomeClient) RelabelAuthFactor(ctx context.Context, authSessionID, label, newLabel string) error {
	_, err := u.binary.relabelAuthFactor(ctx, authSessionID, label, newLabel)
	return err
}

// ReplacePasswordAuthFactor replace an existing auth factor with a new one with given password.
func (u *CryptohomeClient) ReplacePasswordAuthFactor(ctx context.Context, authSessionID, label, newKeyLabel, password string) error {
	_, err := u.binary.replacePasswordAuthFactor(ctx, authSessionID, label, newKeyLabel, password, nil)
	return err
}

// PrepareGuestVault prepares vault for guest session.
func (u *CryptohomeClient) PrepareGuestVault(ctx context.Context) (*uda.PrepareGuestVaultReply, error) {
	binaryMsg, err := u.binary.prepareGuestVault(ctx)

	// Attempt to parse the binaryMsg anyway, we need them to check for the correct error code.
	reply := &uda.PrepareGuestVaultReply{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal PrepareGuestVault reply")
	}

	if err != nil {
		return reply, errors.Wrap(err, "PrepareGuestVault failed")
	}

	return reply, nil
}

// PrepareEphemeralVault prepares vault for ephemeral session.
func (u *CryptohomeClient) PrepareEphemeralVault(ctx context.Context, authSessionID string) error {
	_, err := u.binary.prepareEphemeralVault(ctx, authSessionID)
	return err
}

// PreparePersistentVault prepares vault for persistent user session, the reply is returned as long as they are available, even if the underlying operation failed. The returned error is nil iff the operation is successful.
func (u *CryptohomeClient) PreparePersistentVault(ctx context.Context, authSessionID string, ecryptfs bool) (*uda.PreparePersistentVaultReply, error) {
	binaryMsg, err := u.binary.preparePersistentVault(ctx, authSessionID, ecryptfs)

	// Attempt to parse the binaryMsg anyway, we need them to check for the correct error code.
	reply := &uda.PreparePersistentVaultReply{}
	if err := proto.Unmarshal(binaryMsg, reply); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal PreparePersistentVault reply")
	}

	if err != nil {
		return reply, errors.Wrap(err, "PreparePersistentVault failed")
	}

	return reply, nil
}

// PrepareVaultForMigration prepares vault for migration.
func (u *CryptohomeClient) PrepareVaultForMigration(ctx context.Context, authSessionID string) error {
	_, err := u.binary.prepareVaultForMigration(ctx, authSessionID)
	return err
}

// CreatePersistentUser creates persistent user.
func (u *CryptohomeClient) CreatePersistentUser(ctx context.Context, authSessionID string) error {
	_, err := u.binary.createPersistentUser(ctx, authSessionID)
	return err
}

// MigrateToDircrypto migrates vault to dircrypto. Must be mounted for migration first.
func (u *CryptohomeClient) MigrateToDircrypto(ctx context.Context, userName string) error {
	_, err := u.binary.migrateToDircrypto(ctx, userName)
	return err
}

// InvalidateAuthSession invalidates a user with AuthSessionID.
func (u *CryptohomeClient) InvalidateAuthSession(ctx context.Context, authSessionID string) error {
	_, err := u.binary.invalidateAuthSession(ctx, authSessionID)
	return err
}

// CleanupSession invalidates a user with AuthSessionID and unmount user home directories and daemon-stores.
func (u *CryptohomeClient) CleanupSession(ctx context.Context, authSessionID string) error {
	// Kill the AuthSession.
	if _, err := u.binary.invalidateAuthSession(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to invaldiate AuthSession")
	}
	// Clean up obsolete state, in case there's any.
	if err := u.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount vaults for cleanup")
	}
	return nil
}

// ListAuthFactors lists the auth factors for a given user.
func (u *CryptohomeClient) ListAuthFactors(ctx context.Context, user string) (*uda.ListAuthFactorsReply, error) {
	reply := &uda.ListAuthFactorsReply{}

	binaryMsg, err := u.binary.listAuthFactors(ctx, user)
	if err != nil {
		return reply, errors.Wrap(err, "ListAuthFactors failed")
	}
	if err := proto.Unmarshal(binaryMsg, reply); err != nil {
		return reply, errors.Wrap(err, "failed to unmarshal ListAuthFactors reply")
	}

	return reply, nil
}

// PrepareAddFpAuthFactor prepares adding the fingerprint auth factor.
func (u *CryptohomeClient) PrepareAddFpAuthFactor(ctx context.Context, authSessionID string) (*uda.PrepareAuthFactorReply, error) {
	reply := &uda.PrepareAuthFactorReply{}

	binaryMsg, err := u.binary.prepareAddFpAuthFactor(ctx, authSessionID)
	if err != nil {
		return reply, errors.Wrap(err, "PrepareAuthFactor failed")
	}
	if err := proto.Unmarshal(binaryMsg, reply); err != nil {
		return reply, errors.Wrap(err, "failed to unmarshal PrepareAuthFactor reply")
	}

	return reply, nil
}

// PrepareAuthFpAuthFactor prepares authenticating the fingerprint auth factor.
func (u *CryptohomeClient) PrepareAuthFpAuthFactor(ctx context.Context, authSessionID string) (*uda.PrepareAuthFactorReply, error) {
	reply := &uda.PrepareAuthFactorReply{}

	binaryMsg, err := u.binary.prepareAuthFpAuthFactor(ctx, authSessionID)
	if err != nil {
		return reply, errors.Wrap(err, "PrepareAuthFactor failed")
	}
	if err := proto.Unmarshal(binaryMsg, reply); err != nil {
		return reply, errors.Wrap(err, "failed to unmarshal PrepareAuthFactor reply")
	}

	return reply, nil
}

// PrepareRecoveryAuthFactor creates recovery request, returns the value of the request.
func (u *CryptohomeClient) PrepareRecoveryAuthFactor(ctx context.Context, authSessionID, label, epochResponseHex string) (*uda.CryptohomeRecoveryPrepareOutput, error) {
	reply := &uda.PrepareAuthFactorReply{}

	binaryMsg, err := u.binary.prepareRecoveryAuthFactor(ctx, authSessionID, label, epochResponseHex)
	if err != nil {
		return nil, errors.Wrap(err, "PrepareAuthFactor for recovery failed")
	}
	if err := proto.Unmarshal(binaryMsg, reply); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal PrepareAuthFactor reply")
	}
	switch reply.GetPrepareOutput().Output.(type) {
	case *uda.PrepareOutput_CryptohomeRecoveryOutput:
		return reply.PrepareOutput.GetCryptohomeRecoveryOutput(), nil
	default:
		return nil, errors.Wrap(err, "failed to unmarshal PrepareAuthFactor reply")
	}
}

// TerminateFpAuthFactor terminates the fingerprint auth factor.
func (u *CryptohomeClient) TerminateFpAuthFactor(ctx context.Context, authSessionID string) (*uda.TerminateAuthFactorReply, error) {
	reply := &uda.TerminateAuthFactorReply{}

	binaryMsg, err := u.binary.terminateFpAuthFactor(ctx, authSessionID)
	if err != nil {
		return reply, errors.Wrap(err, "TerminateAuthFactor failed")
	}
	if err := proto.Unmarshal(binaryMsg, reply); err != nil {
		return reply, errors.Wrap(err, "failed to unmarshal TerminateAuthFactor reply")
	}

	return reply, nil
}

// PrepareThenAddFpAuthFactor prepares and adds an fingerprint auth factor.
// Since the CLI output contains all PrepareAuthFactor reply, progress signal, and AddAuthFactor
// reply, just return the execution error.
func (u *CryptohomeClient) PrepareThenAddFpAuthFactor(ctx context.Context, authSessionID, label string) error {
	binaryMsg, err := u.binary.prepareThenAddFpAuthFactor(ctx, authSessionID, label)
	if err != nil {
		testing.ContextLogf(ctx, "prepareAddFpAuthFactor returns with %q", binaryMsg)
	}
	return err
}

// PrepareThenAuthFpAuthFactor prepares and authenticate an fingerprint auth factor.
// Since the CLI output contains all PrepareAuthFactor reply, scan signal, and AuthenticateAuthFactor
// reply, just return the execution error.
func (u *CryptohomeClient) PrepareThenAuthFpAuthFactor(ctx context.Context, authSessionID string, labels []string) error {
	binaryMsg, err := u.binary.prepareThenAuthFpAuthFactor(ctx, authSessionID, labels)
	if err != nil {
		testing.ContextLogf(ctx, "prepareAuthFpAuthFactor returns with %q", binaryMsg)
	}
	return err
}

// WithAuthSession will execute a given block of code within an active
// AuthSession, handling the setup and teardown of the session automatically.
// The given block should take a string parameter (the authSessionID) and return
// nil on success and an error if one occurs.
//
// The given function would normally be written as a function literal which
// calls other cryptohome client functions that get used with an active session.
// So usage would generally look something like:
//
//	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
//	    if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
//	        return errors.Wrap(err, "add some context here")
//	    }
//	    if err := client.PreparePersistentVault(ctx, authSessionID, false /*encryptfs*/); err != nil {
//	        return errors.Wrap(err, "add some different context here")
//	    }
//	}); err != nil {
//	    s.Fatal("Error message that makes sense for this whole block: ", err)
//	}
//
// This example uses a session to create and prepare a new persistent user. Note
// that by using a closure as the parameter we can avoid a bunch of parameter
// boilerplate to pass in other test state like the tast context.
func (u *CryptohomeClient) WithAuthSession(
	ctx context.Context, username string, isEphemeral bool, authIntent uda.AuthIntent,
	f func(string) error) error {
	_, authSessionID, err := u.StartAuthSession(ctx, username, isEphemeral, authIntent)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session")
	}
	defer u.InvalidateAuthSession(ctx, authSessionID)
	return f(authSessionID)
}

// CreateVaultKeyset calls "--action=create_vault_keyset".
func (u *CryptohomeClient) CreateVaultKeyset(ctx context.Context, authSessionID, passkey, keyDataLabel string, authFactorType uda.AuthFactorType, disableKeyData bool) error {
	return u.binary.createVaultKeyset(ctx, authSessionID, passkey, keyDataLabel, authFactorType, disableKeyData)
}

// The following methods that specify the |hashInfo| param are added because the param is rarely specified,
// so we don't want to add it as new param of the standard functions used by many callers.

// AddAuthFactorWithHashInfo creates an auth factor for the user with given password and the hash info metadata.
func (u *CryptohomeClient) AddAuthFactorWithHashInfo(ctx context.Context, authSessionID, label, password string, hashInfo *uda.KnowledgeFactorHashInfo) error {
	_, err := u.binary.addAuthFactor(ctx, authSessionID, label, password, hashInfo)
	return err
}

// UpdatePasswordAuthFactorWithHashInfo updates an auth factor for the user with given password and the hash info metadata.
func (u *CryptohomeClient) UpdatePasswordAuthFactorWithHashInfo(ctx context.Context, authSessionID, label, password string, hashInfo *uda.KnowledgeFactorHashInfo) error {
	_, err := u.binary.updatePasswordAuthFactor(ctx, authSessionID, label, password, hashInfo)
	return err
}

// ReplacePasswordAuthFactorWithHashInfo replace an existing auth factor with a new one with given password and the hash info metadata.
func (u *CryptohomeClient) ReplacePasswordAuthFactorWithHashInfo(ctx context.Context, authSessionID, label, newKeyLabel, password string, hashInfo *uda.KnowledgeFactorHashInfo) error {
	_, err := u.binary.replacePasswordAuthFactor(ctx, authSessionID, label, newKeyLabel, password, hashInfo)
	return err
}

// AddPinAuthFactorWithHashInfo creates a pin auth factor for the user with given PIN and the hash info metadata.
func (u *CryptohomeClient) AddPinAuthFactorWithHashInfo(ctx context.Context, authSessionID, label, pin string, hashInfo *uda.KnowledgeFactorHashInfo) error {
	_, err := u.binary.addPinAuthFactor(ctx, authSessionID, label, pin, hashInfo)
	return err
}

// IsPinWeaverPkEstablishmentBlocked checks whether PinWeaver PK establishment is blocked now.
func (u *CryptohomeClient) IsPinWeaverPkEstablishmentBlocked(ctx context.Context) (bool, error) {
	binaryMsg, err := u.binary.isPinWeaverPkEstablishmentBlocked(ctx)
	if err != nil {
		return false, errors.Wrap(err, "IsPinWeaverPkEstablishmentBlocked failed")
	}
	return strings.Contains(string(binaryMsg), "true"), nil
}

// MigrateLegacyFingerprints migrates legacy fingerprint records to fingerprint auth factors.
func (u *CryptohomeClient) MigrateLegacyFingerprints(ctx context.Context, authSessionID string) (*uda.MigrateLegacyFingerprintsReply, error) {
	binaryMsg, err := u.binary.migrateLegacyFingerprints(ctx, authSessionID)

	// Attempt to parse the binaryMsg anyway, we need them to check for the correct error code.
	reply := &uda.MigrateLegacyFingerprintsReply{}
	if unmarshErr := proto.Unmarshal(binaryMsg, reply); unmarshErr != nil {
		return nil, errors.Wrap(unmarshErr, "failed to unmarshal MigrateLegacyFingerprints reply")
	}

	if err != nil {
		return reply, errors.Wrap(err, "MigrateLegacyFingerprints failed")
	}
	return reply, nil
}
