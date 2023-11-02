// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/dlc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/storage"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const runManyFiles string = "run-manyfiles.py"

var enableTraceCmdVar = testing.RegisterVarString(
	"vm.ManyFiles.enableTraceCmd",
	"false",
	"Run 'trace-cmd record' for each test case on the host.",
)

type manyFilesParams struct {
	kind     string
	cache    string
	caseFold bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManyFiles,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure performances of touching many files",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "keiichiw@google.com"},
		BugComponent: "b:1248538",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Data:         []string{runManyFiles},
		SoftwareDeps: []string{"vm_host", "chrome", "dlc"},
		Vars: []string{
			// Specify guest kernel (if not provided use termina dlc)
			"vm.ManyFiles.kernelPath",
			// Run 'trace-cmd record' for each test case on the host.
			"vm.ManyFiles.enableTraceCmd",
		},
		Timeout: 20 * time.Minute,
		Fixture: "vmDLC",
		Params: []testing.Param{
			{
				// TODO(b/275507715): Add variant with casefold enabled.
				Name: "block",
				Val: manyFilesParams{
					kind: "block",
				},
			},
			{
				Name: "virtiofs",
				Val: manyFilesParams{
					kind:  "virtiofs",
					cache: "auto",
				},
			},
			{
				Name: "virtiofs_casefold",
				Val: manyFilesParams{
					kind:     "virtiofs",
					cache:    "auto",
					caseFold: true,
				},
			},
			{
				Name: "virtiofs_cached",
				Val: manyFilesParams{
					kind:  "virtiofs",
					cache: "always",
				},
			},
			{
				Name: "virtiofs_cached_casefold",
				Val: manyFilesParams{
					kind:     "virtiofs",
					cache:    "always",
					caseFold: true,
				},
			},
		},
	})
}

// waitForPrefix reads line from reader until the line starts with one of
// items in expectedPrefixes.
func waitForPrefix(reader *bufio.Reader, prefixes []string) (string, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", errors.Wrap(err, "failed to read line")
		}

		for _, pre := range prefixes {
			if strings.HasPrefix(line, pre) {
				return line, nil
			}
		}
	}
}

func runOneTestCase(ctx context.Context, toGuest *os.File, reader *bufio.Reader, testCase, outDir string, enableTraceCmd bool) error {
	var collectFunc func(ctx context.Context) error
	if enableTraceCmd {
		outPath := filepath.Join(outDir, fmt.Sprintf("%s-trace.dat", strings.TrimSuffix(testCase, "()")))
		cleanupFunc := func() {}
		var err error

		cleanupFunc, collectFunc, err = tracing.RunTraceCmd(ctx, outPath)
		if err != nil {
			return errors.Wrap(err, "failed to start trace-cmd")
		}
		defer cleanupFunc()
	}

	testing.ContextLog(ctx, "Start test case: ", testCase)
	if _, err := toGuest.WriteString(fmt.Sprintf("RUN\n")); err != nil {
		return errors.Wrap(err, "failed to write a message to toGuestFIFO")
	}
	if _, err := waitForPrefix(reader, []string{"END"}); err != nil {
		return errors.Wrap(err, "failed to wait for END")
	}
	testing.ContextLog(ctx, "Finished test case: ", testCase)

	// Stop trace-cmd
	if collectFunc != nil {
		if err := collectFunc(ctx); err != nil {
			return errors.Wrap(err, "failed to stop trace-cmd")
		}
	}

	return nil
}

func ManyFiles(ctx context.Context, s *testing.State) {
	data := s.FixtValue().(dlc.FixtData)
	kernelPath := data.Kernel
	if kernelPathOverride, ok := s.Var("vm.ManyFiles.kernelPath"); ok {
		kernelPath = kernelPathOverride
	}

	// Create a temporary directory that shared with the guest so the guest can put test logs.
	td, err := ioutil.TempDir("/usr/local/tmp", "tast.vm.ManyFiles.")
	if err != nil {
		s.Fatal("Failed to create temporary directory: ", err)
	}
	defer os.RemoveAll(td)

	// Create a temporary directory on the encrypted file system on `/home/root/${user hash}/`.
	// This directory will be accessed by FIO.
	username := data.Chrome.NormalizedUser()
	rootCryptDir, err := cryptohome.SystemPath(ctx, username)
	if err != nil {
		s.Fatal("Failed to get the cryptohome directory: ", err)
	}
	ud, err := ioutil.TempDir(rootCryptDir, "tast.vm.ManyFiles.")
	defer os.RemoveAll(ud)

	p := s.Param().(manyFilesParams)
	opt, err := storage.NewOption(p.kind, p.cache, p.caseFold)
	if err != nil {
		s.Fatal("Failed to create storage option: ", err)
	}

	outputJSON := filepath.Join(s.OutDir(), "test-result.json")
	scriptArgs := []string{
		"--kind",
		opt.Kind,
		"--mount-src",
		opt.Tag,
		"--working-dir",
		td,
		"--output-json",
		outputJSON,
	}

	// Constructs a crosvm command
	ps, err := storage.GenCrosvmCmd(td, ud, s.OutDir(), kernelPath,
		s.DataPath(runManyFiles),
		opt, scriptArgs)
	if err != nil {
		s.Fatal("Failed to construct crosvm command: ", err)
	}

	// Use FIFO files as the guest's serial device.
	toGuestFIFO := filepath.Join(td, "input.fifo")
	if err := unix.Mkfifo(toGuestFIFO, 0666); err != nil {
		s.Fatal("Failed to make input fifo: ", err)
	}
	fromGuestFIFO := filepath.Join(td, "output.fifo")
	if err := unix.Mkfifo(fromGuestFIFO, 0666); err != nil {
		s.Fatal("Failed to make outputput fifo: ", err)
	}
	vm.SerialIO(toGuestFIFO, fromGuestFIFO)(ps)

	// Increase the max open file limit as the benchmark creates a lot of files.
	args := append([]string{"--nofile=262144", "crosvm"}, ps.ToArgs()...)

	cmd := testexec.CommandContext(ctx, "prlimit", args...)

	// Construct s a crosvm command
	output, err := os.Create(filepath.Join(s.OutDir(), "crosvm.log"))
	if err != nil {
		s.Fatal("Failed to create crosvm log file: ", err)
	}
	defer output.Close()

	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to run crosvm: ", err)
	}

	toGuest, err := os.OpenFile(toGuestFIFO, os.O_WRONLY, 0755)
	if err != nil {
		s.Fatal("Failed to open guest input")
	}
	defer toGuest.Close()

	fromGuest, err := os.Open(fromGuestFIFO)
	if err != nil {
		s.Fatal("Failed to open guest output")
	}
	defer fromGuest.Close()
	reader := bufio.NewReaderSize(fromGuest, 4096)

	enableTraceCmd, err := strconv.ParseBool(enableTraceCmdVar.Value())
	if err != nil {
		s.Fatalf("Failed to parse enableTraceCmdVar %v: %v", enableTraceCmdVar.Value(), err)
	}

	for {
		// Waiting for the guest sending "READY" or "COMPLETE".
		line, err := waitForPrefix(reader, []string{"READY:", "COMPLETE"})
		if err != nil {
			s.Fatal("Failed to wait for 'READY:' or 'COMPLETE': ", err)
		}

		// "COMPLETE" means that the all test cases completed
		if strings.HasPrefix(line, "COMPLETE") {
			s.Log("All the guest test cases are completed")
			break
		}
		// line must start with "READY:"
		trimed := strings.TrimRight(line, " \r\n")
		testCase := strings.TrimPrefix(trimed, "READY:")
		s.Logf("Guest is ready for test case %q", testCase)

		// Drop host caches before starting crosvm
		if err := disk.DropCaches(ctx); err != nil {
			s.Fatal("Failed to drop caches: ", err)
		}

		// GoBigSleepLint: Sleep until virtiofs's cache is invalidated
		if err != testing.Sleep(ctx, storage.VirtioFSCacheTimeoutSecond*time.Second) {
			s.Fatal("Failed to sleep until cache is invalidated: ", err)
		}

		if err := runOneTestCase(ctx, toGuest, reader, testCase, s.OutDir(), enableTraceCmd); err != nil {
			s.Errorf("Failed test on %s : %v", testCase, err)
		}
	}

	if err := cmd.Wait(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to wait crosvm: ", err)
	}

	jsonData, err := ioutil.ReadFile(outputJSON)
	if err != nil {
		s.Fatal("Failed to read fio results: ", err)
	}

	var result map[string]float64
	json.Unmarshal(jsonData, &result)
	perfValues := perf.NewValues()

	for name, ms := range result {
		perfValues.Set(perf.Metric{
			Name:      name,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, ms)
	}
	perfValues.Save(s.OutDir())
}
