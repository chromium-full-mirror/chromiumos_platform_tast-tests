// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SandboxLinuxUnittests,
		Desc: "Runs the sandbox_linux_unittests Chrome binary",
		Attr: []string{"group:mainline", "informational"},
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
	})
}

func SandboxLinuxUnittests(ctx context.Context, s *testing.State) {
	const exec = "sandbox_linux_unittests"

	// This test causes intentional crashes. Clean up after it.
	defer func() {
		crashes, err := crash.GetCrashes(crash.DefaultDirs()...)
		if err != nil {
			s.Error("Failed to get crash files: ", err)
			return
		}
		s.Log("Deleting (expected) crash file(s) for ", exec)
		for _, p := range crashes {
			if fn := filepath.Base(p); !strings.HasPrefix(fn, exec+".") {
				continue
			}
			if err := os.Remove(p); err != nil {
				s.Errorf("Failed to delete %v: %v", p, err)
			}
		}
	}()

	if report, err := gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(s.OutDir(), "gtest.log")),
		gtest.UID(int(sysutil.ChronosUID)),
	).Run(ctx); err != nil {
		s.Errorf("Failed to run %v: %v", exec, err)
		if report != nil {
			for _, name := range report.FailedTestNames() {
				s.Error(name, " failed")
			}
		}
	}
}
