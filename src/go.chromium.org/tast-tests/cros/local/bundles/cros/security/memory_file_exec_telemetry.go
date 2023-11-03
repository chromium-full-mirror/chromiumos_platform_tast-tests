// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// Name of the histogram that records security anomalies.
	securityAnomalyHistogramName = "ChromeOS.SecurityAnomaly"
	// The bucket in |securityAnomalyHistogramName| that tracks successful
	// memfd_create syscalls (baseline condition of memfd execution).
	memfdCreateHistogramBucket = 3
	// The bucket in |securityAnomalyHistogramName| that tracks memory file
	// execution attempts.
	memfdExecuteHistogramBucket = 4
	// The amount of time reserved for cleanup.
	cleanupTimeout = 10 * time.Second
	// The amount of time reserved for UMA metrics to emit.
	umaTimeout = 90 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryFileExecTelemetry,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the blockage of memfd execution attempts and ensures their detection and reporting through secanomalyd",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"chrome", "metrics_consent", "memfd_exec_detection"},
		Attr:         []string{"group:mainline"},
		Fixture:      "chromeLoggedIn",
		Timeout:      umaTimeout + cleanupTimeout,
	})
}

func MemoryFileExecTelemetry(ctx context.Context, s *testing.State) {
	const (
		// These binaries are installed by the
		// chromeos-base/tast-local-helpers-cros package.
		// This program attempts to perform a fexecve syscall on a memfd
		// populated with the binary supplied to it.
		memfdBinPath = "/usr/local/libexec/tast/helpers/local/cros/security.MemoryFileExecTelemetry.memfd_fexecve"
		// This is supplied to `memfdBinPath` as an argument. It simply
		// returns an error if successfully executed.
		testBinPath = "/usr/local/libexec/tast/helpers/local/cros/security.MemoryFileExecTelemetry.test_bin"
	)

	// |cleanupCtx| is used for setting a timeout that includes the cleanup
	// time, and ctx is updated to exclude the cleanup time.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeout)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Make sure secanomalyd isn't running, and then clear pending metrics.
	// This ensures there's no events from before the test starts.
	if err := upstart.StopJob(ctx, "secanomalyd"); err != nil {
		s.Error("Upstart could not stop secanomalyd: ", err)
	}
	// See ClearHistogramTransferFile for more about why this is needed.
	if err := metrics.ClearHistogramTransferFile(); err != nil {
		s.Error("Could not truncate existing metrics files: ", err)
	}

	// Start secanomalyd with the --dev flag, so that metrics are emitted
	// despite being in dev mode.
	if err := upstart.StartJob(ctx, "secanomalyd", upstart.WithArg("DEV", "true")); err != nil {
		s.Fatal("Upstart could not start secanomalyd: ", err)
	}
	// Restart secanomalyd to clear --dev flag at the end of execution.
	defer func(ctx context.Context) {
		if err := upstart.RestartJob(ctx, "secanomalyd"); err != nil {
			s.Error("Could not restart secanomalyd after the test: ", err)
		}
	}(cleanupCtx)

	// Establish a connection to the testing API for reading histogram values.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Samples the state of target histogram before the memfd execution attempt.
	// tconn is passed in to take traces and metrics from Ash.
	oldHistogram, err := metrics.GetHistogram(ctx, tconn, securityAnomalyHistogramName)
	if err != nil {
		s.Fatal("Could not get initial value of histogram: ", err)
	}

	// Verifies the blockage of memfd execution in ChromeOS and acts as a
	// trigger for the detection and reporting mechanism.
	cmd := testexec.CommandContext(ctx, memfdBinPath, testBinPath)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("%q failed: %v", shutil.EscapeSlice(cmd.Args), err)
	}
	// The blockage will generate a crash report. Defer cleanup of the crash.
	defer func() {
		matches, _ := filepath.Glob("/var/spool/crash/security_anomaly.*")

		for _, match := range matches {
			s.Logf("Removing %q", match)
			os.Remove(match)
		}
	}()

	// We can't use metrics.WaitForHistogramUpdate because other buckets in
	// ChromeOS.SecurityAnomaly can be updated by other anomalous events in
	// the system and we don't want to stop waiting because of those events.
	testing.ContextLog(ctx, "Waiting for histogram to update")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		newHistogram, err := metrics.GetHistogram(ctx, tconn, securityAnomalyHistogramName)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get new value of histogram"))
		}
		diff, err := newHistogram.Diff(oldHistogram)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "histogram diff error"))
		}
		foundMemfdCreate := false
		foundMemfdExecute := false
		for _, bucket := range diff.Buckets {
			switch bucket.Min {
			case memfdCreateHistogramBucket:
				foundMemfdCreate = true
			case memfdExecuteHistogramBucket:
				foundMemfdExecute = true
			}
			if foundMemfdCreate && foundMemfdExecute {
				break
			}
		}
		if !foundMemfdCreate || !foundMemfdExecute {
			return errors.New("did not find memfd execution or creation metrics " + diff.String())
		}
		return nil
	}, &testing.PollOptions{Timeout: umaTimeout}); err != nil {
		s.Error("Failed when looking for expected histogram diffs: ", err)
	}
}
