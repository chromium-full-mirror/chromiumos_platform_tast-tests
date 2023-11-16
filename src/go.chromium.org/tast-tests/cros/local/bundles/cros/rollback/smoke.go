// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"bytes"
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartCommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/metrics"
	"go.chromium.org/tast-tests/cros/local/rollback"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
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
		Data:         []string{"rollback_smoke_metrics_file"},
		Fixture:      fixture.CleanOwnership,
		// CleanOwnership doesn't work on reven/flex.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("reven")),
		Params: []testing.Param{{
			Name: "oobe_config_restore_running",
			Val:  oobeConfigRestoreRunningTest,
		}, {
			Name:      "oobe_config_save_no_flag",
			Val:       oobeConfigSaveNoFlagTest,
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "rollback_encrypt_and_failed_decrypt",
			Val:       rollbackEncryptFailedDecryptTest,
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "cleanup_metrics_when_oobe_is_not_completed",
			Val:       onlyCleanupMetricsWhenOobeIsNotCompletedTest,
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "cleanup_files_if_oobe_is_completed",
			Val:       cleanupFilesIfOobeIsCompletedTest,
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:              "tpm_encryption",
			Val:               tpmEncryptionTest,
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpmNvramRollbackSpace()),
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
		}, {
			Name:              "tpm_encryption_corrupted_metrics",
			Val:               tpmEncryptionCorruptedMetricsTest,
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpmNvramRollbackSpace()),
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
		}, {
			Name:              "cleanup_zeroes_tpm_space",
			Val:               cleanupZeroesTpmSpaceTest,
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpmNvramRollbackSpace()),
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
		}},
	})
}

// Smoke runs the Smoke tests.
// Which tests should go here?
// The goal is to create a suite of non-flaky rollback integration tests that can run in the CQ.
// These tests cover:
// - Sandboxing configurations
// - Users, groups and access rights
// - Upstart configurations
// - Communication with a real TPM
// Before adding a test, consider whether your test will contribute towards the above goals.
// If it does not, it may be more suitable to create a unit test.
func Smoke(ctx context.Context, s *testing.State) {
	// If a test should need the cleanupCtx, expand the test function parameters and pass it.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer func() {
		if err := rollback.CleanupRollbackLeftovers(cleanupCtx); err != nil {
			s.Error("Cleanup failed: ", err)
		}
	}()

	if err := metrics.CleanStructuredEvents(ctx); err != nil {
		s.Fatal("Failed to delete old reported events: ", err)
	}
	if err := rollback.PlaceRollbackMetricsFile(ctx, s.DataPath(rollback.RollbackMetricsFileWithPolicyActivatedEvent)); err != nil {
		s.Fatal("Failed to copy metrics file to DUT: ", err)
	}
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
	if err := rollback.RunOobeConfigSave(ctx); err != nil {
		s.Fatal("Failed to run oobe_config_save: ", err)
	}

	if err := rollback.CheckFileDoesNotExist(rollback.SslEncryptedRollbackData); err != nil {
		s.Fatal("Failure when checking that no encrypted data was created: ", err)
	}

	rollbackEvents := []string{rollback.RollbackPolicyActivatedMetric, rollback.RollbackOobeConfigSaveMetric, rollback.RollbackOobeConfigRestoreMetric}
	if err := rollback.CheckEventsHaveNotBeenReported(ctx, rollbackEvents); err != nil {
		s.Fatal("Failure when checking that no rollback metrics were reported: ", err)
	}
}

// rollbackEncryptFailedDecryptTest checks that oobe_config_save encrypts data and leaves the key for powerwash if the flag to save is present
// and that oobe_config_restore does not crash when attempting to decrypt (decryption will fail because we do not write to pstore).
func rollbackEncryptFailedDecryptTest(ctx context.Context, s *testing.State) {
	if err := rollback.PlaceDataSaveFlag(ctx); err != nil {
		s.Fatal("Failed to place data save flag: ", err)
	}
	if err := rollback.RunOobeConfigSave(ctx); err != nil {
		s.Fatal("Failed to run oobe_config_save: ", err)
	}
	if err := rollback.CheckFileExists(rollback.SslEncryptedRollbackData); err != nil {
		s.Fatal("Failed when checking that ssl encrypted data file was created: ", err)
	}
	if err := rollback.CheckFileExists(rollback.SslKey); err != nil {
		s.Fatal("Failed when checking that ssl key file was created: ", err)
	}
	// Remove TPM encrypted file to ensure decryption will fail.
	if err := os.Remove(rollback.TpmEncryptedRollbackData); err != nil && !os.IsNotExist(err) {
		s.Fatal("Failed to remove TPM encrypted data: ", err)
	}

	// Restarting Chrome should trigger a request to oobe_config_restore.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}
	if err := upstart.CheckJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failure when checking that oobe_config_restore is still running after failed decryption: ", err)
	}

	// All rollback events should have been reported after oobe_config_restore
	// decryption attempt.
	rollbackEvents := []string{rollback.RollbackPolicyActivatedMetric, rollback.RollbackOobeConfigSaveMetric, rollback.RollbackOobeConfigRestoreMetric}
	if err := rollback.WaitForEventsTobeReported(ctx, rollbackEvents); err != nil {
		s.Fatal("Failure when checking that all rollback metrics have been reported: ", err)
	}
}

// onlyCleanupMetricsWhenOobeIsNotCompletedTest checks that oobe_config_restore's
// cleanup functionality runs but only removes stale metrics file if OOBE is not yet completed.
func onlyCleanupMetricsWhenOobeIsNotCompletedTest(ctx context.Context, s *testing.State) {
	if err := fakePrecedingRollback(ctx); err != nil {
		s.Fatal("Failed to fake a preceding rollback: ", err)
	}

	// Create a metrics file with events to check that rollback_cleanup reads and
	// reports event metrics. The file created in fakePrecedingRollback is not
	// usable because oobe_config_restore cleaned it out already.
	if err := metrics.CleanStructuredEvents(ctx); err != nil {
		s.Fatal("Failed to delete old reported events: ", err)
	}
	if err := rollback.PlaceRollbackMetricsFile(ctx, s.DataPath(rollback.RollbackMetricsFileWithPolicyActivatedEvent)); err != nil {
		s.Fatal("Failed to copy metrics file to DUT: ", err)
	}

	// Run some checks while OOBE is not completed yet.
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore: ", err)
	}
	// File is not stale yet, should still be around.
	if err := rollback.CheckFileExists(rollback.MetricsData); err != nil {
		s.Fatal("Failure when checking that non-stale metrics file is kept: ", err)
	}

	// Make the file stale by modifying it's last modified date.
	if err := testexec.CommandContext(ctx, "touch", "-d", "16 days ago", rollback.MetricsData).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to make the metrics file stale: ", err)
	}
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore: ", err)
	}
	// oobe_config_restore daemon should have been started.
	if err := upstart.CheckJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failure when checking that oobe_config_restore is running: ", err)
	}
	if err := rollback.CheckFileDoesNotExist(rollback.MetricsData); err != nil {
		s.Fatal("Failure when checking that stale metrics file is deleted: ", err)
	}
	// Deleting the stale file should report only the events previously tracked.
	if err := rollback.WaitForEventsTobeReported(ctx, []string{rollback.RollbackPolicyActivatedMetric}); err != nil {
		s.Fatal("Failure when checking that tracked rollback events have been reported: ", err)
	}
	notTrackedRollbackEvents := []string{rollback.RollbackOobeConfigSaveMetric, rollback.RollbackOobeConfigRestoreMetric}
	if err := rollback.CheckEventsHaveNotBeenReported(ctx, notTrackedRollbackEvents); err != nil {
		s.Fatal("Failure when checking untracked events have not been reported: ", err)
	}

	// Rollback data should be kept until OOBE is finished.
	if err := rollback.CheckFilesExist(
		[]string{
			rollback.DecryptedRollbackData,
			rollback.SslEncryptedRollbackData,
			rollback.TpmEncryptedRollbackData}); err != nil {
		s.Fatal("Failure when checking that rollback data files are kept until OOBE is completed: ", err)
	}
}

// cleanupFilesIfOobeIsCompletedTest checks that oobe_config_restore's cleanup functionality runs
// and removes all remaining rollback files once OOBE is completed.
func cleanupFilesIfOobeIsCompletedTest(ctx context.Context, s *testing.State) {
	if err := fakePrecedingRollback(ctx); err != nil {
		s.Fatal("Failed to fake a preceding rollback: ", err)
	}

	// Create a metrics file with events to check that rollback_cleanup reads and
	// reports event metrics. The file created in fakePrecedingRollback is not
	// usable because oobe_config_restore cleaned it out already.
	if err := metrics.CleanStructuredEvents(ctx); err != nil {
		s.Fatal("Failed to delete old reported events: ", err)
	}
	if err := rollback.PlaceRollbackMetricsFile(ctx, s.DataPath(rollback.RollbackMetricsFileWithPolicyActivatedEvent)); err != nil {
		s.Fatal("Failed to copy metrics file to DUT: ", err)
	}

	// Fake oobe is completed and check that all files are deleted.
	if err := rollback.PlaceOobeCompletedFlag(ctx); err != nil {
		s.Fatal("Failed to fake oobe completion: ", err)
	}

	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore: ", err)
	}

	// oobe_config_restore daemon is not supposed to start, instead we only ran the cleanup.
	if err := upstart.WaitForJobStatus(ctx, "oobe_config_restore", upstartCommon.StopGoal, upstartCommon.WaitingState, upstart.RejectWrongGoal, rollback.WaitForJobStatusTimeout); err != nil {
		s.Fatal("Failure while waiting for oobe_config_restore to stop: ", err)
	}

	// Deleting the rollback metrics file because of rollback cleanup should only
	// report the events previously tracked.
	if err := rollback.WaitForEventsTobeReported(ctx, []string{rollback.RollbackPolicyActivatedMetric}); err != nil {
		s.Fatal("Failure when checking that tracked rollback events have been reported: ", err)
	}
	notTrackedRollbackEvents := []string{rollback.RollbackOobeConfigSaveMetric, rollback.RollbackOobeConfigRestoreMetric}
	if err := rollback.CheckEventsHaveNotBeenReported(ctx, notTrackedRollbackEvents); err != nil {
		s.Fatal("Failure when checking untracked events have not been reported: ", err)
	}

	if err := rollback.CheckFilesDoNotExist([]string{
		rollback.DecryptedRollbackData,
		rollback.SslEncryptedRollbackData,
		rollback.TpmEncryptedRollbackData,
		rollback.MetricsData}); err != nil {
		s.Fatal("Failure when checking that all rollback data was cleaned up: ", err)
	}
}

// tpmEncryptionTest runs encryption and decryption using the rollback TPM space.
// It verifies that encrypted file and decrypted file are present and enforces use of TPM encryption.
func tpmEncryptionTest(ctx context.Context, s *testing.State) {
	if err := tpmEncryptionDecryption(ctx); err != nil {
		s.Fatal("Failed to encrypt and decrypt with TPM: ", err)
	}

	rollbackEvents := []string{rollback.RollbackPolicyActivatedMetric, rollback.RollbackOobeConfigSaveMetric, rollback.RollbackOobeConfigRestoreMetric}
	if err := rollback.CheckEventsHaveBeenReported(ctx, rollbackEvents); err != nil {
		s.Fatal("Rollback events have not been reported: ", err)
	}
}

func tpmEncryptionCorruptedMetricsTest(ctx context.Context, s *testing.State) {
	if err := rollback.PlaceCorruptedRollbackMetrics(ctx); err != nil {
		s.Fatal("Failed to create corrupted metrics file")
	}

	if err := tpmEncryptionDecryption(ctx); err != nil {
		s.Fatal("Failed to encrypt and decrypt with TPM: ", err)
	}

	// No rollback events should have been reported from a corrupted metrics file.
	rollbackEvents := []string{rollback.RollbackPolicyActivatedMetric, rollback.RollbackOobeConfigSaveMetric, rollback.RollbackOobeConfigRestoreMetric}
	if err := rollback.CheckEventsHaveNotBeenReported(ctx, rollbackEvents); err != nil {
		s.Fatal("Failure when checking that no rollback metrics have been reported: ", err)
	}
}

func tpmEncryptionDecryption(ctx context.Context) error {
	if err := rollback.TriggerTpmEncryption(ctx); err != nil {
		return errors.Wrap(err, "failed to encrypt with TPM")
	}

	// Check that rollback space is not 0.
	secret, err := rollback.ReadRollbackTpmNvramSpace(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read rollback space")
	}
	if bytes.Equal(secret, rollback.ZeroTpmSpace[:]) {
		return errors.New("TPM space is still zero after encrypting")
	}

	// Delete the fallback openssl encrypted data to force code to use TPM encrypted file.
	if err := os.Remove(rollback.SslEncryptedRollbackData); err != nil {
		return errors.Wrap(err, "failed to remove SSL encrypted rollback data")
	}

	// Decrypt.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return errors.Wrap(err, "failed to restart ui to decrypt")
	}
	// Ui will request rollback data, which triggers decryption. Wait for that to finish.

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := rollback.CheckFileExists(rollback.DecryptedRollbackData); err != nil {
			return errors.Wrap(err, "could not find decrypted rollback data")
		}
		// After decryption, rollback space is reset. Check the space is 0 again.
		secret, err = rollback.ReadRollbackTpmNvramSpace(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read rollback space"))
		}
		if !bytes.Equal(secret, rollback.ZeroTpmSpace[:]) {
			return errors.Wrapf(err, "TPM space is %v, wanted %v", secret, rollback.ZeroTpmSpace)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Second * 10}); err != nil {
		return errors.Wrap(err, "failure while waiting for successful decryption")
	}

	return nil
}

// cleanupZeroesTpmSpaceTest verifies that running oobe_config_restore upstart job triggers
// cleaning the rollback TPM space if OOBE is completed.
func cleanupZeroesTpmSpaceTest(ctx context.Context, s *testing.State) {
	if err := rollback.TriggerTpmEncryption(ctx); err != nil {
		s.Fatal("Failed to encrypt with TPM: ", err)
	}

	// Check that rollback space is not 0.
	secret, err := rollback.ReadRollbackTpmNvramSpace(ctx)
	if err != nil {
		s.Fatal("Failed to read rollback space: ", err)
	}
	if bytes.Equal(secret, rollback.ZeroTpmSpace[:]) {
		s.Fatal("TPM space is still zero after encrypting")
	}

	// Fake oobe is completed and check that the space is cleared.
	if err := rollback.PlaceOobeCompletedFlag(ctx); err != nil {
		s.Fatal("Failed to fake oobe completion: ", err)
	}

	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		secret, err = rollback.ReadRollbackTpmNvramSpace(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read rollback space"))
		}
		if !bytes.Equal(secret, rollback.ZeroTpmSpace[:]) {
			return errors.Wrapf(err, "TPM space is %v, wanted %v", secret, rollback.ZeroTpmSpace)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Second * 10}); err != nil {
		s.Fatal("Failure while waiting for TPM space reset: ", err)
	}
}

// fakePrecedingRollback can be used for setup, it creates files that would be left by a preceding rollback.
func fakePrecedingRollback(ctx context.Context) error {
	if err := rollback.PlaceFakedDecryptedRollbackData(ctx); err != nil {
		return errors.Wrap(err, "failed to fake decrypted rollback file")
	}
	if err := rollback.RunSaveAndRestore(ctx); err != nil {
		return errors.Wrap(err, "failed to run save and restore")
	}

	// Use reporting rollback events as a sign that oobe_config_restore finished.
	rollbackEvents := []string{rollback.RollbackPolicyActivatedMetric, rollback.RollbackOobeConfigSaveMetric, rollback.RollbackOobeConfigRestoreMetric}
	if err := rollback.WaitForEventsTobeReported(ctx, rollbackEvents); err != nil {
		return err
	}

	if err := rollback.CheckFilesExist(
		[]string{
			rollback.DecryptedRollbackData,
			rollback.SslEncryptedRollbackData,
			rollback.TpmEncryptedRollbackData,
			rollback.MetricsData}); err != nil {
		return errors.Wrap(err, "failure when checking that all rollback data was created")
	}
	return nil
}
