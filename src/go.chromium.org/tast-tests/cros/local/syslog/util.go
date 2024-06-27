// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package syslog

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast/core/errors"
)

// CollectSyslog collects shards of system log between timing of calling this
// function and each call to the returned function.
func CollectSyslog() (func(context.Context, string) error, error) {
	// Store the current log state.
	oldInfo, mfErr := os.Stat(MessageFile)
	if mfErr == nil {
		return func(ctx context.Context, outDir string) error {
			return collectSysMsg(ctx, outDir, oldInfo)
		}, nil
	}
	// Check for logcat.
	lcErr := exec.Command("which", "logcat").Run()
	if lcErr == nil {
		startTime := time.Now()
		return func(ctx context.Context, outDir string) error {
			return collectLogCat(ctx, outDir, startTime)
		}, nil
	}
	return nil, errors.Join(
		errors.Wrapf(mfErr, "filed to stat %s: %v", MessageFile, mfErr),
		errors.Wrap(lcErr, "logcat not found"),
	)
}

func collectSysMsg(ctx context.Context, outDir string, oldInfo os.FileInfo) error {
	dp := filepath.Join(outDir, filepath.Base(MessageFile))

	df, err := os.Create(dp)
	if err != nil {
		return errors.Wrapf(err, "failed to write log: failed to create %s", dp)
	}
	defer df.Close()

	sf, err := os.Open(MessageFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read log: failed to open %s", MessageFile)
	}
	defer sf.Close()

	info, err := sf.Stat()
	if err != nil {
		return errors.Wrapf(err, "failed reading log position: failed to stat %s", MessageFile)
	}

	if os.SameFile(info, oldInfo) {
		// If the file has not rotated just copy everything since the test started.
		if _, err = sf.Seek(oldInfo.Size(), 0); err != nil {
			return errors.Wrapf(err, "failed to read log: failed to seek %s", MessageFile)
		}

		if _, err = io.Copy(df, sf); err != nil {
			return errors.Wrapf(err, "failed to write log: failed to copy %s", MessageFile)
		}
	} else {
		// If the log has rotated copy the old file from where the test started and then copy the entire new file.
		// We assume that the log does not rotate twice during one test.
		// If we fail to open the older log, we still copy the newer one.
		previousLog := MessageFile + ".1"

		sfp, err := os.Open(previousLog)
		if err != nil {
			_, _ = io.Copy(df, sf)

			return errors.Wrapf(err, "failed to read log: failed to open %s", previousLog)
		}
		defer sfp.Close()

		if _, err = sfp.Seek(oldInfo.Size(), 0); err != nil {
			_, _ = io.Copy(df, sf)

			return errors.Wrapf(err, "failed to read log: failed to seek %s", previousLog)
		}

		// Copy previous log.
		if _, err = io.Copy(df, sfp); err != nil {
			_, _ = io.Copy(df, sf)

			return errors.Wrapf(err, "failed to write log: failed to copy previous %s", previousLog)
		}

		// Copy current log.
		if _, err = io.Copy(df, sf); err != nil {
			return errors.Wrapf(err, "failed to write log: failed to copy current %s", previousLog)
		}
	}
	return nil
}

func collectLogCat(ctx context.Context, outDir string, startTime time.Time) error {
	dp := filepath.Join(outDir, "logcat")

	df, err := os.Create(dp)
	if err != nil {
		return errors.Wrapf(err, "failed to write log: failed to create %s", dp)
	}
	defer df.Close()

	cmd := exec.Command("logcat", "-t", fmt.Sprintf("%d", startTime.Unix()))
	cmd.Stdout = df
	err = cmd.Run()
	if err != nil {
		return errors.Wrap(err, "failed to run logcat")
	}
	return nil
}

// ExtractFileName extracts source file name from Entry.
// If there are multiple file names, it extracts the last one.
func ExtractFileName(entry Entry) string {
	r := regexp.MustCompile(`^.*!?\[(?P<filename>\S+)\([-]?\d+\)\].*$`)
	m := r.FindStringSubmatch(entry.Content)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
