// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// TmpPrefs tracks the state of the overridden prefs for powerd.
type TmpPrefs struct {
	tmpDirPath string
}

// WaitForPrefState allows callers to perform additional synchronization for
// CommitTmpPrefs.
type WaitForPrefState func(context.Context) error

const powerdPrefsDir = "/var/lib/power_manager/"
const powerdJob = "powerd"
const tmpPrefSubDir = "powerd_pref_subdir"

// InitTmpPrefs sets up a temp directory for powerd prefs where temporary pref
// values may be written.
func (prefs *TmpPrefs) InitTmpPrefs(ctx context.Context) (CleanupCallback, error) {
	dir, err := os.MkdirTemp("/tmp", "powerd_prefs")
	if err != nil {
		testing.ContextLog(ctx, "Failed to create tmp directory for powerd prefs: ", err)
		return nil, err
	}
	// Add a sub-directory since CopyDir requires that the destination directory
	// not exist.
	dir = filepath.Join(dir, tmpPrefSubDir)
	if err := fsutil.CopyDir(powerdPrefsDir, dir); err != nil {
		testing.ContextLogf(ctx, "Failed to copy directory context from %s to %s: %s", powerdPrefsDir, dir, err)
		return nil, err
	}
	if err := testexec.CommandContext(ctx, "mount", "--bind", dir, powerdPrefsDir).Run(); err != nil {
		testing.ContextLogf(ctx, "Failed to mount directory %s as %s: %s", dir, powerdPrefsDir, err)
		return nil, err
	}
	prefs.tmpDirPath = dir

	return func(ctx context.Context) error {
		if err := testexec.CommandContext(ctx, "umount", powerdPrefsDir).Run(); err != nil {
			testing.ContextLogf(ctx, "Failed to unmount tmp dir %s from %s: %s", prefs.tmpDirPath, powerdPrefsDir, err)
			return err
		}
		if err := os.RemoveAll(prefs.tmpDirPath); err != nil {
			testing.ContextLogf(ctx, "Failed to remove tmp dir %s: %s", prefs.tmpDirPath, err)
			return err
		}
		if upstart.JobExists(ctx, powerdJob) {
			if err := upstart.RestartJob(ctx, powerdJob); err != nil {
				testing.ContextLog(ctx, "Failed to restart powerd job: ", err)
				return err
			}
			// Just create a PowerManager object, which will ensure that powerd has
			// started DBus.
			if _, err := power.NewPowerManager(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to connect to PowerManager DBus interface after restarting powerd: ", err)
				return err
			}
		}
		return nil
	}, nil
}

// SetTmpPref creates a temporary pref for powerd, until the CleanupCallback for
// TmpPrefs is called.
func (prefs *TmpPrefs) SetTmpPref(ctx context.Context, pref, val string) error {
	prefPath := filepath.Join(prefs.tmpDirPath, pref)
	if err := os.WriteFile(prefPath, []byte(val), 0644); err != nil {
		testing.ContextLogf(ctx, "Failed to write %s to file %s: %s", val, prefPath, err)
		return err
	}
	return nil
}

// CommitTmpPrefs ensures that updated pref values take effect for powerd and
// the system. If powerd is currently stopped, it will be started then stopped
// after waiting for new pref values to take effect (some prefs change system
// behavior, even after powerd is stopped). Otherwise, powerd will just be
// restarted.
func (prefs *TmpPrefs) CommitTmpPrefs(ctx context.Context, waitFunc WaitForPrefState) error {
	if upstart.JobExists(ctx, powerdJob) {
		if err := upstart.RestartJob(ctx, powerdJob); err != nil {
			testing.ContextLog(ctx, "Failed to restart powerd job: ", err)
			return err
		}
	} else {
		if err := upstart.StartJob(ctx, powerdJob); err != nil {
			testing.ContextLog(ctx, "Failed to temporarily start powerd job: ", err)
			return err
		}
		// Stop powerd if it wasn't running after waitFunc finishes.
		defer func(c context.Context) {
			if err := upstart.StopJob(c, powerdJob); err != nil {
				testing.ContextLog(ctx, "Failed to stop powerd job: ", err)
			}
		}(ctx)
	}

	if _, err := power.NewPowerManager(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to connect to PowerManager DBus interface after restarting powerd: ", err)
		return err
	}
	if waitFunc != nil {
		return waitFunc(ctx)
	}
	return nil
}
