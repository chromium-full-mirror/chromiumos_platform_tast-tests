// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Rust,
		Desc: "Test the crash signature of rust binaries using the memfd panic handler",
		Contacts: []string{
			"chromeos-data-eng@google.com",
			"allenwebb@chromium.org",
		},
		BugComponent: "b:1032705",
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{
			{
				Val: crashRustParam{
					executable:    "/usr/local/libexec/tast/helpers/local/cros/crash.Rust.panic",
					crashFileName: "crash_Rust_panic",
					metaSig:       "sig=panicked at 'See you later, alligator!', crash.Rust.panic.rs:",
				},
			},
			{
				// cras conditionally compiles the panic hook through cbindgen.
				// This extra test verifies the integration.
				Name: "cras",
				Val: crashRustParam{
					executable:    "/usr/bin/cras",
					username:      "cras",
					extraEnv:      []string{"CRAS_RUST_PANIC_FOR_TESTING=1"},
					crashFileName: "cras",
					metaSig:       "sig=panicked at 'panicing due to CRAS_RUST_PANIC_FOR_TESTING'",
				},
			},
		},
	})
}

type crashRustParam struct {
	executable    string   // name of the executable to run.
	username      string   // User to run the command as. Empty to not change.
	extraEnv      []string // extra environment variables to set for the command.
	crashFileName string   // name for the crash file.
	metaSig       string   // The string that should be found in the meta file.
}

func Rust(ctx context.Context, s *testing.State) {
	param := s.Param().(crashRustParam)

	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent()); err != nil {
		s.Fatal("Failed to set up crash test: ", err)
	}
	defer func() {
		if err := crash.TearDownCrashTest(ctx); err != nil {
			s.Error("Failed to tear down crash test: ", err)
		}
	}()

	var cmd *testexec.Cmd
	if param.username == "" {
		cmd = testexec.CommandContext(ctx, param.executable)
	} else {
		var err error
		cmd, err = testexec.CommandContextUser(ctx, param.username, param.executable)
		if err != nil {
			s.Fatalf("Cannot run %s as %s", param.executable, param.username)
		}
	}
	cmd.Env = append(os.Environ(), param.extraEnv...)
	err := cmd.Run()
	if err == nil {
		s.Fatal("Expected crash, but command exited normally")
	} else if exitError, ok := err.(*exec.ExitError); ok {
		s.Logf("%v exit code: %v", param.executable, exitError.ProcessState.ExitCode())
	} else {
		s.Fatalf("Could not start %v: %v", param.executable, err)
	}
	pid := cmd.Cmd.Process.Pid

	pattern := fmt.Sprintf("%s.*.%d.*", param.crashFileName, pid)
	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	// We might not be logged in, so also allow system crash dir.
	crashDirs = append(crashDirs, crash.SystemCrashDir)
	files, err := crash.WaitForCrashFiles(ctx, crashDirs, []string{pattern})
	if err != nil {
		s.Fatal("Failed to wait for crash files: ", err)
	}

	// Check proclog for the expected environment variable and value.
	found := false
	for _, match := range files[pattern] {
		if strings.HasSuffix(match, ".meta") {
			contents, err := os.ReadFile(match)
			if err != nil {
				s.Errorf("Couldn't read meta file %s contents: %v", match, err)
				continue
			}
			found = true
			if !strings.Contains(string(contents), param.metaSig) {
				s.Error("Failed to find crash signature")
				if err := crash.MoveFilesToOut(ctx, s.OutDir(), match); err != nil {
					s.Error("Failed to save the meta file: ", err)
				}
			}
		}
	}
	if !found {
		s.Error("Failed to find meta file")
	}
	if err := crash.RemoveAllFiles(ctx, files); err != nil {
		s.Log("Couldn't clean up files: ", err)
	}
}
