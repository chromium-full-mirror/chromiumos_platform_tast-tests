// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/tast-tests/cros/remote/firmware"
)

func TestRestoreAPRWViaServoErrors(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "bios.bak")
	if err := os.WriteFile(backupPath, []byte("backup"), 0644); err != nil {
		t.Fatalf("failed to create temp backup file: %v", err)
	}

	mgrWithBackup := NewFirmwareBackupManager(t.TempDir())
	mgrWithBackup.backupPaths[FirmwareAP] = backupPath

	for _, tc := range []struct {
		name   string
		mgr    *FirmwareBackupManager
		helper *firmware.Helper
	}{
		{
			name:   "nil_manager",
			mgr:    nil,
			helper: &firmware.Helper{},
		},
		{
			name:   "no_backup_available",
			mgr:    NewFirmwareBackupManager(t.TempDir()),
			helper: &firmware.Helper{},
		},
		{
			name:   "nil_helper",
			mgr:    mgrWithBackup,
			helper: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.mgr.RestoreAPRWViaServo(t.Context(), tc.helper); err == nil {
				t.Error("RestoreAPRWViaServo() succeeded unexpectedly; want non-nil error")
			}
		})
	}
}
