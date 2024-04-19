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
		unload(ctx, d, name)
	}
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
	rpaths := resultPaths(ctx, d)
	if len(rpaths) == 0 {
		// The test run was not a kunit test. Nothing to do.
		testing.ContextLogf(ctx, "[%v] found to not be a kunit test", name)
		return nil, nil
	}
	testing.ContextLogf(ctx, "Results dir for [%v]: [%v]", name, rpaths)
	for _, rpath := range rpaths {
		output, err := d.Conn().CommandContext(ctx, "cat", rpath).Output()
		if err != nil {
			return nil, errors.Wrapf(err, "cat [%v]", rpath)
		}
		testing.ContextLog(ctx, string(output))
	}
	// TODO(b/332535556): Not implemented.
	// The test results from kunit look as follows:
	//    # Subtest: drm_buddy
	//    1..6
	// # drm_buddy: pass:6 fail:0 skip:0 total:6
	// # Totals: pass:6 fail:0 skip:0 total:6
	//    ok 1 - igt_buddy_alloc_limit
	//    ok 2 - igt_buddy_alloc_range
	//    ok 3 - igt_buddy_alloc_optimistic
	//    ok 4 - igt_buddy_alloc_pessimistic
	//    ok 5 - igt_buddy_alloc_smoke
	//    ok 6 - igt_buddy_alloc_pathological
	// ok 1 - drm_buddy
	return nil, nil
}

// resultPaths returns paths to the results files corresponding to the latest run kunit test.
// Results directories are created under /sys/kernel/debug/kunit/<test_name>. The |test_name| does not follow
// a consistent naming pattern. Hence, we look at the directories under /sys/kernel/debug/kunit and determine
// |test_name| at runtime.
func resultPaths(ctx context.Context, d *dut.DUT) []string {
	dir, err := d.Conn().CommandContext(ctx, "ls", "/sys/kernel/debug/kunit").Output()
	if err != nil {
		// Test was not kunit, ignore.
		testing.ContextLog(ctx, "Unable to list /sys/kernel/debug/kunit")
		return nil
	}
	tdirs := strings.Fields(strings.Trim(string(dir), "\n"))
	if len(tdirs) == 0 {
		return nil
	}
	var paths []string
	for _, tdir := range tdirs {
		paths = append(paths, filepath.Join("/sys/kernel/debug/kunit", tdir, "results"))
	}
	return paths
}

func unload(ctx context.Context, d *dut.DUT, name string) error {
	testing.ContextLog(ctx, "Unload: ", name)
	if err := d.Conn().CommandContext(ctx, "modprobe", "-r", name).Run(); err != nil {
		return errors.Wrapf(err, "modprobe -r [%v]", name)
	}
	return nil
}
