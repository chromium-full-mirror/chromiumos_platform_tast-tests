// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package netexport

import (
	"fmt"
	"os"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
)

const (
	// commandLineArg is an arg that we can pass to either Chrome (Ash) or Lacros
	// Chrome to start networking logging to the given log file.
	commandLineArg = "--log-net-log"

	// chromeCommandLineFile is the file name we specify on Chrome (Ash) startup.
	chromeCommandLineFile = "/tmp/chrome-net-export.json"
	// lacrosCommandLineFile is the file name we specify on Lacros Chrome startup.
	lacrosCommandLineFile = "/tmp/lacros-net-export.json"
)

// CommandLineArgs returns chrome opts with command line args to start chrome
// with network logging enabled.
func CommandLineArgs(bt browser.Type) []chrome.Option {
	// Always pass the `--log-net-log` arg to the ash binary.
	opts := []chrome.Option{chrome.ExtraArgs(fmt.Sprintf("%s=%s", commandLineArg, chromeCommandLineFile))}
	// In lacros, pass the `--log-net-log` arg to the lacros binary as well.
	if bt == browser.TypeLacros {
		opts = append(opts, chrome.LacrosExtraArgs(fmt.Sprintf("%s=%s", commandLineArg, lacrosCommandLineFile)))
	}
	return opts
}

// FromCommandLineArg should be used when Chrome is started with the arg:
// `--log-net-log`. This means that the net export session is already
// started and cannot be stopped, and the net log file(s) already exist.
func FromCommandLineArg(bt browser.Type) (*NetExport, error) {
	logFiles := []string{chromeCommandLineFile}
	// Lacros netlog file only exists in lacros mode.
	if bt == browser.TypeLacros {
		logFiles = append(logFiles, lacrosCommandLineFile)
	}

	// Verify the log files exist. We expect to hit this failure case if Chrome
	// was not started with args from `CommandLineArgs()`.
	for _, logFile := range logFiles {
		if _, err := os.Stat(logFile); errors.Is(err, os.ErrNotExist) {
			return nil, errors.Errorf("net log file does not exist = %s", logFile)
		} else if err != nil {
			return nil, errors.Wrap(err, "failed to verify that log files exist")
		}
	}

	return &NetExport{
		logFiles: logFiles,
		// Set controllers to nil, since the net export session is already started.
		controllers: nil,
	}, nil

}
