// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dumputil contains common utilities for dumping network state which
// are used by various networking tests.
package dumputil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DumpNetworkInfo dumps debug information about the current network status into
// a log file with |filename| in the context OutDir, and returns the last error
// if there is any.
func DumpNetworkInfo(ctx context.Context, filename string) error {
	// Creates a file for output.
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get ContextOutDir")
	}

	f, err := os.OpenFile(filepath.Join(dir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var errs []error

	runCmdAndLog := func(cmd string, args ...string) {
		fullCmd := cmd + " " + strings.Join(args, " ")
		header := "$ " + fullCmd + "\n"
		o, err := testexec.CommandContext(ctx, cmd, args...).Output()
		if err != nil {
			errs = append(errs, errors.Wrap(err, "failed to execute "+fullCmd))
			return
		}
		if _, err := f.WriteString(header + string(o) + "\n"); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to write contents for "+fullCmd))
			return
		}
	}

	// Dumps iptables.
	for _, iptablesCmd := range []string{"iptables", "ip6tables"} {
		for _, table := range []string{"filter", "nat", "mangle"} {
			// `-n` to avoid reverse DNS lookups since DNS service may not be
			// available in the test environment.
			runCmdAndLog(iptablesCmd, "-L", "-x", "-v", "-t", table, "-n", "-w", "3")
		}
	}

	// Dumps ip-addr.
	runCmdAndLog("ip", "addr")

	// Dumps ip-rule.
	for _, family := range []string{"-4", "-6"} {
		runCmdAndLog("ip", family, "rule")
	}

	// Dumps ip-route.
	for _, family := range []string{"-4", "-6"} {
		runCmdAndLog("ip", family, "route", "list", "table", "all")
	}

	// Dumps conntrack.
	for _, family := range []string{"ipv4", "ipv6"} {
		runCmdAndLog("conntrack", "-L", "-f", family)
	}

	// Dumps socket statistics.
	for _, family := range []string{"-4", "-6"} {
		runCmdAndLog("ss", family, "-api")
	}

	// Dump shill status.
	runCmdAndLog("/usr/local/lib/flimflam/test/list-manager")
	runCmdAndLog("/usr/local/lib/flimflam/test/list-profiles")
	runCmdAndLog("/usr/local/lib/flimflam/test/list-devices")
	runCmdAndLog("/usr/local/lib/flimflam/test/list-connected-services")

	for _, err := range errs {
		testing.ContextLog(ctx, "Failed to run cmd: ", err)
	}

	return errors.Join(errs...)
}

// CreateErrorHandler creates an error handler to dump network info on test
// failures. This should be used together with s.AttachErrorHandlers(). Example:
//
//	errorHandler := dumputil.CreateErrorHandler(cleanupCtx)
//	s.AttachErrorHandlers(errorHandler, errorHandler)
func CreateErrorHandler(ctx context.Context) func(string) {
	return func(string) {
		filename := "network_dump_on_error_" + time.Now().Format("030405000") + ".txt"
		if err := DumpNetworkInfo(ctx, filename); err != nil {
			testing.ContextLog(ctx, "Failed to dump network info on error: ", err)
		} else {
			testing.ContextLog(ctx, "Dumped current network info to ", filename)
		}
	}
}
