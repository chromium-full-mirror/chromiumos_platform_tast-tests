// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/dut"
	powerpb "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
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
		ServiceDeps:  []string{"tast.cros.power.SuspendPerfService"},
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
	defaultSuspendSeconds = 10
)

// TODO make a new struct type with name and direction
var defaultMetrics = []string{"Power.KernelSuspendTimeOnAC", "Power.KernelResumeTimeOnAC", "Power.DisplayAfterResumeDurationMsOnAC"}

// Delay and timeout for waitHistogramsUpdate()
var defaultWaitInterval = time.Duration(2) * time.Second
var defaultWaitTimeout = time.Duration(40) * time.Second

func SuspendPerf(ctx context.Context, s *testing.State) {
	args := s.Param().(testArgsForSuspendPerf)

	// Login and setup
	if err := prepareDUT(ctx, s); err != nil {
		s.Fatal("Failed to setup test: ", err)
	}

	// Get old (before the suspend) histograms if exist. Usually this is empty.
	older, err := getHistograms(ctx, s, defaultMetrics)
	if err != nil {
		s.Fatal("Failed to get Histograms from DUT: ", err)
	}

	prev := older
	for i := 0; i < args.numSuspend; i++ {
		// Suspend and resume
		err = suspendDUT(ctx, s, defaultSuspendSeconds)
		if err != nil {
			s.Fatal("Could not suspend DUT: ", err)
		}
		s.Log("Wait for suspend metrics update")

		prev, err = waitHistogramsUpdate(ctx, s, prev)
		if err != nil {
			s.Fatal("Could not observe histogram update: ", err)
		}
	}
	newer := prev

	// Make differences of Histograms
	diff, err := histogram.DiffHistograms(older, newer)
	if err != nil {
		s.Fatal("Failed to make difference of histograms: ", err)
	}

	// Write perf metrics from the Diff Histogram and save it
	pv := perf.NewValues()
	writeMetricsFromHistograms(diff, pv)
	err = pv.Save(s.OutDir())
	if err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

func prepareDUT(ctx context.Context, s *testing.State) error {
	s.Log("====== Prepare DUT ")

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "can not connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	service := powerpb.NewSuspendPerfServiceClient(cl.Conn)
	_, err = service.Prepare(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to guest login")
	}

	return nil
}

func suspendDUT(ctx context.Context, s *testing.State, seconds int) error {
	s.Logf("====== Suspending DUT for %d seconds", seconds)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds+60)*time.Second)
	defer cancel()
	if err := dut.SuspendDUT(ctx, s.DUT(), seconds); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}
	s.Log("====== Resumed")
	return nil
}

func waitHistogramsUpdate(ctx context.Context, s *testing.State, prev []*histogram.Histogram) ([]*histogram.Histogram, error) {

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		curr, err := getHistograms(ctx, s, defaultMetrics)
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

	return getHistograms(ctx, s, defaultMetrics)
}

func getHistograms(ctx context.Context, s *testing.State, names []string) ([]*histogram.Histogram, error) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		return nil, errors.Wrap(err, "can not connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	var hists []*histogram.Histogram
	service := powerpb.NewSuspendPerfServiceClient(cl.Conn)

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
