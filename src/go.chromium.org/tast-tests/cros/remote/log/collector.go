// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package log

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast/core/errors"
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
