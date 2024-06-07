// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"os"
	"path"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/hwsec/enckey"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const attestationDBBackupPath = "/var/lib/attestation/attestation.epb.tast-hwsec-backup"
const tpmManagerLocalDataBackupPath = "/var/lib/tpm_manager/local_tpm_data.tast-hwsec-backup"

// isTPMLocalDataIntact uses tpm_manager_client to check if local data still contains owner password,
// which means the set of important secrets are still intact.
func isTPMLocalDataIntact(ctx context.Context) (bool, error) {
	out, err := testexec.CommandContext(ctx, "tpm_manager_client", "status").Output()
	if err != nil {
		return false, errors.Wrap(err, "failed to call tpm_manager_client")
	}
	return strings.Contains(string(out), "owner_password"), nil
}

// BackupTPMManagerDataIfIntact backs up a the tpm manager data if the important secrets is not cleared.
func BackupTPMManagerDataIfIntact(ctx context.Context) error {
	ok, err := isTPMLocalDataIntact(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check tpm local data")
	}
	if !ok {
		return errors.New("owner password not found")
	}
	if err := fsutil.CopyFile(hwsec.TpmManagerLocalDataPath, tpmManagerLocalDataBackupPath); err != nil {
		return errors.Wrap(err, "failed to copy tpm manager local data")
	}
	return nil
}

// RestoreTPMManagerData copies the backup file back to the location of tpm manager local data.
func RestoreTPMManagerData(ctx context.Context) error {
	if err := fsutil.CopyFile(tpmManagerLocalDataBackupPath, hwsec.TpmManagerLocalDataPath); err != nil {
		return errors.Wrap(err, "failed to copy tpm manager local data backup")
	}
	return nil
}

// RestoreTPMOwnerPasswordIfNeeded restores the owner password from the snapshot stored
// at the beginning of the entire test program, if the owner password got wiped already.
func RestoreTPMOwnerPasswordIfNeeded(ctx context.Context, dc *hwsec.DaemonController) error {
	hasOwnerPassword, err := isTPMLocalDataIntact(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check owner password")
	}
	if hasOwnerPassword {
		return nil
	}
	if err := RestoreTPMManagerData(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to restore tpm manager local data")
		testing.ContextLog(ctx, "If you saw this on local testing, probably the TPM ownership isn't taken by the testing infra")
		testing.ContextLog(ctx, "You chould try to power wash the device and run the test again")
		return errors.Wrap(err, "failed to restore tpm manager local data")
	}
	if err := dc.Restart(ctx, hwsec.TPMManagerDaemon); err != nil {
		return errors.Wrap(err, "failed to restart tpm manager")
	}
	hasOwnerPassword, err = isTPMLocalDataIntact(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check owner password")
	}
	if !hasOwnerPassword {
		return errors.Wrap(err, "no owner password after restoration")
	}
	return nil
}

// CreateFakeAttestationDatabase creates a fake attestation database.
func CreateFakeAttestationDatabase(ctx context.Context, helper *FullHelperLocal) (lastError error) {
	dc := helper.DaemonController()
	// Create dir to back up attestation database.
	if err := os.MkdirAll(path.Dir(attestationDBBackupPath), 0777); err != nil {
		return errors.Wrap(err, "failed to create dir to back up attestation db")
	}

	// Stop the currently running attestation daemon.
	if err := dc.Stop(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to stop attestation service")
	}

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if err := dc.Start(ctx, hwsec.AttestationDaemon); err != nil {
			lastError = errors.Wrap(err, "failed to resume attestation service")
			return
		}
		if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
			lastError = errors.Wrap(err, "failed to prepare for enrollment after creating fake database")
			return
		}
		testing.ContextLog(ctx, "Prepared for enrollment after creating fake database")
	}(ctxForCleanup)

	// Remove the existing attestation database.
	if err := os.Remove(hwsec.AttestationDBPath); err != nil {
		return errors.Wrap(err, "failed to remove attestation database; might be a flex device")
	}

	// Inject fake Google keys.
	if err := enckey.InjectWellKnownGoogleKeys(ctx); err != nil {
		return errors.Wrap(err, "failed to inject well-known keys")
	}
	return nil
}

// RestoreNormalAttestationDatabase creates the normal attestation database back.
func RestoreNormalAttestationDatabase(ctx context.Context, helper *FullHelperLocal) (lastError error) {
	dc := helper.DaemonController()
	// Stop currently running attestation daemon.
	if err := dc.Stop(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to stop attestation service after backing up the database")
	}

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if err := dc.Start(ctx, hwsec.AttestationDaemon); err != nil {
			lastError = errors.Wrap(err, "failed to resume attestation service")
			return
		}
		if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
			lastError = errors.Wrap(err, "failed to prepare for enrollment after reverting to normal database")
			return
		}
		testing.ContextLog(ctx, "Prepared for enrollment after reverting to normal database")
	}(ctxForCleanup)

	// Remove the fake attestation database.
	if err := os.Remove(hwsec.AttestationDBPath); err != nil {
		return errors.Wrap(err, "failed to remove fake attestation database")
	}

	// Remove fake google keys.
	if err := enckey.InjectNormalGoogleKeys(ctx); err != nil {
		return errors.Wrap(err, "failed to inject the normal keys back")
	}
	return nil
}

// BackupAttestationDbWithFakeGoogleKeys backs up the attestation database with fake google keys.
func BackupAttestationDbWithFakeGoogleKeys(ctx context.Context) error {
	if _, err := os.Stat(attestationDBBackupPath); !os.IsNotExist(err) {
		testing.ContextLog(ctx, "Backup db exists. Skipping")
		return nil
	}

	r := NewCmdRunner()
	helper, err := NewFullHelper(ctx, r)
	if err != nil {
		return errors.Wrap(err, "error while creating helper instance")
	}

	if err := CreateFakeAttestationDatabase(ctx, helper); err != nil {
		return errors.Wrap(err, "failed to create the fake attestation database")
	}

	// Backup the fake attestation database.
	if err := fsutil.CopyFile(hwsec.AttestationDBPath, attestationDBBackupPath); err != nil {
		return errors.Wrap(err, "failed to back up the fake attestation database")
	}

	if err := RestoreNormalAttestationDatabase(ctx, helper); err != nil {
		return errors.Wrap(err, "something went wrong while restoring normal attestation database")
	}
	return nil
}
