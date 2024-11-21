// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/guestconn"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/storage"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const runTraverseDirs string = "run-traverse-dirs.py"

type deviceType int

const (
	pmemExt2 deviceType = iota
	virtioFs
)

type testParam struct {
	device deviceType
	dax    bool
}

// String returns a string representation of testParam to pass into the test script.
func (tp testParam) String() string {
	switch tp.device {
	case pmemExt2:
		if tp.dax {
			return "pmem-ext2-dax"
		}
		return "pmem-ext2"
	case virtioFs:
		return "virtiofs"
	}
	return "unknown"
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TraverseDirs,
		Desc:         "Shares a directory with the guest as read-only to check directory traversal",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "keiichiw@google.com"},
		BugComponent: "b:1248538",
		// TODO(b/370873134): Reduce execution frequency to "crosbolt_weekly" once we confirm test's stability.
		Attr:         []string{"group:crosbolt", "crosbolt_nightly", "group:sw_gates_virt", "sw_gates_virt_enabled"},
		SoftwareDeps: []string{"vm_host", "chrome"},
		Data:         []string{runTraverseDirs, guestconn.LibFile},
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{
			{
				Name: "pmem_ext2",
				Val: testParam{
					device: pmemExt2,
				},
			},
			{
				Name: "pmem_ext2_dax",
				Val: testParam{
					device: pmemExt2,
					dax:    true,
				},
			},
			{
				Name: "virtiofs",
				Val: testParam{
					device: virtioFs,
				},
			},
		},
	})
}

func countFiles(dir string) (int, error) {
	count := 0
	err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if path != dir {
			count++
		}
		return nil
	})
	if err != nil {
		return 0, errors.Wrap(err, "failed to count files")
	}
	return count, nil
}

// TraverseDirs checks directory traversal performance.
func TraverseDirs(ctx context.Context, s *testing.State) {
	// Reserve 5 seconds for clean up
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Create a temporary directory that shared with the guest so the guest can put test logs.
	td, err := os.MkdirTemp("/usr/local/tmp", "tast.vm.TraverseDirs.")
	if err != nil {
		s.Fatal("Failed to create temporary directory: ", err)
	}
	defer os.RemoveAll(td)

	kernelPath := "/opt/google/vms/android/vmlinux"

	// Directory path shared with the guest
	sharedDir := "/usr/lib64"

	tp := s.Param().(testParam)

	var storageOpt vm.Option
	var scriptArgs []string
	switch tp.device {
	case pmemExt2:
		storageOpt = vm.PmemExt2(vm.PmemExt2Param{
			Path:           sharedDir,
			BlocksPerGroup: 32768,
			InodesPerGroup: 2048,
			// page size * blocks_per_group * number of block groups
			Size: 4096 * 32768 * 10,
		})
		scriptArgs = []string{
			"--kind",
			tp.String(),
			"--mount-src",
			"/dev/pmem0",
			"--working-dir",
			td,
		}
	case virtioFs:
		tag := "shared"
		// Use the similar configuration with crostini's font sharing.
		storageOpt = vm.SharedDir(vm.SharedDirParam{
			Src:       sharedDir,
			Tag:       tag,
			FsType:    "fs",
			Cache:     "always",
			Timeout:   uint(600),
			Writeback: true,
		})
		scriptArgs = []string{
			"--kind",
			tp.String(),
			"--mount-src",
			tag,
			"--working-dir",
			td,
		}
	default:
		s.Fatal("Unexpected test name: ", tp.String())
	}
	params, err := storage.GenCrosvmCmdFromStorageOpt(
		td, s.OutDir(), kernelPath,
		s.DataPath(runTraverseDirs),
		scriptArgs,
		storageOpt,
	)
	if err != nil {
		s.Fatal("Failed to construct crosvm command: ", err)
	}

	// Use FIFO files as the guest's serial device.
	toGuestFIFO, fromGuestFIFO, err := guestconn.CreateGuestConn(td, params)
	if err != nil {
		s.Fatal("Failed to create guest connection: ", err)
	}

	// Increase the max open file limit as the benchmark creates a lot of files.
	args := append([]string{"--nofile=262144", "crosvm"}, params.ToArgs()...)

	cmd := testexec.CommandContext(ctx, "prlimit", args...)

	// Construct s a crosvm command
	output, err := os.Create(filepath.Join(s.OutDir(), "crosvm.log"))
	if err != nil {
		s.Fatal("Failed to create crosvm log file: ", err)
	}
	defer output.Close()

	cmd.Stdout = output
	cmd.Stderr = output

	// Before starting crosvm, drop host caches
	if err := disk.DropCaches(ctx); err != nil {
		s.Fatal("Failed to drop caches: ", err)
	}

	start := time.Now()

	s.Log("Start crosvm command: ", cmd)

	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to wait crosvm: ", err)
	}

	toGuest, fromGuest, cleanUp, err := guestconn.OpenGuestConn(ctx, toGuestFIFO, fromGuestFIFO)
	defer cleanUp(cleanupCtx)
	if err != nil {
		s.Fatal("Failed to open guest connection: ", err)
	}
	reader := bufio.NewReaderSize(fromGuest, 4096)

	_, err = guestconn.WaitForPrefix(reader, []string{guestconn.PrefixReady})
	if err != nil {
		s.Fatal("Failed to wait for 'READY': ", err)
	}

	bootTime := time.Since(start)
	s.Log("Boot time: ", bootTime)
	traverseStart := time.Now()

	if _, err := toGuest.WriteString(guestconn.Run); err != nil {
		s.Fatal("Failed to signal 'Run' to the guest")
	}

	// Receive "MESSAGE:{number of files}" from the guest
	line, err := guestconn.WaitForPrefix(reader, []string{guestconn.PrefixMessage})
	if err != nil {
		s.Fatal("Failed to wait for 'MESSAGE': ", err)
	}

	traverseTime := time.Since(traverseStart)
	s.Log("Traverse time: ", traverseTime)

	numStr := strings.TrimSuffix(string(line[len(guestconn.PrefixMessage):]), "\r\n")
	numFiles, err := strconv.Atoi(numStr)
	if err != nil {
		s.Fatalf("Failed to parse number of files reported by the guest %q: %v", line, err)
	}

	if err := cmd.Wait(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to wait crosvm: ", err)
	}

	realNumFiles, err := countFiles(sharedDir)
	if err != nil {
		s.Fatal("Failed to count files: ", err)
	}

	if realNumFiles != numFiles {
		s.Errorf("Number of files does not match: %d != %d", realNumFiles, numFiles)
	}

	perfValues := perf.NewValues()
	perfValues.Set(perf.Metric{
		Name:      "boot",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, float64(bootTime.Milliseconds()))
	perfValues.Set(perf.Metric{
		Name:      "traverse",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, float64(traverseTime.Milliseconds()))
	perfValues.Save(s.OutDir())

}
