// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package drivefs

import (
	"context"
	"io"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LogMatch represents either a match of a pattern against a log line or a failure to read such line.
type LogMatch struct {
	// The Line matching a given regex, if such Line is found.
	Line string

	// An error that happened while trying to read a new line from the logs.
	Err error
}

// LogReader can be used to read Drivefs logs and wait for a line matching a given pattern to be written to those logs.
type LogReader struct {
	logPath string
}

// NewLogReader creates a new LogReader that can be used to watch or wait for a given
// regex pattern in drivefs logs.
func NewLogReader(ctx context.Context, normalizedUser, mountPath string) (*LogReader, error) {
	lr := new(LogReader)
	homeDir, err := cryptohome.UserPath(ctx, normalizedUser)
	if err != nil {
		return nil, errors.Wrap(err, "could not obtain the home dir path")
	}
	persistableToken := PersistableToken(mountPath)
	lr.logPath = ConfigPath(homeDir, persistableToken, "Logs", "drivefs.txt")
	return lr, nil
}

// WatchPattern returns a channel that emits a line matching the given pattern
// when it's written to Drivefs logs. Only the first match is emitted. Matching
// lines that were written to the file before calling WatchPattern won't be emitted.
func (lr LogReader) WatchPattern(ctx context.Context, pattern *regexp.Regexp) (chan LogMatch, error) {
	channel := make(chan LogMatch, 1)

	logReader, err := syslog.NewLineReader(ctx, lr.logPath, false, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read file: %v", lr.logPath)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				close(channel)
				return
			default:
			}

			line, err := logReader.ReadLine()
			if err == io.EOF {
				// GoBigSleepLint: sleep to avoid spending unnecessary cycles polling for new log lines.
				testing.Sleep(ctx, 200*time.Millisecond)
				continue
			}
			if err != nil {
				channel <- LogMatch{Line: "", Err: errors.Wrapf(err, "failed to read line from file %v", lr.logPath)}
				logReader.Close()
				return
			}
			if pattern.MatchString(line) {
				channel <- LogMatch{Line: line, Err: nil}
				logReader.Close()
				return
			}
		}
	}()

	return channel, nil
}
