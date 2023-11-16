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

	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

const dataSaveFlag = "/mnt/stateful_partition/.save_rollback_data"
const oobeCompletedFile = "/home/chronos/.oobe_completed"
const oobeConfigRestoreDir = "/var/lib/oobe_config_restore"
const oobeConfigSaveDir = "/var/lib/oobe_config_save"

// CorruptData to put into files that the code may attempt to read.
// This does not replace fuzzing of our data files, it's just to possibly execute a bit more of our code.
const CorruptData = "54686572652061726520616C7761797320636F7272757074206D656E2077686F20686F61726420706F77" +
	"657220666F72207468656972206F776E206761696E2E2E2E2E2E6275742074686572652061726520616C776179732068" +
	"6F6E6F7261626C65206D656E2077686F20686F61726420706F77657220746F206669676874207468656D2E"

// DecryptedRollbackData fakes decrypted rollback data.
const DecryptedRollbackData = "/var/lib/oobe_config_restore/rollback_data"

// SslEncryptedRollbackData fakes rollback data encrypted using open ssl.
const SslEncryptedRollbackData = "/mnt/stateful_partition/unencrypted/preserve/rollback_data"

// SslKey fakes the key that is created after ssl encryption.
const SslKey = "/var/lib/oobe_config_save/data_for_pstore"

// TpmEncryptedRollbackData fakes rollback data encrypted using TPM.
const TpmEncryptedRollbackData = "/mnt/stateful_partition/unencrypted/preserve/rollback_data_tpm"

// MetricsData is the file were rollback metrics are tracked.
const MetricsData = "/mnt/stateful_partition/unencrypted/preserve/enterprise-rollback-metrics-data"

// PlaceDataSaveFlag creates the flag that will trigger rollback's save path.
func PlaceDataSaveFlag(ctx context.Context) error {
	if err := os.WriteFile(dataSaveFlag, []byte(""), 0644); err != nil {
		return errors.Wrap(err, "failed to place data save flag")
	}
	return nil
}

// PlaceFakedDecryptedRollbackData creates decrypted rollback data with the
// right permissions.
func PlaceFakedDecryptedRollbackData(ctx context.Context) error {
	if err := os.WriteFile(DecryptedRollbackData, []byte(CorruptData), 0644); err != nil {
		return errors.Wrap(err, "failed to create decrypted rollback file")
	}
	group, err := user.Lookup("oobe_config_restore")
	if err != nil {
		return errors.Wrap(err, "failed to lookup oobe_config_restore user")
	}
	uid, _ := strconv.Atoi(group.Uid)
	gid, _ := strconv.Atoi(group.Gid)
	if err := os.Chown(DecryptedRollbackData, uid, gid); err != nil {
		return errors.Wrap(err, "failed to change owner of decrypted rollback data")
	}
	return nil
}

// PlaceOobeCompletedFlag fakes oobe completion.
func PlaceOobeCompletedFlag(ctx context.Context) error {
	if err := os.WriteFile(oobeCompletedFile, []byte(""), 0644); err != nil {
		return errors.Wrap(err, "failed to place oobe completed flag")
	}
	return nil
}

// CheckFilesExist verifies all the files provided exists.
func CheckFilesExist(paths []string) error {
	for _, file := range paths {
		if err := CheckFileExists(file); err != nil {
			return err
		}
	}
	return nil
}

// CheckFilesDoNotExist verifies none of the files provided exists.
func CheckFilesDoNotExist(paths []string) error {
	for _, file := range paths {
		if err := CheckFileDoesNotExist(file); err != nil {
			return err
		}
	}
	return nil
}

// CheckFileExists verifies the file provided exists.
func CheckFileExists(path string) error {
	if _, err := os.ReadFile(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errors.Errorf("file %s does not exist", path)
		}
		return err
	}
	return nil
}

// CheckFileDoesNotExist verifies the file provided does not exist.
func CheckFileDoesNotExist(path string) error {
	if _, err := os.ReadFile(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return errors.Errorf("file %s exists", path)
}

// CleanupRollbackLeftovers leaves the DUT in a fresh state after running rollback.
func CleanupRollbackLeftovers(ctx context.Context) (retErr error) {
	retErr = errors.Join(retErr, upstart.StopJob(ctx, "oobe_config_save"))
	retErr = errors.Join(retErr, upstart.StopJob(ctx, "oobe_config_restore"))
	paths := []string{
		dataSaveFlag,
		SslEncryptedRollbackData,
		SslKey,
		TpmEncryptedRollbackData,
		oobeConfigSaveDir,
		oobeConfigRestoreDir,
		MetricsData,
		oobeCompletedFile,
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			retErr = errors.Join(retErr, errors.Wrapf(err, "error removing %s", path))
		}
	}
	return retErr
}
