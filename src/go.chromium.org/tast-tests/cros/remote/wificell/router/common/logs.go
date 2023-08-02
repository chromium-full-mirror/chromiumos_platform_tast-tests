// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/log"
	"go.chromium.org/tast-tests/cros/remote/wificell/fileutil"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// StartTailLogCollectors starts log collectors with log.StartTailCollector.
func StartTailLogCollectors(ctx context.Context, host *ssh.Conn, logsToCollect []string, tailFollowNameSupported bool) (map[string]*log.TailCollector, error) {
	logCollectors := make(map[string]*log.TailCollector)
	for _, p := range logsToCollect {
		logger, err := log.StartTailCollector(ctx, host, p, tailFollowNameSupported)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start log collector")
		}
		logCollectors[p] = logger
	}
	return logCollectors, nil
}

// StopTailLogCollectors closes all log collectors spawned.
func StopTailLogCollectors(ctx context.Context, logCollectors map[string]*log.TailCollector) error {
	var firstErr error
	for _, c := range logCollectors {
		if err := c.Close(); err != nil {
			utils.CollectFirstErr(ctx, &firstErr, err)
		}
	}
	return firstErr
}

// CollectRouterFileLogs dumps collected logs from files on the router to
// "$contextDir/routers/$router/$filename" with a suffix appended to the
// filenames and a timestamp prepended to the filename.
func CollectRouterFileLogs(ctx context.Context, r support.Router, logCollectors map[string]*log.TailCollector, logsToCollect []string, suffix string) error {
	ctx, st := timing.Start(ctx, "CollectRouterFileLogs")
	defer st.End()

	logDir := BuildRouterLogDirName(r)

	var firstErr error
	for _, src := range logsToCollect {
		collector := logCollectors[src]
		logName := filepath.Base(src) + suffix
		if collector == nil {
			testing.ContextLogf(ctx, "No log collector for %s found", src)
			utils.CollectFirstErr(ctx, &firstErr, errors.Errorf("failed to find log collector %q", src))
			continue
		}
		if err := log.DumpCollectedLogsToFile(ctx, collector, logDir, logName); err != nil {
			testing.ContextLogf(ctx, "Failed to collect %q, err: %v", src, err)
			utils.CollectFirstErr(ctx, &firstErr, errors.Wrapf(err, "failed to collect %q", src))
		}
	}
	return firstErr
}

// CollectRouterLogs writes the collected logs to a log file. An optional
// logName may be specified.
func CollectRouterLogs(ctx context.Context, r support.Router, logCollector log.Collector, logName string) error {
	ctx, st := timing.Start(ctx, "CollectRouterLogs")
	defer st.End()
	logDir := BuildRouterLogDirName(r)
	return log.DumpCollectedLogsToFile(ctx, logCollector, logDir, logName)
}

// BuildRouterLogDirName builds the contextual log dir path meant to store all
// log files related to the router.
func BuildRouterLogDirName(r support.Router) string {
	return filepath.Join("routers", r.RouterName())
}

// LogRouterFile writes a file to under the contextual log dir for the router.
func LogRouterFile(ctx context.Context, r support.Router, dstLogFilename string, fileContents []byte) error {
	contextualOutputDirPath := BuildRouterLogDirName(r)
	dstFilePath := filepath.Join(contextualOutputDirPath, dstLogFilename)
	f, err := fileutil.PrepareOutDirFile(ctx, dstFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to prepare output dir file %q", dstFilePath)
	}
	if _, err := f.Write(fileContents); err != nil {
		return errors.Wrapf(err, "failed to write log file to output dir file %q", dstFilePath)
	}
	return nil
}
