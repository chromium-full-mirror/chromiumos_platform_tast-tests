// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/util"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParam struct {
	pinWeaverSupported bool
}

type keyInfo struct {
	username   string
	password   string
	keyLabel   string
	lowEntropy bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: LockToSingleUserMountUntilReboot,
		Desc: "Checks that LockToSingleUserMountUntilReboot method works",
		Contacts: []string{
			"cros-hwsec@google.com",
			"zuan@chromium.org", // Test author
		},
		BugComponent: "b:1188704",
		Attr:         []string{"informational", "group:mainline", "group:cryptohome"},
		SoftwareDeps: []string{"tpm"},
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"no_tpm_dynamic"},
				Val:               testParam{pinWeaverSupported: false},
			},
			{
				Name:              "pin_weaver",
				ExtraSoftwareDeps: []string{"pinweaver"},
				Val:               testParam{pinWeaverSupported: true},
			},
			{
				Name:              "tpm_dynamic",
				ExtraSoftwareDeps: []string{"tpm_dynamic"},
				ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
				Val:               testParam{pinWeaverSupported: false},
			},
		},
		Timeout: 5 * time.Minute,
	})
}

// create2VaultsForTesting will create 2 vaults for testing.
func create2VaultsForTesting(ctx context.Context, utility *hwsec.CryptohomeClient, keyInfo1, keyInfo2 []keyInfo) (returnedErr error) {
	// Cleanup if we failed.
	defer func() {
		if returnedErr != nil {
			if err := utility.UnmountAll(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to unmount when cleaning up create2VaultsForTesting: ", err)
			}

			if err := cleanupVault(ctx, utility); err != nil {
				testing.ContextLog(ctx, "Failed to cleanup vault when cleanup create2VaultsForTesting: ", err)
			}
		}
	}()

	// Create 2 vaults for testing.
	for _, infoList := range [][]keyInfo{keyInfo1, keyInfo2} {
		if err := utility.WithAuthSession(ctx, infoList[0].username, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if err := utility.CreatePersistentUser(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to create user")
			}
			if _, err := utility.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to prepare persistent vault")
			}
			for _, info := range infoList {
				if info.lowEntropy {
					if err := utility.AddPinAuthFactor(ctx, authSessionID, info.keyLabel, info.password); err != nil {
						return errors.Wrap(err, "failed to add PIN key to vault")
					}
				} else {
					if err := utility.AddAuthFactor(ctx, authSessionID, info.keyLabel, info.password); err != nil {
						return errors.Wrap(err, "failed to add password key to vault")
					}
				}
			}
			return nil
		}); err != nil {
			return errors.Wrapf(err, "failed to create vault and add keys for user %s", infoList[0].username)
		}
	}

	// Unmount the vault before further testing.
	if err := utility.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount vault before testing")
	}

	return nil
}

// cleanupVault will delete the first and second user's vault.
func cleanupVault(ctx context.Context, utility *hwsec.CryptohomeClient) (returnedErr error) {
	// Unmount first.
	if err := utility.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount vaults before cleanup")
	}
	// Remove the vault.
	if _, err := utility.RemoveVault(ctx, util.FirstUsername); err != nil {
		returnedErr = errors.Wrap(err, "failed to remove first user's vault")
		// Note: We are not returning here because we want to cleanup the
		// second vault as well.
	}
	if _, err := utility.RemoveVault(ctx, util.SecondUsername); err != nil {
		returnedErr = errors.Wrap(err, "failed to remove second user's vault")
	}
	return returnedErr
}

// checkVaultWorks will check that the vault specified by username, both passwords and pin (if supported) works in both mounting and unlock.
func checkVaultWorks(ctx context.Context, utility *hwsec.CryptohomeClient, keyInfos []keyInfo) error {
	for _, info := range keyInfos {
		if err := utility.WithAuthSession(ctx, info.username, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if info.lowEntropy {
				if _, err := utility.AuthenticatePinAuthFactor(ctx, authSessionID, info.keyLabel, info.password); err != nil {
					return errors.Wrap(err, "failed to authenticate AuthFactor")
				}
			} else {
				if _, err := utility.AuthenticateAuthFactor(ctx, authSessionID, info.keyLabel, info.password); err != nil {
					return errors.Wrap(err, "failed to authenticate AuthFactor")
				}
			}
			if _, err := utility.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to prepare persistent vault")
			}

			return nil
		}); err != nil {
			return errors.Wrapf(err, "failed to mount with %s", info.keyLabel)
		}

		if info.lowEntropy {
			if result, err := utility.CheckPinVault(ctx, info.keyLabel, hwsec.NewPassAuthConfig(info.username, info.password)); !result {
				return errors.Wrapf(err, "failed to check pin key with %s", info.keyLabel)
			}
		} else {
			if result, err := utility.CheckVault(ctx, info.keyLabel, hwsec.NewPassAuthConfig(info.username, info.password)); !result {
				return errors.Wrapf(err, "failed to check password key with %s", info.keyLabel)
			}
		}

		if err := utility.UnmountAll(ctx); err != nil {
			return errors.Wrap(err, "failed to unmount vault")
		}

	}

	return nil
}

// checkBothVaultsAreOperational will check that both first user's vault and second user's vault are operational.
func checkBothVaultsAreOperational(ctx context.Context, utility *hwsec.CryptohomeClient, keyInfos1, keyInfos2 []keyInfo) error {
	// Now test that logging in on both user works as expected.
	if err := checkVaultWorks(ctx, utility, keyInfos1); err != nil {
		return errors.Wrap(err, "first user's vault doesn't work")
	}
	if err := checkVaultWorks(ctx, utility, keyInfos2); err != nil {
		return errors.Wrap(err, "second user's vault doesn't work")
	}

	return nil
}

// checkOthersAreBlocked will check that users other than util.FirstUsername are blocked from mounting and CheckKeyEx (unlock).
func checkOthersAreBlocked(ctx context.Context, utility *hwsec.CryptohomeClient, otherKeyInfo []keyInfo) error {
	for _, info := range otherKeyInfo {
		// Mount should fail.
		if err := utility.MountVault(ctx, info.keyLabel, hwsec.NewPassAuthConfig(info.username, info.password), false, hwsec.NewVaultConfig()); err == nil {
			return errors.Wrapf(err, "second user is mountable with %s after locking to single user", info.keyLabel)
		}

		// CheckKeyEx should fail too.
		if result, _ := utility.CheckVault(ctx, info.keyLabel, hwsec.NewPassAuthConfig(info.username, info.password)); result {
			return errors.Errorf("check key succeeded with %s after locking to single user", info.keyLabel)
		}
	}

	return nil
}

func LockToSingleUserMountUntilReboot(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	// Standard initializations.
	r := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(r, s.DUT())
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}
	utility := helper.CryptohomeClient()

	pinSupported := s.Param().(testParam).pinWeaverSupported

	// LockToSingleUserMountUntilReboot would only available when the TPM is ready.
	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to wait for TPM to be owned: ", err)
	}

	// Create the vaults for testing.
	keyInfos1 := []keyInfo{
		{
			username: util.FirstUsername,
			password: util.FirstPassword1,
			keyLabel: util.Password1Label,
		},
		{
			username: util.FirstUsername,
			password: util.FirstPassword2,
			keyLabel: util.Password2Label,
		}}
	keyInfos2 := []keyInfo{
		{
			username: util.SecondUsername,
			password: util.SecondPassword1,
			keyLabel: util.Password1Label,
		},
		{
			username: util.SecondUsername,
			password: util.SecondPassword2,
			keyLabel: util.Password2Label,
		}}
	if pinSupported {
		keyInfos1 = append(keyInfos1, keyInfo{
			username:   util.FirstUsername,
			password:   util.FirstPin,
			keyLabel:   util.PinLabel,
			lowEntropy: true,
		})
		keyInfos2 = append(keyInfos2, keyInfo{
			username:   util.SecondUsername,
			password:   util.SecondPin,
			keyLabel:   util.PinLabel,
			lowEntropy: true,
		})
	}

	// Clean up before the test run to make sure we start with a clean slate.
	cleanupVault(ctx, utility)

	if err := create2VaultsForTesting(ctx, utility, keyInfos1, keyInfos2); err != nil {
		s.Fatal("Failed to initialize vaults for testing: ", err)
	}
	defer func(ctx context.Context) {
		if err := cleanupVault(ctx, utility); err != nil {
			s.Error("Failed to cleanup vault: ", err)
		}
	}(cleanupCtx)

	// Before starting the actual test, check that everything is alright.
	if err := checkBothVaultsAreOperational(ctx, utility, keyInfos1, keyInfos2); err != nil {
		s.Fatal("Initially created vaults doesn't work: ", err)
	}

	// Now mount the first user's vault and lock to single user mount.
	if err := utility.MountVault(ctx, util.Password2Label, hwsec.NewPassAuthConfig(util.FirstUsername, util.FirstPassword2), false, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to mount the user for lock to single user mount: ", err)
	}

	hasRebooted := false
	defer func(ctx context.Context) {
		if !hasRebooted {
			if err := helper.Reboot(ctx); err != nil {
				s.Error("Failed to reboot: ", err)
			}
		}
	}(cleanupCtx)

	if err := utility.LockToSingleUserMountUntilReboot(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to lock to single user mount: ", err)
	}

	// Check that the other user is blocked.
	if err := checkOthersAreBlocked(ctx, utility, keyInfos2); err != nil {
		s.Fatal("Other users are not blocked: ", err)
	}

	// Unmount the first user's vault.
	if _, err := utility.Unmount(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to unmount first user's vault after locking: ", err)
	}

	// The first user's vault should still work (MountEx).
	if err := checkVaultWorks(ctx, utility, keyInfos1); err != nil {
		s.Fatal("The first user's vault doesn't work after locking: ", err)
	}

	// Now reboot and check that the effects wear off.
	if err := helper.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	hasRebooted = true

	if err := checkBothVaultsAreOperational(ctx, utility, keyInfos1, keyInfos2); err != nil {
		s.Fatal("Vaults doesn't work after reboot: ", err)
	}
}
