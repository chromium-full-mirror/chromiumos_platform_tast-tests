// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	spb "go.chromium.org/chromiumos/system_api/stack_sampled_metrics_status_proto"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/executioncontext"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

const (
	// stackSampledMetricsFileAsh must match kDefaultFilePath in Chrome's
	// chromeos/tast_support/stack_sampling_recorder.cc
	stackSampledMetricsFileAsh = "/tmp/stack-sampling-data"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StackSampledMetricsV2,
		Desc: "Check that stack-sampled metrics work",
		Contacts: []string{
			"chromeos-data-eng@google.com",
			"troywang@google.com",
		},
		BugComponent: "b:1087262",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "stack_sampled_metrics"},
		Timeout:      2 * time.Minute,
		Fixture:      fixture.ChromeLoggedInWithStackSampledMetrics,
	})
}

func StackSampledMetricsV2(ctx context.Context, s *testing.State) {
	// Remove any stale files.
	os.Remove(stackSampledMetricsFileAsh)

	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	// Set up the browser, open a window.
	const url = chrome.NewTabURL
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer conn.Close()

	type processThread struct {
		processType int32
		threadType  int32
	}
	// We always expect to see at least the following process + threads being profiled.
	// List should generally match chrome/common/profiler/thread_profiler_browsertest.cc
	expectedResults := []processThread{
		{executioncontext.BrowserProcess, executioncontext.MainThread},
		{executioncontext.BrowserProcess, executioncontext.IOThread},
		{executioncontext.RendererProcess, executioncontext.MainThread},
		{executioncontext.RendererProcess, executioncontext.IOThread},
		{executioncontext.RendererProcess, executioncontext.CompositorThread},
		{executioncontext.GPUProcess, executioncontext.MainThread},
		{executioncontext.GPUProcess, executioncontext.IOThread},
		{executioncontext.GPUProcess, executioncontext.CompositorThread},
		{executioncontext.NetworkServiceProcess, executioncontext.IOThread},
	}
	testing.ContextLog(ctx, "Waiting for all processes + threads to be profiled")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		statusFile, err := os.Open(stackSampledMetricsFileAsh)
		if err != nil {
			return errors.Wrap(err, "failed to open status file")
		}
		defer statusFile.Close()

		if err := unix.Flock(int(statusFile.Fd()), unix.LOCK_EX); err != nil {
			return errors.Wrap(err, "failed to lock status file")
		}

		statusBytes, err := io.ReadAll(statusFile)
		if err != nil {
			return errors.Wrap(err, "failed to read status file")
		}
		status := &spb.StackSampledMetricsStatus{}
		if err := proto.Unmarshal(statusBytes, status); err != nil {
			return errors.Wrap(err, "failed to interpret status data")
		}

		var missedExpections []processThread
		for _, expectation := range expectedResults {
			found := false
		ProcessLoop:
			for processType, threadCountMap := range status.GetProcessTypeToThreadCountMap() {
				for threadType, count := range threadCountMap.GetThreadTypeToSuccessCount() {
					if expectation.processType == processType && expectation.threadType == threadType && count > 0 {
						found = true
						break ProcessLoop
					}
				}
			}

			if !found {
				missedExpections = append(missedExpections, expectation)
			}
		}

		if len(missedExpections) > 0 {
			var missedExpectionsStr []string
			for _, missedExpection := range missedExpections {
				missedExpectionsStr = append(missedExpectionsStr, fmt.Sprintf("%+v", missedExpection))
			}
			return errors.New("not all process + threads profiled: " + strings.Join(missedExpectionsStr, ", "))
		}

		return nil
	}, nil); err != nil {
		s.Error("Chrome did not profile expected process+threads: ", err)
	}

}
