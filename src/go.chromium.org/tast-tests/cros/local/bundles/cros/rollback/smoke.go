// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartCommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
		Attr:         []string{"group:mainline"},
		Timeout:      1 * time.Minute,
		Fixture:      fixture.CleanOwnership,
		// CleanOwnership doesn't work on reven/flex.
		HardwareDeps: hwdep.D(hwdep.SkipOnPlatform("reven")),
		Params: []testing.Param{{
			Name: "oobe_confg_restore_running",
			Val:  oobeConfigRestoreRunningTest,
		}, {
			Name: "oobe_config_save_no_flag",
			Val:  oobeConfigSaveNoFlagTest,
		}, {
			Name: "rollback_encrypt_and_failed_decrypt",
			Val:  rollbackEncryptFailedDecryptTest,
		}, {
			Name: "cleanup_metrics_when_oobe_is_not_completed",
			Val:  onlyCleanupMetricsWhenOobeIsNotCompletedTest,
		}, {
			Name: "cleanup_files_if_oobe_is_completed",
			Val:  cleanupFilesIfOobeIsCompletedTest,
		}},
	})
}

// Corrupt data to put into files that the code may attempt to read.
// This does not replace fuzzing of our data files, it's just to possibly execute a bit more of our code.
const corruptData = "54686572652061726520616C7761797320636F7272757074206D656E2077686F20686F61726420706F77" +
	"657220666F72207468656972206F776E206761696E2E2E2E2E2E6275742074686572652061726520616C776179732068" +
	"6F6E6F7261626C65206D656E2077686F20686F61726420706F77657220746F206669676874207468656D2E"

const dataSaveFlag = "/mnt/stateful_partition/.save_rollback_data"
const sslEncryptedRollbackData = "/mnt/stateful_partition/unencrypted/preserve/rollback_data"
const tpmEncryptedRollbackData = "/mnt/stateful_partition/unencrypted/preserve/rollback_data_tpm"
const metricsData = "/mnt/stateful_partition/unencrypted/preserve/enterprise-rollback-metrics-data"

const oobeConfigSaveDir = "/var/lib/oobe_config_save"
const sslKey = "/var/lib/oobe_config_save/data_for_pstore"

const oobeConfigRestoreDir = "/var/lib/oobe_config_restore"
const decryptedRollbackData = "/var/lib/oobe_config_restore/rollback_data"

const oobeCompletedFile = "/home/chronos/.oobe_completed"

// Smoke runs the Smoke tests.
// Which tests should go here?
// The goal is to create a suite of non-flaky rollback integration tests that can run in the CQ.
// These tests cover:
// - Sandboxing configurations
// - Users, groups and access rights
// - Upstart configurations
// Before adding a test, consider whether your test will contribute towards the above goals.
// If it does not, it may be more suitable to create a unit test.
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

// onlyCleanupMetricsWhenOobeIsNotCompletedTest checks that oobe_config_restore's
// cleanup functionality runs but only removes stale metrics file if OOBE is not yet completed.
func onlyCleanupMetricsWhenOobeIsNotCompletedTest(ctx context.Context, s *testing.State) {
	defer cleanupRollbackFiles(ctx)

	if err := fakePrecedingRollback(ctx); err != nil {
		s.Fatal("Failed to fake a preceding rollback: ", err)
	}

	// Run some checks while OOBE is not completed yet.
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore: ", err)
	}
	// File is not stale yet, should still be around.
	if err := checkFileExists(metricsData); err != nil {
		s.Fatal("Failure when checking that non-stale metrics file is kept: ", err)
	}
	// Make the file stale by modifying it's last modified date.
	if err := testexec.CommandContext(ctx, "touch", "-d", "16 days ago", metricsData).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to make the metrics file stale: ", err)
	}
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore: ", err)
	}
	// oobe_config_restore daemon should have been started.
	if err := upstart.CheckJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failure when checking that oobe_config_restore is running: ", err)
	}
	if err := checkFileDoesNotExist(metricsData); err != nil {
		s.Fatal("Failure when checking that stale metrics file is deleted: ", err)
	}
	// Rollback data should be kept until OOBE is finished.
	if err := checkFilesExist(
		[]string{
			decryptedRollbackData,
			sslEncryptedRollbackData,
			tpmEncryptedRollbackData}); err != nil {
		s.Fatal("Failure when checking that rollback data files are kept until OOBE is completed: ", err)
	}
}

// cleanupFilesIfOobeIsCompletedTest checks that oobe_config_restore's cleanup functionality runs
// and removes all remaining rollback files once OOBE is completed.
func cleanupFilesIfOobeIsCompletedTest(ctx context.Context, s *testing.State) {
	defer cleanupRollbackFiles(ctx)

	if err := fakePrecedingRollback(ctx); err != nil {
		s.Fatal("Failed to fake a preceding rollback: ", err)
	}

	// Fake oobe is completed and check that all files are deleted.
	if err := testexec.CommandContext(ctx, "touch", oobeCompletedFile).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to fake oobe completion: ", err)
	}

	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore: ", err)
	}

	// oobe_config_restore daemon is not supposed to start, instead we only ran the cleanup.
	if err := upstart.WaitForJobStatus(ctx, "oobe_config_restore", upstartCommon.StopGoal, upstartCommon.WaitingState, upstart.TolerateWrongGoal, 30*time.Second); err != nil {
		s.Fatal("Failure while waiting for oobe_config_restore to stop: ", err)
	}

	if err := checkFilesDoNotExist([]string{
		decryptedRollbackData,
		sslEncryptedRollbackData,
		tpmEncryptedRollbackData,
		metricsData}); err != nil {
		s.Fatal("Failure when checking that all rollback data was cleaned up: ", err)
	}
}

// fakePrecedingRollback can be used for setup, it creates files that would be left by a preceding rollback.
func fakePrecedingRollback(ctx context.Context) error {
	if err := placeFakedRollbackMetrics(ctx); err != nil {
		return errors.Wrap(err, "failed to create metrics file")
	}
	if err := placeFakedDecryptedRollbackData(ctx); err != nil {
		return errors.Wrap(err, "failed to fake decrypted rollback file")
	}
	if err := runSaveAndRestore(ctx); err != nil {
		return errors.Wrap(err, "failed to run save and restore")
	}
	if err := checkFilesExist(
		[]string{
			decryptedRollbackData,
			sslEncryptedRollbackData,
			tpmEncryptedRollbackData,
			metricsData}); err != nil {
		return errors.Wrap(err, "failure when checking that all rollback data was created")
	}
	return nil
}

func placeFakedRollbackMetrics(ctx context.Context) error {
	if err := os.WriteFile(metricsData, []byte(corruptData), 0664); err != nil {
		return errors.Wrap(err, "failed to create metrics file")
	}
	return nil
}

func placeFakedDecryptedRollbackData(ctx context.Context) error {
	if err := os.WriteFile(decryptedRollbackData, []byte(corruptData), 0644); err != nil {
		return errors.Wrap(err, "failed to create decrypted rollback file")
	}
	group, err := user.Lookup("oobe_config_restore")
	if err != nil {
		return errors.Wrap(err, "failed to lookup oobe_config_restore user")
	}
	uid, _ := strconv.Atoi(group.Uid)
	gid, _ := strconv.Atoi(group.Gid)
	if err := os.Chown(decryptedRollbackData, uid, gid); err != nil {
		return errors.Wrap(err, "failed to change owner of decrypted rollback data")
	}
	return nil
}

// runSaveAndRestore runs rollback's save and restore path.
// Note that while this executes the whole path, decryption will fail on device that use openssl/pstore for encryption.
func runSaveAndRestore(ctx context.Context) error {
	if err := placeDataSaveFlag(ctx); err != nil {
		return errors.Wrap(err, "failed to place data save flag")
	}
	if err := runOobeConfigSave(ctx); err != nil {
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

func runOobeConfigSave(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "start", "oobe_config_save").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to start oobe_config_save")
	}
	return nil
}

func placeDataSaveFlag(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "touch", "/mnt/stateful_partition/.save_rollback_data").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to place data save flag")
	}
	return nil
}

func checkFilesExist(paths []string) error {
	for _, file := range paths {
		if err := checkFileExists(file); err != nil {
			return err
		}
	}
	return nil
}

func checkFilesDoNotExist(paths []string) error {
	for _, file := range paths {
		if err := checkFileDoesNotExist(file); err != nil {
			return err
		}
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
		metricsData,
		oobeCompletedFile,
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to remove %s", path)
		}
	}
	return nil
}
