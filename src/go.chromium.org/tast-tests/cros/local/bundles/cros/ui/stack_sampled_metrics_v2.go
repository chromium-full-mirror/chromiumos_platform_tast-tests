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

	spb "chromiumos/system_api/stack_sampled_metrics_status_proto"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/executioncontext"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

const (
	// stackSampledMetricsFileAsh and stackSampledMetricsFileLacros must match
	// kDefaultFilePath in Chrome's chromeos/tast_support/stack_sampling_recorder.cc
	stackSampledMetricsFileAsh    = "/tmp/stack-sampling-data"
	stackSampledMetricsFileLacros = "/tmp/stack-sampling-data-lacros"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StackSampledMetricsV2,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check that stack-sampled metrics work",
		Contacts: []string{
			"chromeos-data-eng@google.com",
			"iby@chromium.org",
		},
		BugComponent: "b:1087262",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "stack_sampled_metrics"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: fixture.ChromeLoggedInWithStackSampledMetrics,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacrosWithStackSampledMetrics",
			ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			Val:               browser.TypeLacros,
		}},
	})
}

func StackSampledMetricsV2(ctx context.Context, s *testing.State) {
	browserType := s.Param().(browser.Type)

	var fileName string
	if browserType == browser.TypeAsh {
		fileName = stackSampledMetricsFileAsh
	} else {
		fileName = stackSampledMetricsFileLacros
	}
	// Remove any stale files.
	os.Remove(fileName)

	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	// Set up the browser, open a window.
	const url = chrome.NewTabURL
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, url)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
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
		statusFile, err := os.Open(fileName)
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
