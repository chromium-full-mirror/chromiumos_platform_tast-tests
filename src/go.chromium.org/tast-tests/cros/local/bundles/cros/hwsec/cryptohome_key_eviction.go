// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CryptohomeKeyEviction,
		Desc: "Ensures that the cryptohome properly manages key eviction from the tpm",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"tpm"},
		Attr:         []string{"group:hwsec", "hwsec_nightly", "group:cryptohome"},
		Timeout:      10 * time.Minute,
		Fixture:      "ussAuthSessionFixture",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"no_tpm_dynamic"},
		}, {
			Name:              "tpm_dynamic",
			ExtraSoftwareDeps: []string{"tpm_dynamic"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}},
	})
}

// CryptohomeKeyEviction ensures that the cryptohome properly manages key eviction from the tpm.
// This test verifies this behaviour by creating 30 keys using chaps, and then remounting a user's cryptohome.
// Mount requires use of the user's cryptohome key, and thus the mount only succeeds if the cryptohome key was properly evicted and reloaded into the TPM.
func CryptohomeKeyEviction(ctx context.Context, s *testing.State) {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec helper: ", err)
	}
	cryptohome := helper.CryptohomeClient()

	chaps, err := pkcs11.NewChaps(ctx, cmdRunner, cryptohome)
	if err != nil {
		s.Fatal("Failed to create chaps client: ", err)
	}

	mountInfo := hwsec.NewCryptohomeMountInfo(cmdRunner, cryptohome)

	const (
		user     = util.FirstUsername
		password = util.FirstPassword
	)

	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to ensure TPM is ready: ", err)
	}

	defer func(ctx context.Context) {
		// Ensure we remove the user account after the test.
		if err := mountInfo.CleanUpMount(ctx, user); err != nil {
			s.Fatal("Failed to cleanup: ", err)
		}
	}(ctx)

	// Ensure clean cryptohome.
	if err := mountInfo.CleanUpMount(ctx, user); err != nil {
		s.Fatal("Failed to cleanup: ", err)
	}

	// Mount the user vault.
	if err := cryptohome.MountVault(ctx, util.PasswordLabel, hwsec.NewPassAuthConfig(user, password), true, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to mount vault: ", err)
	}

	// Wait and get the user slot.
	if err := cryptohome.WaitForUserToken(ctx, user); err != nil {
		s.Fatal("Failed to wait for user token: ", err)
	}
	slot, err := cryptohome.GetTokenForUser(ctx, user)
	if err != nil {
		s.Fatal("Failed to get user slot: ", err)
	}

	// First we inject 30 tokens into chaps. This forces the cryptohome key to get evicted.
	for i := 0; i < 30; i++ {
		if err := chaps.ReplayWifiBySlot(ctx, slot, "--inject"); err != nil {
			s.Fatal("Failed to inject a key into a PKCS #11 token and tests that it can sign: ", err)
		}
	}

	// Then we get a user to remount cryptohome. This process uses the cryptohome key,
	// and if the user was able to login, the cryptohome key was correctly reloaded.
	if _, err := cryptohome.Unmount(ctx, user); err != nil {
		s.Fatal("Failed to unmount: ", err)
	}
	if err := cryptohome.MountVault(ctx, util.PasswordLabel, hwsec.NewPassAuthConfig(user, password), false, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to mount vault: ", err)
	}
}
