// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package netperf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Ping,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ping different domains to determine the internet connection health",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"vsavu@google.com", // Test author
		},
		BugComponent: "b:1263917", // ChromeOS > Software > Commercial (Enterprise) > Testing
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:crosbolt",
			"crosbolt_nightly",
		},
		Params: []testing.Param{{
			Name: "gstatic",
			Val:  "gstatic.com",
		}, {
			Name: "tlsdate",
			Val:  "clients3.google.com",
		}},
	})
}

func Ping(ctx context.Context, s *testing.State) {
	domain := s.Param().(string)

	const pingTimeout = 10 * time.Second
	const totalTimeout = 1 * time.Minute

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Make sure we have enough time to run this test.
	expectedDeadline := time.Now().Add(totalTimeout)

	if deadline, hasDeadline := ctx.Deadline(); hasDeadline && expectedDeadline.After(deadline) {
		s.Fatalf("Test does not have enough time to run. Want to run until %s, can only run until %s", expectedDeadline.Format(time.RFC3339), deadline.Format(time.RFC3339))
	}

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed saving perf data: ", err)
		}
	}()

	defer func(context.Context) {
		dumpfile := "network_dump_" + time.Now().Format("030405000") + ".txt"
		if err := dumputil.DumpNetworkInfo(ctx, dumpfile); err != nil {
			s.Error(ctx, "Failed to dump network info: ", err)
		}
	}(cleanupCtx)

	count := 0
	start := time.Now()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		count = count + 1

		start := time.Now()
		if err := ping.ExpectPingSuccessWithTimeout(ctx, domain, "chronos", pingTimeout); err != nil {
			return err
		}
		elapsed := time.Now().Sub(start)

		pv.Set(perf.Metric{
			Name:      "latency",
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, (float64)(elapsed.Milliseconds()))

		return nil
	}, &testing.PollOptions{
		Timeout:  totalTimeout,
		Interval: 1 * time.Nanosecond, // Wait as little as possible between attempts.
	}); err != nil {
		s.Fatal("Failed to reach gstatic.com: ", err)
	}

	// Only log metrics if successful.
	totalDuration := time.Now().Sub(start)
	pv.Set(perf.Metric{
		Name:      "time_to_success",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, (float64)(totalDuration.Milliseconds()))

	pv.Set(perf.Metric{
		Name:      "attempts_to_success",
		Unit:      "attempts",
		Direction: perf.SmallerIsBetter,
	}, (float64)(count))
}
