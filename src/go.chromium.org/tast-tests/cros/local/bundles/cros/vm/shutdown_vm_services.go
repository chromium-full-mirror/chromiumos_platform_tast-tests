// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShutdownVMServices,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that vm service processes don't crash during process shutdown",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "chibar@google.com"},
		BugComponent: "b:1248538",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"vm_host", "chrome", "android_vm"},
		// Timeout for reliable ARCVM boot + 15 secs for actual test execution
		Timeout: chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout + 15*time.Second,
	})
}

func ShutdownVMServices(ctx context.Context, s *testing.State) {
	crashDirs := []string{crash.SystemCrashDir}
	regexes := []string{`vm_concierge\.\d{8}\.\d{6}\.\d+\.\d+\.dmp`, `crosvm\.\d{8}\.\d{6}\.\d+\.\d+\.dmp`}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent(), crash.DevImage()); err != nil {
		s.Fatal("Failed to set up crash test: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
	if err != nil {
		s.Fatal("Failed to login: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Start ARCVM to make sure vm service processes are running
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to run arc: ", err)
	}
	defer a.Close(cleanupCtx)

	// Restart ui to shutdown vm service processes
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart ui: ", err)
	}

	var notFoundError crash.RegexesNotFound
	// Check if dump files are created
	files, err := crash.WaitForCrashFiles(ctx, crashDirs, regexes, crash.Timeout(5*time.Second))
	if err == nil {
		// All of the regexes are matched
		for _, dumps := range files {
			s.Error("Detected a process crash: ", dumps)
		}
	} else if errors.As(err, &notFoundError) {
		// Some of the regexes may be matched
		err, _ := err.(crash.RegexesNotFound)
		if len(err.PartialMatches) > 0 {
			for _, dumps := range err.PartialMatches {
				s.Error("Detected a process crash: ", dumps)
			}
		}
	} else {
		s.Fatal("Failed to wait for crash files: ", err)
	}
}
