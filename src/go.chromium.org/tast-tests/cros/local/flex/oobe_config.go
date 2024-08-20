// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"
	"os"

	"go.chromium.org/tast/core/errors"
)

const (
	// FlexConfigInitialFilePath is the file path where Flex OOBE config exists initially
	// (in unencrypted stateful partition).
	FlexConfigInitialFilePath = "/mnt/stateful_partition/unencrypted/flex_config/config.json"

	// FlexConfigESPFilePath is the file path where Flex OOBE config exists after the
	// config is moved to encrypted stateful partition (ESP) on oobe_config_restore startup.
	FlexConfigESPFilePath = "/var/lib/oobe_config_restore/flex_config/config.json"

	flexConfigInitialDirPath = "/mnt/stateful_partition/unencrypted/flex_config"
	flexConfigESPDirPath     = "/var/lib/oobe_config_restore/flex_config"

	oobeConfigRestoreUID = 20121
	oobeConfigRestoreGID = 20121
)

// WriteFlexOobeConfigToDisk creates the OOBE config directory and file for Flex token-based
// enrollment.
func WriteFlexOobeConfigToDisk(oobeConfig string) error {
	if err := os.Mkdir(flexConfigInitialDirPath, 0740); err != nil {
		return errors.Wrapf(err, "failed to create %s directory", flexConfigInitialDirPath)
	}
	if err := os.WriteFile(FlexConfigInitialFilePath, []byte(oobeConfig), 0640); err != nil {
		return errors.Wrapf(err, "failed to create %s", FlexConfigInitialFilePath)
	}
	if err := os.Chown(FlexConfigInitialFilePath, oobeConfigRestoreUID, oobeConfigRestoreGID); err != nil {
		return errors.Wrapf(err, "failed to chown %s", FlexConfigInitialFilePath)
	}
	if err := os.Chown(flexConfigInitialDirPath, oobeConfigRestoreUID, oobeConfigRestoreGID); err != nil {
		return errors.Wrapf(err, "failed to chown %s", flexConfigInitialDirPath)
	}
	return nil
}

// CleanUpFlexConfigDirs deletes the Flex OOBE config directories and files from both the unencrypted
// and encrypted stateful partition.
func CleanUpFlexConfigDirs(ctx context.Context) error {
	if err := os.RemoveAll(flexConfigInitialDirPath); err != nil {
		return errors.Wrapf(err, "failed to delete Flex config dir %s", flexConfigInitialDirPath)
	}
	if err := os.RemoveAll(flexConfigESPDirPath); err != nil {
		return errors.Wrapf(err, "failed to delete Flex config dir %s", flexConfigESPDirPath)
	}
	return nil
}
