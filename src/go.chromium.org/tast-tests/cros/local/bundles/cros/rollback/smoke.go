// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"io/fs"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs rollback executables in different states of the system, making sure nothing crashes and the output looks ok",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mpolzer@google.com",
			"crisguerrero@chromium.org",
		},
		BugComponent: "b:1031231",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Timeout:      1 * time.Minute,
		Fixture:      fixture.CleanOwnership,
		Params: []testing.Param{{
			Name: "oobe_confg_restore_running",
			Val:  oobeConfigRestoreRunningTest,
		}, {
			Name: "oobe_config_save_no_flag",
			Val:  oobeConfigSaveNoFlagTest,
		}, {
			Name: "rollback_encrypt_and_failed_decrypt",
			Val:  rollbackEncryptFailedDecryptTest,
		}},
	})
}

const dataSaveFlag = "/mnt/stateful_partition/.save_rollback_data"
const sslEncryptedRollbackData = "/mnt/stateful_partition/unencrypted/preserve/rollback_data"
const tpmEncryptedRollbackData = "/mnt/stateful_partition/unencrypted/preserve/rollback_data_tpm"

const oobeConfigSaveDir = "/var/lib/oobe_config_save"
const sslKey = "/var/lib/oobe_config_save/data_for_pstore"

const oobeConfigRestoreDir = "/var/lib/oobe_config_restore"

// Smoke runs the Smoke tests.
func Smoke(ctx context.Context, s *testing.State) {
	// Every small subtest has its own function and is declared in the test parameters.
	// This allows to run test with different attributes but avoids creating a separate file for each subtest.
	s.Param().(func(context.Context, *testing.State))(ctx, s)
}

// oobeConfigRestoreRunningTest checks that oobe_config_restore is running when oobe is not finished
func oobeConfigRestoreRunningTest(ctx context.Context, s *testing.State) {
	if err := upstart.CheckJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failure when checking that oobe_config_restore is running: ", err)
	}
}

// oobeConfigSaveNoFlagTest checks that oobe_config_save does nothing if the flag to save data is not present
func oobeConfigSaveNoFlagTest(ctx context.Context, s *testing.State) {
	defer cleanupRollbackFiles(ctx)
	if err := runOobeConfigSave(ctx); err != nil {
		s.Fatal("Failed to run oobe_config_save: ", err)
	}

	if err := checkFileDoesNotExist(sslEncryptedRollbackData); err != nil {
		s.Fatal("Failure when checking that no encrypted data was created: ", err)
	}
}

// rollbackEncryptFailedDecryptTest checks that oobe_config_save encrypts data and leaves the key for powerwash if the flag to save is present
// and that oobe_config_restore does not crash when attempting to decrypt (decryption will fail because we do not write to pstore).
func rollbackEncryptFailedDecryptTest(ctx context.Context, s *testing.State) {
	defer cleanupRollbackFiles(ctx)
	if err := placeDataSaveFlag(ctx); err != nil {
		s.Fatal("Failed to place data save flag: ", err)
	}
	if err := runOobeConfigSave(ctx); err != nil {
		s.Fatal("Failed to run oobe_config_save: ", err)
	}
	if err := checkFileExists(sslEncryptedRollbackData); err != nil {
		s.Fatal("Failed when checking that ssl encrypted data file was created: ", err)
	}
	if err := checkFileExists(sslKey); err != nil {
		s.Fatal("Failed when checking that ssl key file was created: ", err)
	}

	// Restarting Chrome should trigger a request to oobe_config_restore
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}
	if err := upstart.CheckJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failure when checking that oobe_config_restore is still running after failed decryption: ", err)
	}
}

func runOobeConfigSave(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "start", "oobe_config_save").Run(); err != nil {
		return errors.Wrap(err, "failed to start oobe_config_save")
	}
	return nil
}

func placeDataSaveFlag(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "touch", "/mnt/stateful_partition/.save_rollback_data").Run(); err != nil {
		return errors.Wrap(err, "failed to place data save flag")
	}
	return nil
}

func checkFileExists(path string) error {
	if _, err := os.ReadFile(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errors.Errorf("file %s does not exist", path)
		}
		return err
	}
	return nil
}

func checkFileDoesNotExist(path string) error {
	if _, err := os.ReadFile(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return errors.Errorf("file %s exists", path)
}

func cleanupRollbackFiles(ctx context.Context) error {
	paths := []string{
		dataSaveFlag,
		sslEncryptedRollbackData,
		tpmEncryptedRollbackData,
		oobeConfigSaveDir,
		oobeConfigRestoreDir,
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to remove %s", path)
		}
	}
	return nil
}
