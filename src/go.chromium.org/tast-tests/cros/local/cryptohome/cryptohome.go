// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cryptohome operates on encrypted home directories.
package cryptohome

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// WaitForUserTimeout is the maximum time until a user mount is available.
	WaitForUserTimeout = hwsec.WaitForUserTimeout

	// GuestUser is the name representing a guest user account.
	// Defined in libbrillo/brillo/cryptohome.cc.
	GuestUser = hwsec.GuestUser

	// KioskUser is the name representing a kiosk user account.
	KioskUser = hwsec.KioskUser

	// userCleanupWaitTime is the time we wait to cleanup a user post user creation.
	userCleanupWaitTime = 5 * time.Second

	// defaultGaiaPasswordLabel is the default label used to sign into chromebook using their GAIA account.
	defaultGaiaPasswordLabel = "gaia"

	// persistentTestFile is filename used for creating test file
	persistentTestFile = "file"

	persistentTestFileContent = "content"
)

func newClient() *hwsec.CryptohomeClient {
	return hwsec.NewCryptohomeClient(hwseclocal.NewLoglessCmdRunner())
}

func newMountInfo() *hwsec.CryptohomeMountInfo {
	r := hwseclocal.NewLoglessCmdRunner()
	return hwsec.NewCryptohomeMountInfo(r, hwsec.NewCryptohomeClient(r))
}

// UserHash returns user's cryptohome hash.
func UserHash(ctx context.Context, user string) (string, error) {
	client := newClient()
	hash, err := client.GetUserHash(ctx, user)
	if err != nil {
		return "", errors.Wrap(err, "failed to get user hash")
	}
	return hash, nil
}

// UserPath returns the path to user's encrypted home directory.
func UserPath(ctx context.Context, user string) (string, error) {
	// When in a guest user session, return the fixed path mounted by
	// mountns.EnterUserSessionMountNS.
	// TODO: Don't rely on the username.
	if user == "$guest@gmail.com" {
		return "/home/chronos/user", nil
	}

	client := newClient()
	path, err := client.GetHomeUserPath(ctx, user)
	if err != nil {
		return "", errors.Wrap(err, "failed to get user home path")
	}
	return path, nil
}

// MyFilesPath returns the path to the user's MyFiles directory within
// their encrypted home directory.
func MyFilesPath(ctx context.Context, user string) (string, error) {
	userPath, err := UserPath(ctx, user)
	if err != nil {
		return "", err
	}
	return filepath.Join(userPath, "MyFiles"), nil
}

// DownloadsPath returns the path to the user's Downloads directory within
// their encrypted home directory.
func DownloadsPath(ctx context.Context, user string) (string, error) {
	myFilesPath, err := MyFilesPath(ctx, user)
	if err != nil {
		return "", err
	}
	return filepath.Join(myFilesPath, "Downloads"), nil
}

// SystemPath returns the path to user's encrypted system directory.
func SystemPath(ctx context.Context, user string) (string, error) {
	client := newClient()
	path, err := client.GetRootUserPath(ctx, user)
	if err != nil {
		return "", errors.Wrap(err, "failed to get user home path")
	}
	return path, nil
}

// RemoveUserDir removes a user's encrypted home directory.
// Success is reported if the user directory doesn't exist,
// but an error will be returned if the user is currently logged in.
func RemoveUserDir(ctx context.Context, user string) error {
	client := newClient()
	if _, err := client.RemoveVault(ctx, user); err != nil {
		return errors.Wrap(err, "failed to remove cryptohome")
	}
	return nil
}

// MountType is a type of the user mount.
type MountType = hwsec.MountType

const (
	// Ephemeral is used to specify that the expected user mount type is ephemeral.
	Ephemeral = hwsec.Ephemeral
	// Permanent is used to specify that the expected user mount type is permanent.
	Permanent = hwsec.Permanent
)

// WaitForUserMountAndValidateType waits for user's encrypted home directory to
// be mounted and validates that it is of correct type.
func WaitForUserMountAndValidateType(ctx context.Context, user string, mountType MountType) error {
	cmi := newMountInfo()

	if err := cmi.WaitForUserMountAndValidateType(ctx, user, mountType); err != nil {
		return errors.Wrap(err, "failed to wait for user mount and validate type")
	}
	return nil
}

// WaitForUserMount waits for user's encrypted home directory to be mounted and
// validates that it is of permanent type for all users except guest.
func WaitForUserMount(ctx context.Context, user string) error {
	cmi := newMountInfo()

	if err := cmi.WaitForUserMount(ctx, user); err != nil {
		return errors.Wrap(err, "failed to wait for user mount")
	}
	return nil
}

// CreateVault creates the vault for the user with given password.
func CreateVault(ctx context.Context, user, password string) error {
	testing.ContextLogf(ctx, "Creating vault mount for user %q", user)
	client := newClient()
	cmi := newMountInfo()

	if err := client.MountVault(ctx, defaultGaiaPasswordLabel, hwsec.NewPassAuthConfig(user, password), true, hwsec.NewVaultConfig()); err != nil {
		return errors.Wrap(err, "failed to create user vault")
	}

	err := testing.Poll(ctx, func(ctx context.Context) error {
		path, err := cmi.UserCryptohomePath(ctx, user)
		if err != nil {
			return errors.Wrap(err, "failed to locate user cryptohome path")
		}

		if _, err := os.Stat(path); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 60 * time.Second, Interval: 1 * time.Second})

	if err != nil {
		return errors.Wrapf(err, "failed to create vault for %s", user)
	}
	return nil
}

// MountVault mounts the vault for the user with given password.
func MountVault(ctx context.Context, user, password string) error {
	testing.ContextLogf(ctx, "Creating vault mount for user %q", user)
	client := newClient()

	if err := client.MountVault(ctx, defaultGaiaPasswordLabel, hwsec.NewPassAuthConfig(user, password), false, hwsec.NewVaultConfig()); err != nil {
		return errors.Wrap(err, "failed to create user vault")
	}
	return nil
}

// RemoveVault removes the vault for the user.
func RemoveVault(ctx context.Context, user string) error {
	testing.ContextLogf(ctx, "Removing vault for user %q", user)
	client := newClient()
	cmi := newMountInfo()

	if _, err := client.RemoveVault(ctx, user); err != nil {
		return errors.Wrap(err, "failed to remove cryptohome")
	}

	path, err := cmi.UserCryptohomePath(ctx, user)
	if err != nil {
		return errors.Wrap(err, "failed to locate user cryptohome path")
	}

	// Ensure that the vault does not exist.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return errors.Wrapf(err, "cryptohome could not remove vault for user %q", user)
	}
	return nil
}

// UnmountAll unmounts all user vaults.
func UnmountAll(ctx context.Context) error {
	testing.ContextLog(ctx, "Unmounting all user vaults")
	client := newClient()

	if err := client.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount vaults")
	}
	return nil
}

// UnmountVault unmounts the vault for the user.
func UnmountVault(ctx context.Context, user string) error {
	testing.ContextLogf(ctx, "Unmounting vault for user %q", user)
	client := newClient()
	cmi := newMountInfo()

	if _, err := client.Unmount(ctx, user); err != nil {
		return errors.Wrapf(err, "failed to unmount vault for user %q", user)
	}

	if mounted, err := cmi.IsMounted(ctx, user); err == nil && mounted {
		return errors.Errorf("cryptohome did not unmount user %q", user)
	}
	return nil
}

// ForceRemoveVault unmounts all vaults and remove the vault for the specified user.
func ForceRemoveVault(ctx context.Context, user string) error {
	if err := UnmountAll(ctx); err != nil {
		return err
	}
	if err := RemoveVault(ctx, user); err != nil {
		return err
	}
	return nil
}

// MountedVaultPath returns the path where the decrypted data for the user is located.
func MountedVaultPath(ctx context.Context, user string) (string, error) {
	cmi := newMountInfo()

	path, err := cmi.MountedVaultPath(ctx, user)
	if err != nil {
		return "", errors.Wrap(err, "failed to locate user vault path")
	}

	return path, nil
}

// IsMounted checks if the vault for the user is mounted.
func IsMounted(ctx context.Context, user string) (bool, error) {
	cmi := newMountInfo()

	mounted, err := cmi.IsMounted(ctx, user)
	if err != nil {
		return false, errors.Errorf("failed to check user %q is mounted", user)
	}
	return mounted, nil
}

// MountGuest sends a request to cryptohome to create a mount point for a
// guest user.
func MountGuest(ctx context.Context) error {
	testing.ContextLog(ctx, "Mounting guest cryptohome")
	client := newClient()
	cmi := newMountInfo()

	if err := client.MountGuest(ctx); err != nil {
		return errors.Wrap(err, "failed to request mounting guest vault")
	}

	if err := cmi.WaitForUserMount(ctx, hwsec.GuestUser); err != nil {
		return errors.Wrap(err, "failed to mount guest vault")
	}
	return nil
}

// CheckMountNamespace checks whether the user session mount namespace has been created.
func CheckMountNamespace(ctx context.Context) error {
	cmi := newMountInfo()

	if err := cmi.CheckMountNamespace(ctx); err != nil {
		return errors.Wrap(err, "failed to check mount namespace")
	}
	return nil
}

// CheckService performs high-level verification of cryptohome.
func CheckService(ctx context.Context) error {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		return errors.Wrap(err, "failed to create hwsec local helper")
	}
	daemonController := helper.DaemonController()

	if err := daemonController.Ensure(ctx, hwsec.CryptohomeDaemon); err != nil {
		return errors.Wrap(err, "failed to ensure cryptohome")
	}

	return nil
}

// CheckDeps performs high-level verification of cryptohome related daemons.
func CheckDeps(ctx context.Context) error {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		return errors.Wrap(err, "failed to create hwsec local helper")
	}
	daemonController := helper.DaemonController()

	if err := daemonController.EnsureDaemons(ctx, hwsec.HighLevelTPMDaemons); err != nil {
		return errors.Wrap(err, "failed to ensure high-level TPM daemons")
	}

	return nil
}

// WriteFileForPersistence writes a files which can be later verified to exist.
func WriteFileForPersistence(ctx context.Context, username string) error {

	// Write a test file to verify persistence.
	userPath, err := UserPath(ctx, username)
	if err != nil {
		return errors.Wrap(err, "user vault path fetch failed")
	}
	filePath := filepath.Join(userPath, persistentTestFile)
	if err := os.WriteFile(filePath, []byte(persistentTestFileContent), 0644); err != nil {
		return errors.Wrap(err, "write file operation failed")
	}
	return nil
}

// VerifyFileForPersistence writes a files which can be later verified to exist.
func VerifyFileForPersistence(ctx context.Context, username string) error {
	userPath, err := UserPath(ctx, username)
	if err != nil {
		return errors.Wrap(err, "failed to get user vault path")
	}
	filePath := filepath.Join(userPath, persistentTestFile)
	// Verify that file is still there.
	if content, err := os.ReadFile(filePath); err != nil {
		return errors.Wrap(err, "failed to read test file")
	} else if bytes.Compare(content, []byte(persistentTestFileContent)) != 0 {
		return errors.Wrap(err, "incorrect tests file content")
	}
	return nil
}

// VerifyFileUnreadability verifies that the file
// written(orUnwritten as part of WriteFileForPersistence) is unreadable.
func VerifyFileUnreadability(ctx context.Context, username string) error {
	userPath, err := UserPath(ctx, username)
	if err != nil {
		return errors.Wrap(err, "failed to get user vault path")
	}
	filePath := filepath.Join(userPath, persistentTestFile)
	// Verify non-persistence.
	if _, err := os.ReadFile(filePath); err == nil {
		return errors.Wrap(err, "file is persisted when it is not expected to be")
	}
	return nil
}

// TestLockScreen does lock screen password checks.
func TestLockScreen(ctx context.Context, userName, userPassword, wrongPassword, keyLabel string, client *hwsec.CryptohomeClient) error {
	accepted, err := client.CheckVault(ctx, keyLabel, hwsec.NewPassAuthConfig(userName, userPassword))
	if err != nil {
		return errors.Wrap(err, "failed to check correct password")
	}
	if !accepted {
		return errors.New("correct password rejected")
	}

	accepted, err = client.CheckVault(ctx, "" /* label */, hwsec.NewPassAuthConfig(userName, userPassword))
	if err == nil {
		return errors.Wrap(err, "empty key label check succeeded when it shouldn't")
	}
	if accepted {
		return errors.New("wildcard label accepted when AuthFactor should not accept empty label")
	}

	accepted, err = client.CheckVault(ctx, keyLabel, hwsec.NewPassAuthConfig(userName, wrongPassword))
	if err == nil {
		return errors.Wrap(err, "wrong password check succeeded when it shouldn't")
	}
	if accepted {
		return errors.New("wrong password check returned true despite an error")
	}

	return nil
}

// MountAndVerify tests that after a successful mount with AuthSession, the testFile still exists.
// Note: Caller takes care of the unmount operation
func MountAndVerify(ctx context.Context, userName, authSessionID string, ecryptFs bool) error {
	client := newClient()

	if _, err := client.PreparePersistentVault(ctx, authSessionID, ecryptFs); err != nil {
		return errors.Wrap(err, "prepare persistent vault")
	}

	// Verify that file is still there.
	if err := VerifyFileForPersistence(ctx, userName); err != nil {
		return errors.Wrap(err, "verify test file")
	}
	return nil
}

// CheckKeyBackingStoreExists checks the existence of keyset file under user shadow directory.
func CheckKeyBackingStoreExists(ctx context.Context, keysetPath, userName string) error {
	const shadowDir = "/home/.shadow"
	hash, err := UserHash(ctx, userName)
	if err != nil {
		return errors.Wrap(err, "failed to get user hash")
	}
	if _, err = os.Stat(filepath.Join(shadowDir, hash, keysetPath)); err != nil {
		return errors.Wrap(err, "failed to stat key backing store file")
	}
	return nil
}

// TestPinCounterWithAuthSession tests that PIN is locked out after too many wrong trials and can be reset by the correct password.
func TestPinCounterWithAuthSession(ctx context.Context, authSessionID, passwordLabel, userPassword, pinLabel, userPin, wrongPin string, client *hwsec.CryptohomeClient) error {
	const numberOfWrongAttemptToNotLock = 4
	const numberOfWrongAttemptToLock = 5
	// Try authenticate with wrong PIN to increase the PIN counter, but don't lock out.
	for i := 0; i < numberOfWrongAttemptToNotLock; i++ {
		_, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, wrongPin)
		if err == nil {
			return errors.Wrap(err, "authentication with wrong PIN succeeded unexpectedly")
		}
	}

	// Authenticate with correct PIN factor should reset the counter since it is not locked out yet.
	if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
		return errors.Wrap(err, "authenticating with correct PIN failed")
	}

	// Try authenticate with wrong PIN 5 times to lock out the PIN.
	replyError := &uda.AuthenticateAuthFactorReply{}
	var err error
	for i := 0; i < numberOfWrongAttemptToLock; i++ {
		replyError, err = client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, wrongPin)
		if err == nil {
			return errors.Wrap(err, "authentication with wrong PIN succeeded unexpectedly")
		}
	}
	if replyError.Error != uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_CREDENTIAL_LOCKED {
		return errors.Errorf("PIN is not locked out after too many wrong attempts. The received wrong error message is: %v", replyError.Error)
	}

	// Authenticate with correct PIN should fail since the PIN is locked out.
	replyError, err = client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin)
	if err == nil {
		return errors.Wrap(err, "authenticating with correct PIN after PIN lock out succeded unexpectedly, should have failed")
	}
	if replyError.Error != uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_TPM_DEFEND_LOCK {
		return errors.Errorf("PIN should have been in locked out state, but it is not. The error message received is: %v", replyError.Error)
	}

	// Authenticate with password AuthFactor and reset the PIN counter.
	if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
		return errors.Wrap(err, "failed to authenticate with password AuthFactor after PIN is locked")
	}

	// Authenticate with correct PIN should now succeed.
	if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
		return errors.Wrap(err, "authenticating with correct PIN failed after the counter is reset")
	}
	return nil
}

// TestPinCounterMechanism tests that PIN is locked out after too many wrong trials and can be reset by the correct password
func TestPinCounterMechanism(ctx context.Context, userName, passwordLabel, userPassword, pinLabel, userPin, wrongPin string, client *hwsec.CryptohomeClient) error {
	const numberOfWrongAttemptToNotLock = 4
	const numberOfWrongAttemptToLock = 5

	// Start an Auth session and get an authSessionID.
	_, authSessionID, err := client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start Auth session")
	}
	defer client.InvalidateAuthSession(ctx, authSessionID)

	return TestPinCounterWithAuthSession(ctx, authSessionID, passwordLabel, userPassword, pinLabel, userPin, wrongPin, client)
}

// CheckCliErrorCode examines that the error from cryptohome cli invocation contains the desired error code.
// Return an error explaining the mismatch.
func CheckCliErrorCode(err error, expectedErrorCode int) error {
	if expectedErrorCode == int(uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_NOT_SET) {
		if err != nil {
			return errors.Wrap(err, "cryptohome cli returns an error")
		}
	} else if err == nil || !strings.Contains(err.Error(), strconv.Itoa(expectedErrorCode)) {
		return errors.Wrapf(err, "cryptohome cli does not return expected error code: %d, got: ", expectedErrorCode)
	}
	return nil
}
