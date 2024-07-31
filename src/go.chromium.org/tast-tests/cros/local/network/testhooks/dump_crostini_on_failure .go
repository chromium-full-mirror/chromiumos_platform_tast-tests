// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// dumpCrostiniOnFailureHook implements hook interface.
type dumpCrostiniOnFailureHook struct {
	noopTearDownMixin

	crostini     *vm.Container
	errorHandler func(string)
}

func (h *dumpCrostiniOnFailureHook) name() string {
	return "dump_crostini_on_failure"
}

func (h *dumpCrostiniOnFailureHook) setUp(ctx context.Context) error {
	if h.crostini == nil {
		testing.ContextLogf(ctx, "Skip setup of %s due to empty crostini", h.name())
		return nil
	}

	h.errorHandler = func(errMsg string) {
		if err := h.dumpNetworkStates(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to dump network states in crostini on failure: ", err)
		}
	}

	return nil
}

func (h *dumpCrostiniOnFailureHook) dumpNetworkStates(ctx context.Context) error {
	// Creates a file for output.
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get ContextOutDir")
	}

	filename := "network_dump_on_error_crostini_" + time.Now().Format("030405000") + ".txt"

	f, err := os.OpenFile(filepath.Join(dir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var errs []error

	runCmdAndLog := func(tag string, cmd *testexec.Cmd) {
		fullCmd := strings.Join(cmd.Args, " ")
		header := "(" + tag + ") $ " + fullCmd + "\n"
		o, err := cmd.Output(testexec.DumpLogOnError)
		if err != nil {
			errs = append(errs, errors.Wrap(err, "failed to execute '"+fullCmd+"' in "+tag))
			return
		}
		if _, err := f.WriteString(header + string(o) + "\n"); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to write contents for '"+fullCmd+"' in "+tag))
			return
		}
	}

	c := h.crostini

	// Crostini.
	runCmdAndLog("crostini", c.Command(ctx, "ip", "addr"))
	runCmdAndLog("crostini", c.Command(ctx, "ip", "-4", "route"))
	runCmdAndLog("crostini", c.Command(ctx, "ip", "-6", "route"))
	runCmdAndLog("crostini", c.Command(ctx, "cat", "/etc/resolv.conf"))

	// Termina.
	runCmdAndLog("termina", c.VM.Command(ctx, "ip", "addr"))
	runCmdAndLog("termina", c.VM.Command(ctx, "ip", "-4", "route"))
	runCmdAndLog("termina", c.VM.Command(ctx, "ip", "-6", "route"))
	runCmdAndLog("termina", c.VM.Command(ctx, "cat", "/etc/resolv.conf"))

	return errors.Join(errs...)
}

func (h *dumpCrostiniOnFailureHook) onError(errMsg string) {
	if h.errorHandler == nil {
		return
	}
	h.errorHandler(errMsg)
}

func (h *dumpCrostiniOnFailureHook) OnFatal(errMsg string) {
	if h.errorHandler == nil {
		return
	}
	h.errorHandler(errMsg)
}
