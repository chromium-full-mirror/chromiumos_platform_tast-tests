// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package statefulmigration

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast-tests/cros/remote/statefulmigration"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type vaultType int64

type params struct {
	VaultType vaultType
}

const (
	noneVaultType vaultType = iota
	ecryptfsVaultType
	fscryptVaultType
	defaultVaultType
)

const (
	userNameEcryptfs     = "foo_ecryptfs@bar.baz"
	userPasswordEcryptfs = "secret"
	userNameFscrypt      = "foo_fscrypt@bar.baz"
	userPasswordFscrypt  = "secret"
	userNameLvm          = "foo_lvm@bar.baz"
	userPasswordLvm      = "secret"
	testFile             = "test_file"
	testFileContents     = "test file contents"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VaultCompat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify vaults are compatible across LVM migration",
		Contacts: []string{
			"chromeos-storage@google.com",
			"sarthakkukreti@google.com", // Test author
		},
		BugComponent: "b:974567",
		Attr:         []string{"group:lvm_migration", "lvm_migration_cryptohome"},
		SoftwareDeps: []string{"tpm", "reboot", "vpd"},
		Timeout:      5 * time.Minute,
	})
}

func prepareVault(ctx context.Context, dut *dut.DUT, cryptohome *hwsec.CryptohomeClient, cmdRunner *hwsecremote.CmdRunnerRemote, vtype vaultType, create bool, username, password string) error {
	// None is a wrong type.
	if vtype == noneVaultType || vtype > defaultVaultType {
		return errors.Errorf("unsupported type: %v", vtype)
	}

	config := hwsec.NewVaultConfig()
	if vtype == ecryptfsVaultType {
		config.Ecryptfs = true
	}
	if err := cryptohome.MountVault(ctx, "password", hwsec.NewPassAuthConfig(username, password) /* create=*/, true, config); err != nil {
		return errors.Wrap(err, "failed to create user vault for testing")
	}

	if err := hwsec.WriteUserTestContent(ctx, cryptohome, cmdRunner, username, testFile, testFileContents); err != nil {
		return errors.Wrap(err, "failed to write user vault test content")
	}

	if err := cryptohome.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount all vaults")
	}

	return nil
}

func verifyVault(ctx context.Context, dut *dut.DUT, cryptohome *hwsec.CryptohomeClient, cmdRunner *hwsecremote.CmdRunnerRemote, vtype vaultType, username, password string) error {
	// None is a wrong type.
	if vtype == noneVaultType || vtype > defaultVaultType {
		return errors.Errorf("unsupported type: %v", vtype)
	}

	config := hwsec.NewVaultConfig()
	if vtype == ecryptfsVaultType {
		config.Ecryptfs = true
	}

	if err := cryptohome.MountVault(ctx, "password", hwsec.NewPassAuthConfig(username, password) /* create=*/, false, config); err != nil {
		return errors.Wrap(err, "failed to create user vault for testing")
	}

	// User vault should already exist and shouldn't be recreated.
	if content, err := hwsec.ReadUserTestContent(ctx, cryptohome, cmdRunner, username, testFile); err != nil {
		return errors.Wrap(err, "failed to read user vault test content")
	} else if !bytes.Equal(content, []byte(testFileContents)) {
		return errors.Errorf("unexpected user vault test file content: got %q, want %q", string(content), testFileContents)
	}

	if err := cryptohome.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount all vaults")
	}

	return nil

}

func preMigrationVaultTest(ctx context.Context, dut *dut.DUT) error {
	cmdRunner := hwsecremote.NewCmdRunner(dut)
	helper, err := hwsecremote.NewHelper(cmdRunner, dut)
	if err != nil {
		return errors.Wrap(err, "failed to create hwsec remote helper")
	}
	cryptohome := helper.CryptohomeClient()

	// Create eCryptfs and fscrypt vaults.
	if err := prepareVault(ctx, dut, cryptohome, cmdRunner, ecryptfsVaultType, true, userNameEcryptfs, userPasswordEcryptfs); err != nil {
		return errors.Wrap(err, "failed to create eCryptfs vault for testing")
	}

	if err := prepareVault(ctx, dut, cryptohome, cmdRunner, fscryptVaultType, true, userNameFscrypt, userPasswordFscrypt); err != nil {
		return errors.Wrap(err, "failed to create fscrypt vault for testing")
	}

	return nil
}

func postMigrationVaultTest(ctx context.Context, dut *dut.DUT) error {
	cmdRunner := hwsecremote.NewCmdRunner(dut)
	helper, err := hwsecremote.NewHelper(cmdRunner, dut)
	if err != nil {
		return errors.Wrap(err, "failed to create hwsec remote helper")
	}
	cryptohome := helper.CryptohomeClient()

	// Validate that existing eCryptfs and fscrypt vaults still work after migration.
	if err := verifyVault(ctx, dut, cryptohome, cmdRunner, ecryptfsVaultType, userNameEcryptfs, userPasswordEcryptfs); err != nil {
		return errors.Wrap(err, "failed to verify eCryptfs vault for testing")
	}

	if err := verifyVault(ctx, dut, cryptohome, cmdRunner, fscryptVaultType, userNameFscrypt, userPasswordFscrypt); err != nil {
		return errors.Wrap(err, "failed to verify fscrypt vault for testing")
	}

	// Validate creation of new LVM vault and that files can be added to the vault.
	if err := prepareVault(ctx, dut, cryptohome, cmdRunner, defaultVaultType, true, userNameLvm, userPasswordLvm); err != nil {
		return errors.Wrap(err, "failed to create LVM vault for testing")
	}

	return nil
}

func VaultCompat(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	ops := &statefulmigration.Operations{
		PreMigration:  preMigrationVaultTest,
		PostMigration: postMigrationVaultTest,
	}

	// Run migration test.
	if err := statefulmigration.MigrationTest(ctx, dut, s.RPCHint(), s.OutDir(), ops); err != nil {
		s.Fatal("Failed to run migration test: ", err)
	}
}
