// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package drivefs

import (
	"context"
	"os"
	"path"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	mountPointTimeout = 60 * time.Second // timeout waiting for CrosDisks to mount Drivefs.
	// Allow up to 70s to cover both the 1st (20s) and 2nd (+40s = 60s)
	// GlobalPhenotypeExperimentFetcher backoff retries on a clean start.
	fuseIoTimeout = 70 * time.Second // timeout waiting for the FUSE to be operational.

	driveFsLogStartupHeader        = "drivefs log started"
	driveFsLogNoFeatureConfigDiff  = "No change in CoreFeatureConfig for account os_account"
	driveFsLogTokensChanged        = "Experiment tokens changed for account os_account"
	driveFsLogCoreStarted          = "DriveFs core has started with: metrics"
	driveFsLogSafeRestartRequested = "Safe restart requested:"
	driveFsLogScheduledRestart     = "DriveFs scheduled restart."
)

// WaitForDriveFs checks that the Drivefs mount is ready for IO.
// The username supplied must come from a user logged in using GAIA login.
func WaitForDriveFs(ctx context.Context, username string) (string, error) {
	// Attempt to normalize the logged in user
	normUser, err := session.NormalizeEmail(username, true)
	if err != nil {
		return "", errors.Wrap(err, "failed to normalize user name")
	}

	// Check that cache folder was created by cryptohome.
	homePath, err := cryptohome.UserPath(ctx, normUser)
	if err != nil {
		return "", errors.Wrap(err, "failed to get home path")
	}
	cachePath := path.Join(homePath, "GCache", "v2")
	if dir, err := os.Stat(cachePath); !dir.IsDir() {
		return "", errors.Wrapf(err, "failed as cache dir %s is missing", cachePath)
	}

	// It takes some time for request to mount Drive to be handled by CrosDisks
	// that creates the mount point. Poll for a mount point until timeout.
	mounts, err := waitForMatchingMount(ctx, mountPointTimeout, isDriveFs)
	if err != nil {
		return "", errors.Wrap(err, "failed with timeout while waiting for mountpoint creation")
	}
	if len(mounts) != 1 {
		return "", errors.Wrapf(err, "failed one drivefs mount expected found %d. Mounts found: %v", len(mounts), mounts)
	}
	mountPath := mounts[0].MountPath
	testing.ContextLog(ctx, "drivefs is mounted into ", mountPath)

	// On a clean start, DriveFS fetches server feature configs via Phenotype
	// (retrying 20s after an initial 500ms timeout, and backing off to +40s = 60s
	// if the first retry also times out under heavy boot load) and self-restarts
	// to apply them. Poll until the current DriveFS instance has persisted its
	// experiment token, verified its feature config without pending restarts,
	// finished starting core, and mounted the root directory.
	testing.ContextLogf(ctx, "Waiting up to %v for drivefs to stabilize", fuseIoTimeout)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		currMounts, err := findMatchingMount(isDriveFs)
		if err != nil {
			return errors.Wrap(err, "io error trying to list mounts")
		}
		if len(currMounts) != 1 {
			return errors.Errorf("expected one drivefs mount, found %d", len(currMounts))
		}
		mountPath = currMounts[0].MountPath
		if err := checkDriveFsStabilized(homePath, mountPath); err != nil {
			return err
		}
		drivefsRoot := path.Join(mountPath, "root")
		dir, err := os.Stat(drivefsRoot)
		if err != nil {
			return err
		}
		if !dir.IsDir() {
			return errors.Errorf("no root folder inside %s", mountPath)
		}
		return nil
	}, &testing.PollOptions{Timeout: fuseIoTimeout, Interval: time.Second}); err != nil {
		return "", errors.Wrap(err, "failed while waiting for drivefs to stabilize")
	}

	return mountPath, nil
}

// checkDriveFsStabilized verifies that DriveFS has finished its initial
// server experiment/feature config fetch and is not about to self-restart.
func checkDriveFsStabilized(homePath, mountPath string) error {
	persistableToken := PersistableToken(mountPath)
	if persistableToken == "" {
		return errors.Errorf("failed to obtain persistable token from mount path %q", mountPath)
	}

	experimentTokenPath := ConfigPath(homePath, persistableToken, "experiment_token")
	if _, err := os.Stat(experimentTokenPath); err != nil {
		return errors.Wrap(err, "experiment_token not yet persisted")
	}

	logPath := ConfigPath(homePath, persistableToken, "Logs", "drivefs.txt")
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		return errors.Wrap(err, "failed to read drivefs.txt")
	}
	latestLog := string(logBytes)
	if idx := strings.LastIndex(latestLog, driveFsLogStartupHeader); idx != -1 {
		latestLog = latestLog[idx:]
	}

	if strings.Contains(latestLog, driveFsLogSafeRestartRequested) ||
		strings.Contains(latestLog, driveFsLogScheduledRestart) ||
		strings.Contains(latestLog, driveFsLogTokensChanged) {
		return errors.New("drivefs is restarting to apply updated feature config")
	}
	if !strings.Contains(latestLog, driveFsLogNoFeatureConfigDiff) {
		return errors.New("drivefs has not yet verified CoreFeatureConfig for os_account")
	}
	if !strings.Contains(latestLog, driveFsLogCoreStarted) {
		return errors.New("drivefs core has not yet finished starting")
	}
	return nil
}

func findMatchingMount(matcher func(sysutil.MountInfo) bool) (matches []sysutil.MountInfo, err error) {
	info, err := sysutil.MountInfoForPID(sysutil.SelfPID)
	if err != nil {
		return
	}
	for i := range info {
		if matcher(info[i]) {
			matches = append(matches, info[i])
		}
	}
	return
}

func waitForMatchingMount(ctx context.Context, timeout time.Duration, matcher func(sysutil.MountInfo) bool) ([]sysutil.MountInfo, error) {
	var matches []sysutil.MountInfo
	var err error
	testing.ContextLogf(ctx, "Waiting %v for a matching mount to appear", timeout)
	err = testing.Poll(ctx, func(ctx context.Context) error {
		matches, err = findMatchingMount(matcher)
		if err != nil {
			return errors.Wrap(err, "io error trying to list mounts")
		}
		if len(matches) == 0 {
			return errors.Wrap(os.ErrNotExist, "matching mount was not found")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: time.Second})
	return matches, err
}

func isDriveFs(info sysutil.MountInfo) bool {
	return info.Fstype == "fuse.drivefs"
}
