// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package kunit is a wrapper that runs kunit tests on the target DUT or VM.
package kunit

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Wrapper,
		Desc: "Wrapper test that runs kunit tests",
		Contacts: []string{
			"chromeos-kernel@google.com",
			"shraash@google.com",
			"tzungbi@chromium.org",
			"zsm@chromium.org",
		},
		// ChromeOS > Platform > System > Kernel > Syzkaller > Syzkaller-Dev > DKT
		BugComponent: "b:1174001",
		Timeout:      10 * time.Minute,
		// TODO(b/332535556): A new group for kunit tests will need to be created. "group:syzkaller" is
		// meant to be temporary.
		Attr: []string{"group:syzkaller"},
	})
}

// Wrapper runs kunit tests against DUTs or VMs with kernels built with "USE=kunit". At the
// moment this USE flag is only enabled on devices with debug kernels. This might
// change in the future.
func Wrapper(ctx context.Context, s *testing.State) {
	d := s.DUT()
	board, err := findBoard(ctx, d)
	if err != nil {
		s.Fatal("Unable to find board: ", err)
	}
	s.Log("Board found to be: ", board)

	modules, err := kunitModules(ctx, d)
	if err != nil {
		s.Fatal("Unable to list kunit modules: ", err)
	}
	s.Logf("Found [%v] tests: %v", len(modules), modules)

	for _, module := range modules {
		name := kunitName(module)
		if err := runKunit(ctx, d, name); err != nil {
			s.Fatal("Failed to run kunit: ", err)
		}
		failed, err := kunitResult(ctx, d, name)
		if err != nil {
			s.Fatal("Failed to retrieve kunit results: ", err)
		}
		if len(failed) != 0 {
			s.Logf("Test [%v] with subtests [%v] failed on [%v]", name, failed, board)
		}
		if err := unload(ctx, d, name); err != nil {
			s.Fatal("Failed to unload module: ", err)
		}
	}
}

func findBoard(ctx context.Context, d *dut.DUT) (string, error) {
	board, err := reporters.New(d).Board(ctx)
	if err != nil {
		return "", errors.Wrap(err, "unable to find board")
	}
	return board, nil
}

func kunitModules(ctx context.Context, d *dut.DUT) ([]string, error) {
	kr, err := d.Conn().CommandContext(ctx, "uname", "-r").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to find uname")
	}
	mpath := filepath.Join("/lib/modules", strings.TrimRight(string(kr), "\n"))
	for _, suffix := range []string{"*.ko.gz", "*.ko"} {
		output, err := d.Conn().CommandContext(ctx, "find", mpath, "-name", suffix).Output()
		if err != nil {
			return nil, errors.Wrapf(err, "find [%v] failed with [%v]", mpath, suffix)
		}
		if len(output) == 0 {
			continue
		}
		lines := strings.Split(string(output), "\n")
		var modules []string
		for _, line := range lines {
			// TODO(b/332535556): Filter out non-kunit tests.
			if strings.Contains(line, "test") {
				modules = append(modules, line)
			}
		}
		if len(modules) != 0 {
			return modules, nil
		}
	}
	return nil, errors.Errorf("unable to locate modules at [%v]", mpath)
}

func kunitName(module string) string {
	for _, suffix := range []string{".ko", ".ko.gz"} {
		module = strings.TrimSuffix(module, suffix)
	}
	return filepath.Base(module)
}

func runKunit(ctx context.Context, d *dut.DUT, name string) error {
	testing.ContextLog(ctx, "Modprobe: ", name)
	if err := d.Conn().CommandContext(ctx, "modprobe", name).Run(); err != nil {
		return errors.Wrapf(err, "modprobe [%v]", name)
	}
	return nil
}

func kunitResult(ctx context.Context, d *dut.DUT, name string) ([]string, error) {
	// TODO(b/332535556): Not implemented.
	return nil, nil
}

func unload(ctx context.Context, d *dut.DUT, name string) error {
	testing.ContextLog(ctx, "Unload: ", name)
	if err := d.Conn().CommandContext(ctx, "modprobe", "-r", name).Run(); err != nil {
		return errors.Wrapf(err, "modprobe -r [%v]", name)
	}
	return nil
}
