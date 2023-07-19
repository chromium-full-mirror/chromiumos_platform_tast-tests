// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Histograms,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the client computed crash severity and product group data are being reported to the appropriate UMA histogram",
		Contacts:     []string{"cros-telemetry@google.com"},
		BugComponent: "b:1032705",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational"},
	})
}

func Histograms(ctx context.Context, s *testing.State) {
	const (
		// The crash product group `platform` has an enum corresponding to 2.
		// Check out `CrashSeverityProductType` in tools/metrics/histograms/enums.xml.
		platformProductGroup = 2
		name                 = "ChromeOS.Stability.Error"
		timeout              = 10 * time.Second
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent()); err != nil {
		s.Fatal("Failed to set up crash test: ", err)
	}
	defer func(ctx context.Context) {
		if err := crash.TearDownCrashTest(ctx); err != nil {
			s.Error("Failed to tear down crash test: ", err)
		}
	}(cleanupCtx)

	inducePlatformCrash := func() {
		s.Log("Starting a target process")
		target := testexec.CommandContext(ctx, "/usr/bin/sleep", "300")
		if err := target.Start(); err != nil {
			s.Fatal("Failed to start a target process to kill: ", err)
		}
		defer func() {
			target.Kill()
			target.Wait()
		}()

		s.Log("Intentionally crashing the target process")
		if err := unix.Kill(target.Process.Pid, unix.SIGSEGV); err != nil {
			s.Fatal("Failed to induce an artifical crash: ", err)
		}

		s.Log("Waiting for reported meta file")
		expectedRegexes := []string{`coreutils\.\d{8}\.\d{6}\.\d+` + `\.` + strconv.Itoa(int(target.Process.Pid)) + `\` + `.meta`}

		crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
		if err != nil {
			s.Fatal("Couldn't get daemon store dirs: ", err)
		}

		files, err := crash.WaitForCrashFiles(ctx, crashDirs, expectedRegexes)
		if err != nil {
			s.Fatal("Couldn't find expected files: ", err)
		}

		if err := crash.RemoveAllFiles(ctx, files); err != nil {
			s.Log("Couldn't clean up files: ", err)
		}
	}

	// Configure Chrome to check for new external histograms every second.
	cr, err := chrome.New(ctx, chrome.ExtraArgs("--external-metrics-collection-interval=1"))
	if err != nil {
		s.Fatal("Chrome login: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	h1, err := metrics.GetHistogram(ctx, tconn, name)
	if err != nil {
		s.Fatal("Failed to get histogram: ", err)
	}
	s.Log("Got histogram: ", h1)

	inducePlatformCrash()

	s.Logf("Waiting for %v histogram update", name)
	h2, err := metrics.WaitForHistogramUpdate(ctx, tconn, name, h1, timeout)
	if err != nil {
		s.Fatal("Failed to get histogram update: ", err)
	}
	s.Log("Got histogram update: ", h2)

	foundExpectedSample := false

	// Finding the `HistogramBucket` corresponding to `platformProductGroup` and
	// checking whether only one sample was reported.
	for _, value := range h2.Buckets {
		if value.Min == platformProductGroup {
			if value.Count != 1 {
				s.Error("Expected 1 sample to be reported but got: ", value.Count)
			}
			foundExpectedSample = true
			break
		}
	}

	// As the histogram definitely contains at least one sample (which is
	// checked by `WaitForHistogramUpdate`), and we have checked above that the
	// sample was not in the `platformProductGroup` bucket, we know that the
	// sample reported belongs to the wrong histogram bucket.
	if !foundExpectedSample {
		s.Error("Sample not reported to the expected histogram bucket: ", h2.Buckets)
	}
}
