// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This functionality attempts to gather Lacros log files relevant to the
// current test and place them in the test's output directory. It does this
// by filtering log files based on the test's start time.
//
// This is a best-effort attempt. There might be edge cases where relevant
// log entries fall outside of the captured files, or where additional
// entries are collected. In particular, the chrome.TryReuseSession and
// chrome.ForceReuseSession options are not supported.
//
// For a guaranteed and complete log record, refer to `system_logs/lacros`.

package lacros

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// SystemLogDir is the directory that contains Lacros logs.
	SystemLogDir = "/var/log/lacros/"

	// LogFileTimestampFormat is the timestamp format of Lacros log filenames.
	LogFileTimestampFormat = "060102-150405"
	// LogTimestampFormat is the timestamp format inside Lacros logs.
	LogTimestampFormat = "2006-01-02T15:04:05.000000Z"
)

// Warning message.
const msgLacrosLogsIncomplete = "Per-test lacros logs may be incomplete, please check system_logs/lacros instead"

// SaveLogsAfter writes Lacros log files with event happening after startTime.
// This is used to select the Lacros logs relevant to the current test and
// copy them in the output directory.
func SaveLogsAfter(ctx context.Context, outDir string, startTime time.Time) (retErr error) {
	// Print warning message on error.
	defer func() {
		if retErr != nil {
			testing.ContextLog(ctx, msgLacrosLogsIncomplete)
		}
	}()

	// If startTime wasn't set, abort.
	if startTime.IsZero() {
		testing.ContextLog(ctx, "Per-test Lacros logs are not supported in the chrome reuse case")
		return nil
	}

	// Get the list of relevant log files in reverse chronological order.
	logFiles, err := lacrosLogFilesFromSystemDirAfter(ctx, startTime)
	if err != nil {
		return errors.Wrap(err, "failed to list relevant Lacros log files")
	}

	// Copy files to outDir.
	for _, origFilePath := range logFiles {
		fileName := filepath.Base(origFilePath)
		destFilePath := filepath.Join(outDir, fileName)
		// Check if a file with the same name already exists.
		if _, err := os.Stat(destFilePath); err == nil {
			if fileName == "lacros.log" {
				// If a lacros.log file already exists in the test directory, we need to
				// first replace it with its rotated version inside the system log directory.
				if err := replaceLacrosLog(ctx, destFilePath); err != nil {
					return errors.Wrapf(err, "failed to replace %v with its rotated version", destFilePath)
				}
			} else {
				// A different log file with the same name exists.
				return errors.Errorf("file already exists in output directory: %s", destFilePath)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return errors.Wrapf(err, "failed to access log file: %v", destFilePath)
		}
		if err := fsutil.CopyFile(origFilePath, destFilePath); err != nil {
			return errors.Wrapf(err, "failed to copy %v to %v", origFilePath, destFilePath)
		}
	}
	return nil
}

// replaceLacrosLog replaces the given lacros.log file with its rotated version inside the system log directory.
// This situation can arise if Ash Chrome is restarted e.g. with `chrome.New` during a test.
// In that case, the most recent lacros.log file would have been copied to the test's out directory
// at the end of the first execution.
// When the second execution of Ash Chrome starts, and Lacros is subsequently launched,
// a new lacros.log file is generated, and the previous log is rotated by Lacros.
// This function identifies the rotated log, so that we can keep its name and content in sync
// inside the test's out directory.
func replaceLacrosLog(ctx context.Context, lacrosLogPath string) error {
	destDir := filepath.Dir(lacrosLogPath)
	systemLogFilePath, err := findRotatedLogFile(lacrosLogPath)
	if err != nil {
		return errors.Wrap(err, "failed to find log in system path")
	}

	newLogFilePath := filepath.Join(destDir, filepath.Base(systemLogFilePath))
	if err := fsutil.CopyFile(systemLogFilePath, newLogFilePath); err != nil {
		return errors.Wrapf(err, "failed to move file %s to %s", systemLogFilePath, newLogFilePath)
	}
	if err := os.Remove(lacrosLogPath); err != nil {
		return errors.Wrapf(err, "failed to remove file %s", lacrosLogPath)
	}

	return nil
}

// findRotatedLogFile tries to find the rotated version of the given lacros.log file,
// using the first line of the log file to identify it.
func findRotatedLogFile(logFilePath string) (string, error) {
	// Get the first line of the given log file.
	logFirstLine, err := firstLine(logFilePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read first line from %s", logFilePath)
	}
	// Find all existing Lacros system log files.
	logFiles, err := allLacrosLogFilesFromSystemDir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get log files list")
	}
	// Find the file that starts with the same line.
	for _, systemLogFilePath := range logFiles {
		systemLogFirstLine, err := firstLine(systemLogFilePath)
		if err != nil {
			return "", errors.Wrapf(err, "failed to read first line from %s", systemLogFilePath)
		}
		if logFirstLine == systemLogFirstLine {
			return systemLogFilePath, nil
		}
	}
	return "", errors.Errorf("failed to find the rotated version of %s", logFilePath)
}

// lacrosLogFilesFromSystemDirAfter returns the list of Lacros log files with events happening after startTime.
func lacrosLogFilesFromSystemDirAfter(ctx context.Context, startTime time.Time) (filteredFiles []string, _ error) {
	files, err := allLacrosLogFilesFromSystemDir()
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if firstTimestamp, err := firstTimestamp(file); err == nil {
			if firstTimestamp.IsZero() {
				// Empty file.
				continue
			}
			if !firstTimestamp.Before(startTime) {
				// If the first timestamp in the file comes after startTime,
				// add the log file to the list.
				filteredFiles = append(filteredFiles, file)
			} else {
				// Otherwise, given that the list is in reverse chronological order,
				// none of the subsequent files needs to be considered.
				break
			}
		} else {
			// Unable to parse the log file for some reason other than "empty file".
			testing.ContextLog(ctx, "Failed parsing Lacros log file: ", file)
		}
	}
	return filteredFiles, nil
}

// allLacrosLogFilesFromSystemDir returns all Lacros log files, in reverse chronological order
// (from the most to the least recent).
func allLacrosLogFilesFromSystemDir() ([]string, error) {
	// Get all timestamped Lacros logs.
	pattern := filepath.Join(SystemLogDir, "lacros_*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list files with pattern %v", pattern)
	}

	// Sort them in reverse chronological order (from the most to the least recent).
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	// Handle the special case of lacros.log, the most recent file that hasn't been rotated yet.
	latestFile := filepath.Join(SystemLogDir, "lacros.log")
	if _, err := os.Stat(latestFile); err == nil {
		files = append([]string{latestFile}, files...)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return files, errors.Wrap(err, "failed to access lacros.log")
	}

	return files, nil
}

// firstTimestamp returns the timestamp of the first line in a log file.
func firstTimestamp(filePath string) (time.Time, error) {
	firstLine, err := firstLine(filePath)
	if err != nil {
		return time.Time{}, err
	}

	// Extract the timestamp.
	timestampEndIndex := strings.Index(firstLine, " ")
	if timestampEndIndex == -1 {
		return time.Time{}, errors.Errorf("failed parsing timestamp line: %v", firstLine)
	}

	// Parse the timestamp.
	timestampString := firstLine[:timestampEndIndex]
	timestamp, err := time.Parse(LogTimestampFormat, timestampString)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "failed parsing timestamp: %v", timestampString)
	}
	return timestamp, nil
}

// firstLine returns the first line in a log file.
func firstLine(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open %v", filePath)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Check if there's a line to read, and if there is, return it.
	if scanner.Scan() {
		return scanner.Text(), nil
	}

	// Handle error or EOF.
	if err := scanner.Err(); err != nil {
		return "", errors.Wrapf(err, "failed reading first line of %v", filePath)
	}
	return "", errors.Errorf("file %v is empty", filePath)
}
