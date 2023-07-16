// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LogReader keeps a persistent view of the log files created by a VM, and
// can be used to save them to an output directory for tast.
type LogReader struct {
	vmName  string
	ownerID string
	reader  *syslog.LineReader
}

// TrySaveAllVMLogs tries to save all VM logs.
func TrySaveAllVMLogs(ctx context.Context, user, dir string) error {
	ownerID, err := cryptohome.UserHash(ctx, user)
	if err != nil {
		return err
	}
	// Wait 1s for the files to be available.
	testing.Sleep(ctx, time.Second)

	logDir := "/run/daemon-store/crosvm/" + ownerID + "/log/"
	logFiles, err := os.ReadDir(logDir)
	if err != nil {
		return errors.Wrap(err, "failed to read logs in /run/daemon-store/crosvm/")
	}

	for _, file := range logFiles {
		if !strings.Contains(file.Name(), ".log") {
			continue
		}
		fileName := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		decodedName, err := base64.URLEncoding.WithPadding(base64.StdPadding).DecodeString(fileName)
		if err != nil {
			return errors.Wrapf(err, "failed to decode log file name: %s", fileName)
		}
		vmName := string(decodedName)
		reader, err := syslog.NewLineReader(ctx, filepath.Join(logDir, file.Name()), true,
			&testing.PollOptions{Timeout: 1 * time.Second})
		if err != nil {
			return errors.Wrapf(err, "failed to start a new LineReader that reports log messages for %s", vmName)
		}
		logReader := &LogReader{vmName, ownerID, reader}
		defer logReader.Close()
		if err := logReader.TrySaveLogs(ctx, dir); err != nil {
			return errors.Wrapf(err, "failed to save log for %s", vmName)
		}
	}
	return nil
}

// Close closes the underlying syslog.LineReader.
func (r *LogReader) Close() error {
	return r.reader.Close()
}

// TrySaveLogs attempts to save the VM logs to the given directory. The logs
// will be saved in a file called "<vm name>_logs.txt".
func (r *LogReader) TrySaveLogs(ctx context.Context, dir string) error {
	path := filepath.Join(dir, r.vmName+"_logs.txt")
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "failed creating log file at %v", path)
	}
	defer f.Close()

	for {
		line, err := r.reader.ReadLine()
		if err != nil {
			if err != io.EOF {
				return errors.Wrapf(err, "failed to read file %v", path)
			}
			break
		}
		_, err = f.WriteString(line)
		if err != nil {
			return errors.Wrapf(err, "failed to write %q to file %v", line, path)
		}
	}
	return nil
}
