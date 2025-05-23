// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/testing"
)

// PostTimeout is the standard time reserved for post-test tasks.
var PostTimeout = 30 * time.Second

// RunCrostiniPostTest runs hooks that should run after every test but before
// the precondition closes (if it's going to) e.g. collecting logs from the
// container.
func RunCrostiniPostTest(ctx context.Context, p PreData) {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		testing.ContextLog(ctx, "Failed to get name of directory")
		return
	}

	// If we haven't connected to chrome successfully, then the
	// test didn't get to do anything with the VM that could
	// possibly have generated logs, and even if it did we
	// couldn't access them, so bail out here.
	if p.Chrome == nil {
		testing.ContextLog(ctx, "Failed before connecting to chrome, no logs generated")
		return
	}

	// Container logs require a running VM and container. If one
	// hasn't been set, we can't fetch them.
	if p.Container != nil {
		trySaveContainerLogs(ctx, dir, p.Container)

		if err := p.Container.Cleanup(ctx, "."); err != nil {
			testing.ContextLog(ctx, "Failed to remove all files in home directory in the container: ", err)
		}
	} else {
		testing.ContextLog(ctx, "No active container, can't get journalctl logs")
	}

	// LXC logs only require a running VM, so even if the
	// container hasn't been set we can try to get a running VM
	// and use that.
	var machine *vm.VM
	if p.Container != nil {
		machine = p.Container.VM
	} else {
		machine2, err := vm.GetRunningVM(ctx, p.Chrome.NormalizedUser(), vm.Termina)
		machine = machine2
		if err != nil {
			testing.ContextLog(ctx, "Failed to get running VM, won't get LXC logs: ", err)
		}
	}
	if machine != nil {
		writeLXCLogs(ctx, dir, machine)
	}

	// VM logs are stored on the host, so we don't need the VM to
	// be running at all to get them.
	if err := vm.TrySaveAllVMLogs(ctx, p.Chrome.NormalizedUser(), dir); err != nil {
		testing.ContextLog(ctx, "Failed to save VM logs: ", err)
	}
}

// RunBaguettePostTest runs hooks that should run after every test but before
// the precondition closes (if it's going to) e.g. collecting logs from the
// guest.
func RunBaguettePostTest(ctx context.Context, p PreData) {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		testing.ContextLog(ctx, "Failed to get name of directory")
		return
	}

	// If we haven't connected to chrome successfully, then the
	// test didn't get to do anything with the VM that could
	// possibly have generated logs, and even if it did we
	// couldn't access them, so bail out here.
	if p.Chrome == nil {
		testing.ContextLog(ctx, "Failed before connecting to chrome, no logs generated")
		return
	}

	// VM logs are stored on the host, so we don't need the VM to
	// be running at all to get them.
	if err := vm.TrySaveAllVMLogs(ctx, p.Chrome.NormalizedUser(), dir); err != nil {
		testing.ContextLog(ctx, "Failed to save VM logs: ", err)
	}
}

// When we run trySaveContainerLogs we only want to capture logs since we last
// ran i.e. from the test that just finished, not all logs since the start of
// the suite. Sadly, Debian's journalctl in stable is too old to support cursor
// files, so we have to parse a cursor out of the log stream and remember it
// between calls to trySaveContainerLogs.
var cursor string

// trySaveContainerLogs fetches new (i.e. since last time the function
// successfully ran) logs from the container and writes them to
// crostini_journalctl.txt
func trySaveContainerLogs(ctx context.Context, dir string, cont *vm.Container) {
	if cont == nil {
		testing.ContextLog(ctx, "No active container")
		return
	}
	args := []string{"sudo", "journalctl", "--no-pager", "--show-cursor"}
	if cursor != "" {
		args = append(args, "--cursor")
		args = append(args, cursor)
	}
	cmd := cont.Command(ctx, args...)
	output, err := cmd.Output()
	if err != nil {
		testing.ContextLog(ctx, "Error running journalctl: ", err)
		return
	}

	path := filepath.Join(dir, "crostini_journalctl.txt")
	err = os.WriteFile(path, output, 0644)
	if err != nil {
		testing.ContextLog(ctx, "Error writing journalctl to log: ", err)
		return
	}

	cursorMarker := []byte("-- cursor: ")
	pos := bytes.LastIndex(output, cursorMarker)
	if pos == -1 {
		testing.ContextLog(ctx, "No journalctl cursor found")
		return
	}
	cursor = string(output[pos+len(cursorMarker):])
}

func writeLXCLogs(ctx context.Context, dir string, machine *vm.VM) {
	path := filepath.Join(dir, "crostini_logs.txt")
	testing.ContextLog(ctx, "Creating crostini log file at ", path)
	f, err := os.Create(path)
	if err != nil {
		testing.ContextLog(ctx, "Error creating crostini log file: ", err)
		return
	}
	defer f.Close()

	f.WriteString("lxc info and lxc.log:\n")

	result, err := machine.LXCCommandCombined(ctx, "info penguin --show-log")
	if err != nil {
		testing.ContextLog(ctx, "Error getting lxc logs: ", err)
	}
	f.WriteString(string(result) + "\n")

	f.WriteString("\n\nconsole.log:\n")
	result, err = machine.LXCCommandCombined(ctx, "console penguin --show-log")
	if err != nil {
		testing.ContextLog(ctx, "Error getting boot logs: ", err)
	}
	f.WriteString(string(result) + "\n")
}
