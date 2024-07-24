// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/systemlogs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// dumpChromeOnFailureHook implements hook interface.
type dumpChromeOnFailureHook struct {
	noopTearDownMixin

	getChrome    func() *chrome.Chrome
	errorHandler func()
}

func (h *dumpChromeOnFailureHook) name() string {
	return "dump_chrome_on_failure"
}

func dumpNetworkEventLog(ctx context.Context, cr *chrome.Chrome) error {
	if cr == nil {
		testing.ContextLog(ctx, "Skip dumpNetworkEventLog since Chrome is nil")
		return nil
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test API connection")
	}
	logs, err := systemlogs.GetSystemLogs(ctx, tconn, "network_event_log")
	if err != nil {
		return errors.Wrap(err, "failed to get network_event_log section from system logs")
	}
	filename := "chrome_network_event_log_" + time.Now().Format("030405000") + ".txt"
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get out dir of the test")
	}
	f, err := os.OpenFile(filepath.Join(outDir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	defer f.Close()
	if _, err := f.Write([]byte(logs)); err != nil {
		return errors.Wrap(err, "failed to write to log file")
	}
	return nil
}

func (h *dumpChromeOnFailureHook) setUp(ctx context.Context) error {
	h.errorHandler = func() {
		if err := dumpNetworkEventLog(ctx, h.getChrome()); err != nil {
			testing.ContextLog(ctx, "Failed to dump network event log from Chrome: ", err)
		}
	}
	return nil
}

func (h *dumpChromeOnFailureHook) onError(errMsg string) {
	h.errorHandler()
}

func (h *dumpChromeOnFailureHook) OnFatal(errMsg string) {
	h.errorHandler()
}
