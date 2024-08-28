// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package log

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Buffer is a buffer for storing logs that supports dumping its contents.
type Buffer struct {
	lock sync.Mutex
	buf  bytes.Buffer
}

// Write writes d into the Buffer.
func (b *Buffer) Write(d []byte) (int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buf.Write(d)
}

// Reset resets the buffer.
func (b *Buffer) Reset() {
	b.lock.Lock()
	b.buf.Reset()
	b.lock.Unlock()
}

// Dump copies the Buffer to w and resets the Buffer.
func (b *Buffer) Dump(w io.Writer) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if _, err := b.buf.WriteTo(w); err != nil {
		return err
	}
	b.buf.Reset()
	return nil
}

// Collector collects log messages in a buffer that may be dumped when desired.
type Collector interface {
	// Dump copies the contents collected to w and resets the log buffer.
	Dump(w io.Writer) error

	// Reset resets log buffer, clearing any previously collected logs since the
	// start of log collection or the last Dump call.
	Reset()

	// Close stops the collector and resets the log buffer.
	Close() error
}

// DumpCollectedLogsToFile will dump the buffer of the logCollector to a new
// log file. The file is saved to the specified directory under the current
// output directory for ctx. The filename is constructed with BuildLogFilename.
func DumpCollectedLogsToFile(ctx context.Context, logCollector Collector, contextualOutputDirPath, logName string) error {
	// Prepare output file.
	dstLogFilename := BuildLogFilename(logName)
	dstFilePath := filepath.Join(contextualOutputDirPath, dstLogFilename)
	f, err := fileutils.PrepareOutDirFile(ctx, dstFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to prepare output dir file %q", dstFilePath)
	}
	// Dump buffer of collected logs to file.
	if err := logCollector.Dump(f); err != nil {
		return errors.Wrapf(err, "failed to dump logs to %q", dstFilePath)
	}
	return nil
}

// BuildLogFilename builds a log filename with a minimal timestamp prefix, all
// the name parts in the middle delimited by "_" with non-word characters
// replaced with underscores, and a ".log" file extension.
//
// This not only communicates the time of the log to users, but keeps similar
// files in chronological order within the same directory when displayed sorted
// by name (alphanumerical order) by most programs.
//
// Example result: "20220523-122753_dbus_bluetooth_PostTest"
func BuildLogFilename(nameParts ...string) string {
	// Build timestamp prefix.
	timestamp := time.Now().Format("20060102-150405")
	// Join and sanitize name parts.
	name := strings.Join(nameParts, "_")
	name = regexp.MustCompile("\\W").ReplaceAllString(name, "_")
	name = regexp.MustCompile("_+").ReplaceAllString(name, "_")
	// Combine timestamp, name, and extension.
	if name != "" {
		name = "_" + name
	}
	return fmt.Sprintf("%s%s.log", timestamp, name)
}

// Collect copies the logs from the DUT to the host in context_out_dir.
// It copies chrome, messages, fakedms and ui logs by default.
func Collect(ctx context.Context, dut *dut.DUT) {
	logsToCopy := []string{
		"/var/log/chrome",
		"/var/log/messages",
		"/var/enrolling-fdms",
		"/var/log/ui",
	}
	fileutils.CopyFromDUTToHost(ctx, dut, logsToCopy)
}

// logMergeConfig defines the configuration for merging a specific type of log.
type logMergeConfig struct {
	pattern         string
	combinedDirName string
	// copyUnique specifies whether to copy unique files names
	// in the combined directory. If false, a counter is appended to
	// file names to avoid overwriting.
	copyUnique bool
}

// MergeLogs merges all logs under output dir from different timestamps
// into a single dir, based on the logMergeConfig configs.
// For example, it merges all "chrome_*" files in different directories
// into "combined_chrome_logs" directory.
func MergeLogs(ctx context.Context) {
	// Configuration for log merging.
	logConfigs := []logMergeConfig{
		{
			pattern:         `chrome_\d{6}-\d{6}`,
			combinedDirName: "combined_chrome_logs",
			copyUnique:      true,
		},
		{
			pattern:         `ui\.\d{8}-\d{6}`,
			combinedDirName: "combined_ui_logs",
			copyUnique:      true,
		},
		{
			pattern:         `messages`,
			combinedDirName: "combined_messages",
			copyUnique:      false, // will copy all files with name "messages".
		},
		{
			pattern:         `fakedms.log`,
			combinedDirName: "combined_fakedms_logs",
			copyUnique:      false, // will copy all files with name "fakedms.log".
		},
	}

	ctxOutDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		testing.ContextLog(ctx, "Failed to get the output directory in context")
		return
	}

	for _, config := range logConfigs {
		if err := mergeLogsByPattern(ctx, ctxOutDir, config.pattern, config.combinedDirName, config.copyUnique); err != nil {
			testing.ContextLogf(ctx, "Failed to merge logs into %s: %v", config.combinedDirName, err)
		}
	}
}

// mergeLogsByPattern merges log files matching the given pattern into a combined directory.
func mergeLogsByPattern(ctx context.Context, outDir, pattern, combinedDirName string, copyUnique bool) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return errors.Wrapf(err, "failed to compile pattern %q", pattern)
	}

	combinedLogsDir := filepath.Join(outDir, combinedDirName)
	if err := os.MkdirAll(combinedLogsDir, 0777); err != nil {
		return errors.Wrapf(err, "failed to create combined logs directory %q", combinedLogsDir)
	}

	if err := filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the combined logs directory itself.
		if info.IsDir() && info.Name() == combinedDirName {
			return filepath.SkipDir
		}

		if !info.IsDir() && regex.MatchString(info.Name()) {
			var destFilePath string
			if copyUnique {
				// If copyUnique is true, only copy if the file doesn't exist.
				destFilePath = filepath.Join(combinedLogsDir, info.Name())
				if _, err := os.Stat(destFilePath); err == nil {
					// File already exists, skip copying.
					testing.ContextLogf(ctx, "File %s already exists in %s, skipping", info.Name(), combinedDirName)
					return nil
				} else if !os.IsNotExist(err) {
					// Some other error occurred.
					return errors.Wrapf(err, "failed to stat file %q", destFilePath)
				}
			} else {
				// If copyUnique is false, append the current timestamp to make it unique.
				destFilePath = filepath.Join(combinedLogsDir, info.Name()+"_"+time.Now().UTC().Format(time.RFC3339Nano))
			}

			// Move the log file to the combined directory.
			if err := os.Rename(path, destFilePath); err != nil {
				return errors.Wrapf(err, "failed to move log file from %s to %s", path, destFilePath)
			}
		}
		return nil
	}); err != nil {
		return errors.Wrapf(err, "failed to walk through output directory %q", outDir)
	}

	return nil
}
