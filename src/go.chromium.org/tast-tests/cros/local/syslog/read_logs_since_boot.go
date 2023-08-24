// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package syslog

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	auditLogPath   = "/var/log/audit/audit.log"
	upstartLogPath = "/var/log/upstart.log"
	statLogPath    = "/proc/stat"
)

// BootTime returns last boot time.
func BootTime() (time.Time, error) {
	statLogs, err := ioutil.ReadFile(statLogPath)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "failed to read status logs")
	}
	rx := regexp.MustCompile(`btime ([0-9]*)`)
	tout := rx.FindStringSubmatch(string(statLogs))
	if len(tout) != 2 {
		return time.Time{}, errors.Wrap(err, "failed to find btime")
	}
	toutint, err := strconv.ParseInt(tout[1], 10, 64)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "error parsing boot time")
	}
	bootTime := time.Unix(toutint, 0)
	return bootTime, nil
}

// AuditLogsSinceBoot returns all audit logs since last boot.
func AuditLogsSinceBoot(ctx context.Context) ([]string, error) {
	bootTime, err := BootTime()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get boot time")
	}
	auditLogs, err := ioutil.ReadFile(auditLogPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read audit log")
	}
	rx := regexp.MustCompile(`(?:msg=audit\()([0-9]*)\.([0-9]*)`)
	lines := strings.Split(string(auditLogs), "\n")
	for i, l := range lines {
		regMatch := rx.FindStringSubmatch(l)
		if len(regMatch) < 3 {
			continue
		}

		seconds, err := strconv.ParseInt(regMatch[1], 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing audit timestamp seconds")
		}
		nanoseconds, err := strconv.ParseInt(regMatch[2], 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing audit timestamp nanoseconds")
		}

		t := time.Unix(seconds, nanoseconds)
		if bootTime.Before(t) {
			recentLogs := lines[i:]
			return recentLogs, nil
		}
	}
	return nil, nil
}

// getLogsSinceRFCTimestamp parses a slice of strings assuming RFC timestamp is first.
// Example log:
// 2022-09-28T00:12:08.927021Z log message here will be returned.
func getLogsSinceRFCTimestamp(lines []string, bootTime time.Time) ([]string, error) {
	if len(lines) < 1 {
		return nil, errors.New("failure: upstart logs empty")
	}
	for i, l := range lines {
		splitstr := strings.Split(l, " ")
		if len(splitstr) < 1 {
			return nil, errors.New("failed to parse upstart log")
		}
		t, err := time.Parse(time.RFC3339, strings.Trim(splitstr[0], "\x00"))
		if err != nil {
			return nil, err
		}
		if bootTime.Before(t) {
			recentLogs := lines[i:]
			return recentLogs, nil
		}
	}
	return nil, errors.New("failed to return logs")
}

// UpstartLogsSinceBoot returns all upstart logs since last boot.
func UpstartLogsSinceBoot(ctx context.Context) ([]string, error) {
	bootTime, err := BootTime()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get boot time")
	}

	upstartLogs, err := os.ReadFile(upstartLogPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read upstart log")
	}
	lines := strings.Split(string(upstartLogs), "\n")
	logsSinceBoot, err := getLogsSinceRFCTimestamp(lines, bootTime)
	if err != nil {
		// Place upstart file in artifacts for debugging.
		// If test setup failed, then the output failingUpstartLogFile may not exist.
		dir, ok := testing.ContextOutDir(ctx)
		if !ok || dir == "" {
			return nil, errors.New("failed to get CtxOutDir")
		}

		failingUpstartLogFile := filepath.Join(dir, "failingupstart.log")
		source, err := os.Open(upstartLogPath)
		if err != nil {
			return nil, err
		}
		defer source.Close()

		destination, err := os.Create(failingUpstartLogFile)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create failingupstartlog file")
		}
		defer destination.Close()
		_, err = io.Copy(destination, source)
		if err != nil {
			return nil, errors.Wrap(err, "failed to save upstart file")
		}
		return nil, errors.Wrap(err, "failed to parse upstart logs")
	}
	return logsSinceBoot, nil
}
