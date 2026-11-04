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

// containerLogsTimeout and containerCleanupTimeout cap guest commands so a
// stalled VM cannot starve subsequent VM log collection or fixture cleanup.
const (
	containerLogsTimeout    = 15 * time.Second
	containerCleanupTimeout = 10 * time.Second
)

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
		trySaveContainerLogs(ctx, dir, p.Container, p.Post)
		cleanupContainerHome(ctx, p.Container)
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

	// Container logs require a running VM and container. If one
	// hasn't been set, we can't fetch them.
	if p.Container != nil {
		trySaveContainerLogs(ctx, dir, p.Container, p.Post)
		cleanupContainerHome(ctx, p.Container)
	} else {
		testing.ContextLog(ctx, "No active container, can't get journalctl logs")
	}

	// VM logs are stored on the host, so we don't need the VM to
	// be running at all to get them.
	if err := vm.TrySaveAllVMLogs(ctx, p.Chrome.NormalizedUser(), dir); err != nil {
		testing.ContextLog(ctx, "Failed to save VM logs: ", err)
	}
}

func cleanupContainerHome(ctx context.Context, cont *vm.Container) {
	cleanupCtx, cancel := context.WithTimeout(ctx, containerCleanupTimeout)
	defer cancel()
	if err := cont.Cleanup(cleanupCtx, "."); err != nil {
		testing.ContextLog(ctx, "Failed to remove all files in home directory in the container: ", err)
	}
}

// trySaveContainerLogs fetches new (i.e. since last time the function
// successfully ran for this VM) logs from the container and writes them to
// crostini_journalctl.txt.
func trySaveContainerLogs(ctx context.Context, dir string, cont *vm.Container, post *PostTestData) {
	if cont == nil {
		testing.ContextLog(ctx, "No active container")
		return
	}
	cmdCtx, cancel := context.WithTimeout(ctx, containerLogsTimeout)
	defer cancel()

	args := []string{"sudo", "journalctl", "--no-pager", "--show-cursor"}
	if post != nil && post.journalCursor != "" {
		args = append(args, "--cursor", post.journalCursor)
	} else {
		// Limit initial queries (including setup failures) to the current boot
		// and a bounded line count so slow low-perf DUTs do not time out
		// streaming historical logs.
		args = append(args, "-b", "-n", "2000")
	}
	cmd := cont.Command(cmdCtx, args...)
	output, cmdErr := cmd.Output()
	if cmdErr != nil {
		testing.ContextLog(ctx, "Error running journalctl: ", cmdErr)
		// Preserve journalCursor on timeouts so partial output does not lose our
		// position, but clear it on non-timeout failures in case the cursor
		// became invalid.
		if cmdCtx.Err() == nil && post != nil {
			post.journalCursor = ""
		}
	}
	if len(output) == 0 {
		return
	}

	path := filepath.Join(dir, "crostini_journalctl.txt")
	if err := os.WriteFile(path, output, 0644); err != nil {
		testing.ContextLog(ctx, "Error writing journalctl to log: ", err)
		return
	}

	// Only advance the cursor when journalctl completed cleanly so partial
	// output on timeout cannot corrupt subsequent incremental fetches.
	if cmdErr != nil || post == nil {
		return
	}
	cursor := parseJournalCursor(output)
	if cursor == "" {
		// If journalctl exited cleanly without a cursor (e.g. "-- No entries --"
		// after a journal rollback or vacuum), reset journalCursor so later
		// calls fall back to a bounded initial capture instead of staying stuck.
		post.journalCursor = ""
		testing.ContextLog(ctx, "No journalctl cursor found")
		return
	}
	post.journalCursor = cursor
}

// parseJournalCursor extracts the cursor from the final "-- cursor: ..." line
// emitted by `journalctl --show-cursor`, or returns "" if absent.
func parseJournalCursor(output []byte) string {
	trimmed := bytes.TrimRight(output, "\r\n")
	lastNL := bytes.LastIndexByte(trimmed, '\n')
	after, ok := bytes.CutPrefix(trimmed[lastNL+1:], []byte("-- cursor: "))
	if !ok {
		return ""
	}
	return string(bytes.TrimSpace(after))
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
