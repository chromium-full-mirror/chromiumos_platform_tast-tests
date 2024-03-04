// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/tracing"
	powerpb "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	// traceCmdEventsVarName is the name of the variable to specify events for trace-cmd to collect.
	traceCmdEventsVarName = "power.SuspendPerf.traceCmdEvents"
)

var traceCmdEventsVar = testing.RegisterVarString(
	traceCmdEventsVarName,
	"",
	"Comma-separated events to enable trace-cmd and to ask it to record. (e.g. 'syscalls,sched:*')",
)

type testArgsForSuspendPerf struct {
	numSuspend int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that performance of suspend/resume",
		Contacts: []string{
			"cros-suspend-resume@google.com",
			"mhiramat@google.com",
		},
		BugComponent: "b:256693104",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.power.SuspendPerfService", "tast.cros.tracing.TraceCmdService"},
		// (40 sec for histograms + 10 + 60 sec suspend/resume) * 5 times
		Timeout: 10 * time.Minute,
		Params: []testing.Param{{
			Name: "",
			Val: testArgsForSuspendPerf{
				numSuspend: 5,
			},
		}},
	})
}

const (
	defaultSuspendSeconds       = 10
	defaultRedialTimeoutSeconds = 60

	defaultInstanceName = "suspend_perf"
	defaultBufferSize   = 10240
)

// TODO make a new struct type with name and direction.
var defaultMetrics = []string{"Power.KernelSuspendTimeOnAC", "Power.KernelResumeTimeOnAC", "Power.DisplayAfterResumeDurationMsOnAC"}

// Delay and timeout for waitHistogramsUpdate().
var defaultWaitInterval = time.Duration(2) * time.Second
var defaultWaitTimeout = time.Duration(40) * time.Second

var remoteCommandTimeout = time.Duration(3) * time.Second

func SuspendPerf(ctx context.Context, s *testing.State) {
	args := s.Param().(testArgsForSuspendPerf)

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	if err := initTracing(ctx, cl); err != nil {
		s.Log("Failed to initialize tracing, but this is ignorable: ", err)
	}
	defer cleanupTracing(ctx, s, cl)

	// Login and setup
	service := powerpb.NewSuspendPerfServiceClient(cl.Conn)
	if _, err := service.Prepare(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to login on DUT: ", err)
	}

	// Get old (before the suspend) histograms if exist. Usually this is empty.
	older, err := getHistograms(ctx, s, service, defaultMetrics)
	if err != nil {
		s.Fatal("Failed to get Histograms from DUT: ", err)
	}

	prev := older
	seconds := defaultSuspendSeconds

	for i := 0; i < args.numSuspend; i++ {
		if err := startTracing(ctx, s, cl); err != nil {
			s.Log("Failed to start tracing, but this is ignorable: ", err)
		}

		// Suspend and resume
		s.Logf("Suspending DUT for %d seconds", seconds)
		req := powerpb.SuspendRequest{Seconds: int32(seconds)}
		if res, err := service.Suspend(ctx, &req); err != nil {
			if res != nil && res.Failed {
				if res.Output != "" {
					s.Logf("Suspend command failed, the command output is: %s", res.Output)
				}
				s.Fatal("Failed to suspend DUT: ", err)
			}
			s.Log("Ignore suspend command error if connection is lost: ", err)

			// Reconnect because suspend can disconnect network.
			cl, err = redialRPC(ctx, s, defaultRedialTimeoutSeconds+seconds)
			if err != nil {
				s.Fatal("Failed to reconnect the RPC: ", err)
			}
			// defer cl.Close() is already set.
		}
		s.Log("Resumed")

		s.Log("Wait for suspend metrics update")
		service = powerpb.NewSuspendPerfServiceClient(cl.Conn)
		prev, err = waitHistogramsUpdate(ctx, s, service, prev)
		if err != nil {
			s.Fatal("Could not observe histogram update: ", err)
		}
		saveTraceData(ctx, s, cl, i)
	}
	newer := prev

	// Make differences of Histograms.
	diff, err := histogram.DiffHistograms(older, newer)
	if err != nil {
		s.Fatal("Failed to make difference of histograms: ", err)
	}

	// Write perf metrics from the Diff Histogram and save it.
	pv := perf.NewValues()
	writeMetricsFromHistograms(diff, pv)
	err = pv.Save(s.OutDir())
	if err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

func redialRPC(ctx context.Context, s *testing.State, timeoutSeconds int) (*rpc.Client, error) {
	d := s.DUT()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Set a short timeout to the iteration in case of
		// any SSH operations blocking for a long time.
		ctx, cancel := context.WithTimeout(ctx, remoteCommandTimeout)
		defer cancel()

		if err := d.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to DUT after suspend")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  time.Duration(timeoutSeconds) * time.Second,
		Interval: defaultWaitInterval,
	}); err != nil {
		return nil, err
	}

	return rpc.Dial(ctx, d, s.RPCHint())
}

func shouldRunTraceCmd() bool {
	return traceCmdEventsVar.Value() != ""
}

// initTracing creates an instance in DUT
func initTracing(ctx context.Context, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	// Add a new tracing instance for this test
	// TODO: tune the parameters
	_, err := tracing.NewRemoteInstance(ctx, cl, defaultInstanceName,
		tracing.CPUBufferKiB(defaultBufferSize),
		tracing.InitialStop(),
		tracing.EnableEvents(strings.Split(traceCmdEventsVar.Value(), ",")...))
	return err
}

// startTracing starts tracing in DUT
func startTracing(ctx context.Context, s *testing.State, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	if err := tracing.StartRemoteInstanceTrace(ctx, cl, defaultInstanceName); err != nil {
		return errors.Wrap(err, "failed to reconnect tracing")
	}
	return nil
}

// saveTraceData fetches the trace data from DUT and save it in s.OutDir().
func saveTraceData(ctx context.Context, s *testing.State, cl *rpc.Client, i int) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	dest := fmt.Sprintf("%s/trace-%d.dat", s.OutDir(), i)
	if err := tracing.SaveRemoteInstanceTraceData(ctx, cl, defaultInstanceName,
		func(src string) error {
			return s.DUT().GetFile(ctx, src, dest)
		}); err != nil {
		return errors.Wrap(err, "failed to copy the data file from DUT")
	}
	s.Logf("Save trace data into %q", dest)

	return nil
}

func cleanupTracing(ctx context.Context, s *testing.State, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	if cl == nil {
		return errors.Errorf("failed to cleaning up a trace instance: %q", defaultInstanceName)
	}
	s.Logf("Cleaning up a trace instance: %s", defaultInstanceName)
	return tracing.CleanupRemoteInstance(ctx, cl, defaultInstanceName)
}

func waitHistogramsUpdate(ctx context.Context, s *testing.State, service powerpb.SuspendPerfServiceClient, prev []*histogram.Histogram) ([]*histogram.Histogram, error) {

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		curr, err := getHistograms(ctx, s, service, defaultMetrics)
		if err != nil {
			return err
		}

		diff, err := histogram.DiffHistograms(prev, curr)
		if err != nil {
			return err
		}

		for _, h := range diff {
			if h.TotalCount() == 0 {
				return errors.Errorf("histogram %s is still empty", h.Name)
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  defaultWaitTimeout,
		Interval: defaultWaitInterval,
	}); err != nil {
		return nil, err
	}

	return getHistograms(ctx, s, service, defaultMetrics)
}

func getHistograms(ctx context.Context, s *testing.State, service powerpb.SuspendPerfServiceClient, names []string) ([]*histogram.Histogram, error) {
	var hists []*histogram.Histogram
	for _, n := range names {
		hist, err := getHistogram(ctx, s, service, n)
		if err != nil {
			return nil, err
		}
		hists = append(hists, hist)
	}
	return hists, nil
}

func getHistogram(ctx context.Context, s *testing.State, service powerpb.SuspendPerfServiceClient, name string) (*histogram.Histogram, error) {

	req := powerpb.HistogramRequest{Name: name}

	res, err := service.GetHistogram(ctx, &req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get a histogram of "+name)
	}

	return histogram.NewHistogramFromProto(res), nil
}

func writeMetricsFromHistograms(hs []*histogram.Histogram, pv *perf.Values) {
	for _, h := range hs {
		writeMetricsFromHistogram(h, pv)
	}
}

// writeMetricsFromHistogram writes <histname>_mean, <histname>_p50 (median), <histname>_p100 (max) to @pv
func writeMetricsFromHistogram(hist *histogram.Histogram, pv *perf.Values) {
	if hist.TotalCount() == 0 {
		return
	}
	mean, err := hist.Mean()
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_mean", hist.Name),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, mean)
	}
	p50, err := hist.Percentile(50)
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_p50", hist.Name),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, p50)
	}
	p100, err := hist.Percentile(100)
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_p100", hist.Name),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, p100)
	}
}
