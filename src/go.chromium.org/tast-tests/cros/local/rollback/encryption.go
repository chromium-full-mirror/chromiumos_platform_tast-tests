// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartCommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

// ZeroTpmSpace variable.
var ZeroTpmSpace = [32]byte{}

// WaitForJobStatusTimeout provides time to ensure jobs change their status.
const WaitForJobStatusTimeout = 30 * time.Second

// RunSaveAndRestore runs rollback's save and restore path.
// Note that while this executes the whole path, decryption will fail on device
// that use openssl/pstore for encryption.
func RunSaveAndRestore(ctx context.Context) error {
	if err := PlaceDataSaveFlag(ctx); err != nil {
		return errors.Wrap(err, "failed to place data save flag")
	}
	if err := RunOobeConfigSave(ctx); err != nil {
		return errors.Wrap(err, "failed to run oobe_config_save")
	}

	// Restart oobe_config_restore as would usually happen during boot after rollback.
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		return errors.Wrap(err, "failed to restart oobe_config_restore")
	}

	// Restarting Chrome should trigger a request to oobe_config_restore, hence attempts to decrypt.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return errors.Wrap(err, "failed to restart Chrome")
	}
	return nil
}

// RunOobeConfigSave runs rollback's save and waits until it finishes.
func RunOobeConfigSave(ctx context.Context) error {
	// Stop oobe_config_restore to make sure it's not just in the process of starting and tries to decrypt our data while we encrypt it.
	if err := upstart.StopJob(ctx, "oobe_config_restore"); err != nil {
		return errors.Wrap(err, "failed to stop oobe_config_restore")
	}
	defer upstart.StartJob(ctx, "oobe_config_restore")

	if err := testexec.CommandContext(ctx, "start", "oobe_config_save").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to start oobe_config_save")
	}
	if err := upstart.WaitForJobStatus(ctx, "oobe_config_save", upstartCommon.StopGoal, upstartCommon.WaitingState, upstart.TolerateWrongGoal, WaitForJobStatusTimeout); err != nil {
		return errors.Wrap(err, "failed to wait for oobe_config_save to finish running")
	}
	return nil
}

// TriggerTpmEncryption triggers TPM encryption.
func TriggerTpmEncryption(ctx context.Context) error {
	// Stop oobe_config_restore to make sure it's not just in the process of starting and tries to decrypt our data while we encrypt it.
	if err := upstart.StopJob(ctx, "oobe_config_restore"); err != nil {
		return errors.Wrap(err, "failed to stop oobe_config_restore")
	}
	defer upstart.StartJob(ctx, "oobe_config_restore")

	if err := testexec.CommandContext(ctx, "systemd-tmpfiles", "--create", "--remove", "--clean", "/usr/lib/tmpfiles.d/on-demand/oobe_config_save.conf").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run tmpfiles for oobe_config_save")
	}
	if err := testexec.CommandContext(ctx, "sudo", "-u", "oobe_config_save", "-g", "oobe_config", "--", "oobe_config_save", "-tpm_encrypt").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run oobe_config_save executable")
	}
	if err := upstart.WaitForJobStatus(ctx, "oobe_config_save", upstartCommon.StopGoal, upstartCommon.WaitingState, upstart.TolerateWrongGoal, WaitForJobStatusTimeout); err != nil {
		return errors.Wrap(err, "failed to wait for oobe_config_save to finish running")
	}
	return nil
}

// ReadRollbackTpmNvramSpace read the rollback TPM space.
func ReadRollbackTpmNvramSpace(ctx context.Context) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "rollback_space_content_*")
	if err != nil {
		return []byte{}, errors.Wrap(err, "failed to create tmp file")
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if err := testexec.CommandContext(ctx, "tpm_manager_client", "read_space", "--index=0x100e", fmt.Sprintf("--file=%v", tmpFile.Name())).Run(testexec.DumpLogOnError); err != nil {
		return []byte{}, errors.Wrap(err, "failed to read NVRAM data")
	}
	spaceContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return []byte{}, errors.Wrap(err, "failed to read tmp file")
	}
	return spaceContent, nil
}
