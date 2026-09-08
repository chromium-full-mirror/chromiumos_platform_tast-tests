// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast/core/testing"
)

const waitHistogramTimeout = 10 * time.Second

func init() {
	testing.AddTest(&testing.Test{
		Func: RustBindings,
		Desc: "Checks that the rust bindings reports histograms to Chrome",
		Contacts: []string{
			"chromeos-data-eng@google.com",
			"vovoy@chromium.org",
		},
		BugComponent: "b:1087262", // ChromeOS > Data > Engineering > Metrics
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Timeout:      chrome.MinLoginTimeout + time.Minute + 8*waitHistogramTimeout,
	})
}

func RustBindings(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx, chrome.ExtraArgs("--external-metrics-collection-interval=1"))
	if err != nil {
		s.Fatal("Chrome login: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	const (
		prefix                         = "MetricsLibraryTest"
		commandSendToUMA               = "SendToUMA"
		histogramSendToUMA             = prefix + "SendToUMA"
		sampleSendToUMA                = 1
		commandSendEnumToUMA           = "SendEnumToUMA"
		histogramSendEnumToUMA         = prefix + "SendEnumToUMA"
		sampleSendEnumToUMA            = 2
		commandSendRepeatedEnumToUMA   = "SendRepeatedEnumToUMA"
		histogramSendRepeatedEnumToUMA = prefix + "SendRepeatedEnumToUMA"
		sampleSendRepeatedEnumToUMA    = 3
		commandSendLinearToUMA         = "SendLinearToUMA"
		histogramSendLinearToUMA       = prefix + "SendLinearToUMA"
		sampleSendLinearToUMA          = 20
		commandSendPercentageToUMA     = "SendPercentageToUMA"
		histogramSendPercentageToUMA   = prefix + "SendPercentageToUMA"
		sampleSendPercentageToUMA      = 30
		commandSendSparseToUMA         = "SendSparseToUMA"
		histogramSendSparseToUMA       = prefix + "SendSparseToUMA"
		sampleSendSparseToUMA          = 120
		noRepeatCount                  = 1
		repeatCount                    = 5
	)

	// Report the histograms and check the results.
	executeRustBinding(ctx, s, commandSendToUMA, histogramSendToUMA, strconv.Itoa(sampleSendToUMA), "0" /*min*/, "20" /*max*/, "20" /*nbuckets*/)
	waitCheckHistogram(ctx, s, tconn, histogramSendToUMA, sampleSendToUMA, noRepeatCount)

	executeRustBinding(ctx, s, commandSendEnumToUMA, histogramSendEnumToUMA, strconv.Itoa(sampleSendEnumToUMA), "3" /*max*/)
	waitCheckHistogram(ctx, s, tconn, histogramSendEnumToUMA, sampleSendEnumToUMA, noRepeatCount)

	executeRustBinding(ctx, s, commandSendRepeatedEnumToUMA, histogramSendRepeatedEnumToUMA, strconv.Itoa(sampleSendRepeatedEnumToUMA), "5" /*max*/, strconv.Itoa(repeatCount) /*num_samples*/)
	waitCheckHistogram(ctx, s, tconn, histogramSendRepeatedEnumToUMA, sampleSendRepeatedEnumToUMA, repeatCount)

	executeRustBinding(ctx, s, commandSendLinearToUMA, histogramSendLinearToUMA, strconv.Itoa(sampleSendLinearToUMA), "100" /*max*/)
	waitCheckHistogram(ctx, s, tconn, histogramSendLinearToUMA, sampleSendLinearToUMA, noRepeatCount)

	executeRustBinding(ctx, s, commandSendPercentageToUMA, histogramSendPercentageToUMA, strconv.Itoa(sampleSendPercentageToUMA))
	waitCheckHistogram(ctx, s, tconn, histogramSendPercentageToUMA, sampleSendPercentageToUMA, noRepeatCount)

	executeRustBinding(ctx, s, commandSendSparseToUMA, histogramSendSparseToUMA, strconv.Itoa(sampleSendSparseToUMA))
	waitCheckHistogram(ctx, s, tconn, histogramSendSparseToUMA, sampleSendSparseToUMA, noRepeatCount)

	// SendCrosEventToUMA updates histogram Platform.CrOSEvent. There may be existing buckets in Platform.CrOSEvent.
	const (
		commandSendCrosEventToUMA = "SendCrosEventToUMA"
		histogramCrosEvent        = "Platform.CrOSEvent"
		vmDiskEraseFailed         = "Vm.DiskEraseFailed"
		indexVMDiskEraseFailed    = 23
	)
	hOld, err := metrics.GetHistogram(ctx, tconn, histogramCrosEvent)
	if err != nil {
		s.Fatal("Failed to get histogram: ", err)
	}
	executeRustBinding(ctx, s, commandSendCrosEventToUMA, vmDiskEraseFailed)
	h := waitHistogramUpdate(ctx, s, tconn, histogramCrosEvent, hOld)
	// waitHistogramUpdate() returns the diff to hOld. The histogram before calling SendCrosEventToUMA would be excluded.
	checkHistogram(s, h, indexVMDiskEraseFailed, noRepeatCount)

	// Ensure any leftover UMA events file is removed on test exit so that
	// unconsumed events do not leak into subsequent tests.
	defer os.Remove("/var/lib/metrics/uma-events")

	// These are not histograms. It's not necessary to verify the result on Chrome.
	// These binding functions always return error when not on the DUT. Check if they return OK on the DUT.
	// The rust program would abort and fail this test when a binding function returns error.
	// Note: SendCrashToUMA must pass a crash type recognized by Chrome's
	// ChromeOSMetricsProvider::LogCrash ("user", "kernel", or "uncleanshutdown")
	// to avoid hitting NOTREACHED() when ExternalMetrics collects uma-events.
	executeRustBinding(ctx, s, "SendUserActionToUMA", prefix+"SendUserActionToUMA")
	executeRustBinding(ctx, s, "SendCrashToUMA", "user")

	// Emit and wait for a final histogram sample to ensure Chrome's ExternalMetrics
	// collector consumes the preceding SendUserActionToUMA and SendCrashToUMA events
	// before the test exits.
	const histogramFlush = prefix + "Flush"
	executeRustBinding(ctx, s, commandSendPercentageToUMA, histogramFlush, strconv.Itoa(sampleSendPercentageToUMA))
	waitCheckHistogram(ctx, s, tconn, histogramFlush, sampleSendPercentageToUMA, noRepeatCount)
}

func executeRustBinding(ctx context.Context, s *testing.State, args ...string) {
	const executable = "/usr/local/libexec/tast/helpers/local/cros/metrics.RustBindings"

	cmd := testexec.CommandContext(ctx, executable, args...)
	err := cmd.Run(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run RustBindings: ", err)
	}
}

func waitHistogram(ctx context.Context, s *testing.State, tconn *chrome.TestConn, name string) *histogram.Histogram {
	s.Logf("Waiting for %v histogram", name)
	h, err := metrics.WaitForHistogram(ctx, tconn, name, waitHistogramTimeout)
	if err != nil {
		s.Fatalf("%s: Failed to get histogram: %v", name, err)
	}
	return h
}

func waitHistogramUpdate(ctx context.Context, s *testing.State, tconn *chrome.TestConn, name string, hOld *histogram.Histogram) *histogram.Histogram {
	s.Logf("Waiting for %v histogram update", name)
	h, err := metrics.WaitForHistogramUpdate(ctx, tconn, name, hOld, waitHistogramTimeout)
	if err != nil {
		s.Fatalf("%s: Failed to get histogram: %v", name, err)
	}
	return h
}

func checkHistogram(s *testing.State, h *histogram.Histogram, val, numSamples int64) {
	if len(h.Buckets) != 1 {
		s.Fatalf("Got %v buckets instead of 1", len(h.Buckets))
	}
	if h.Buckets[0].Min != int64(val) || h.Buckets[0].Max != int64(val)+1 {
		s.Errorf("Bucket has range [%v, %v) but sample was %v", h.Buckets[0].Min, h.Buckets[0].Max, val)
	}
	if h.Buckets[0].Count != numSamples {
		s.Errorf("Bucket has count %v instead of %d", h.Buckets[0].Count, numSamples)
	} else if tc := h.TotalCount(); tc != numSamples {
		s.Errorf("Histogram has total count %v instead of %d", tc, numSamples)
	}
}

func waitCheckHistogram(ctx context.Context, s *testing.State, tconn *chrome.TestConn, name string, val, numSamples int64) {
	h := waitHistogram(ctx, s, tconn, name)
	checkHistogram(s, h, val, numSamples)
}
