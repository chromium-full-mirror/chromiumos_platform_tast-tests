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

	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/fsutil"
	"chromiumos/tast/local/hwsec/enckey"
	"chromiumos/tast/testing"
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

// BackupAttestationDbWithFakeGoogleKeys backs up the attestation database.
func BackupAttestationDbWithFakeGoogleKeys(ctx context.Context) (lastErr error) {
	if _, err := os.Stat(attestationDBBackupPath); !os.IsNotExist(err) {
		testing.ContextLog(ctx, "Backup db exists. Skipping")
		return
	}

	// Create dir to backup a9n db.
	if err := os.MkdirAll(path.Dir(attestationDBBackupPath), 0644); err != nil {
		return errors.Wrap(err, "failed to create dir to back up attestation db")
	}

	// Initialize daemon controller.
	r := NewCmdRunner()
	helper, err := NewFullHelper(ctx, r)
	if err != nil {
		return errors.Wrap(err, "error while creating helper")
	}
	dc := helper.DaemonController()

	// Stop the currently running a9n daemon.
	if err := dc.Stop(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to stop attestation service")
	}

	// Remove the existing a9n DB.
	if err := os.Remove(hwsec.AttestationDBPath); err != nil {
		return errors.Wrap(err, "failed to remove attestation database")
	}

	// Inject fake Google Keys.
	if err := enckey.InjectWellKnownGoogleKeys(ctx); err != nil {
		return errors.Wrap(err, "failed to inject well-known keys")
	}

	// Revert the key injection if other parts of this function fails.
	defer func() {
		if lastErr != nil {
			if err := enckey.InjectNormalGoogleKeysAndRestart(ctx, dc); err != nil {
				testing.ContextLog(ctx, "Failed to inject the normal keys back: ", err)
			}
		}
	}()

	// Start a9n daemon again.
	if err := dc.Start(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to start attestation while enabling ali")
	}

	// Ensure a9n is prepared for enrollment.
	if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
		return errors.Wrap(err, "failed to prepare for enrollment")
	}
	testing.ContextLog(ctx, "Prepared for Enrollment")

	// Backup the newly created a9n DB with fake google keys.
	if err := fsutil.CopyFile(hwsec.AttestationDBPath, attestationDBBackupPath); err != nil {
		return errors.Wrap(err, "failed to back up fake attestation database")
	}

	if err := dc.Stop(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to stop attestation after backing up fake a9n db")
	}

	if err := os.Remove(hwsec.AttestationDBPath); err != nil {
		return errors.Wrap(err, "failed to remove fake attestation database")
	}

	if err := enckey.InjectNormalGoogleKeys(ctx); err != nil {
		return errors.Wrap(err, "failed to inject the normal keys back")
	}

	if err := dc.Start(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to start attestation after backing up fake a9n db")
	}

	testing.ContextLog(ctx, "Sleeping for 5s so that attestation daemon can start and create the database")
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	// Replace the a9n DB with the fake one.
	if err := fsutil.CopyFile(attestationDBBackupPath, hwsec.AttestationDBPath); err != nil {
		return errors.Wrap(err, "failed to replace with the fake attestation database")
	}
	return nil
}
