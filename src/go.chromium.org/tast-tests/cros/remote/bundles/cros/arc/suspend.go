// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"math"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/dut"
	"go.chromium.org/tast-tests/cros/services/cros/arc"
	arcpb "go.chromium.org/tast-tests/cros/services/cros/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type testArgsForSuspend struct {
	suspendDurationSeconds          int
	suspendDurationAllowanceSeconds float64
	numTrialsPerLogin               int
	numLoginTrials                  int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Suspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks the behavior of ARC around suspend/resume",
		Contacts: []string{
			"cros-vm-technology@google.com",
			"hikalium@chromium.org",
		},
		// ChromeOS > Platform > Virtualization > VM Technology
		BugComponent: "b:930563",
		/* This test is hardware dependent */
		SoftwareDeps: []string{
			"chrome",
			"android_vm",
			"no_qemu", /* TODO(b/209400676): Remove this once the issue on betty is fixed. */
		},
		ServiceDeps: []string{"tast.cros.arc.SuspendService"},
		Timeout:     60 * time.Minute,
		Params: []testing.Param{{
			// TODO(b/214861486): Re-enable the test once it is stabilized
			// ExtraAttr: []string{"group:mainline", "informational"},
			Name: "s120c5",
			Val: testArgsForSuspend{
				suspendDurationSeconds:          120, /* Longer than CONFIG_RCU_CPU_STALL_TIMEOUT */
				suspendDurationAllowanceSeconds: 5,
				numTrialsPerLogin:               5,
				numLoginTrials:                  2,
			},
		}, {
			// TODO(b/214861486): Re-enable the test once it is stabilized
			// ExtraAttr: []string{"group:mainline", "informational"},
			Name: "s600c2",
			Val: testArgsForSuspend{
				suspendDurationSeconds:          600, /* Long enough to trigger watchdog timeouts */
				suspendDurationAllowanceSeconds: 5,
				numTrialsPerLogin:               2,
				numLoginTrials:                  2,
			},
		}, {
			// Simplest case
			// Suspend twice to ensure that the suspend duration is not doubly counted.
			// Also, re-login to ensure the suspend duration is correctly injected
			// beyond the crosvm's lifetime.
			ExtraAttr: []string{"group:mainline", "informational"},
			Name:      "s10c2",
			Val: testArgsForSuspend{
				suspendDurationSeconds:          10, /* Long enough to trigger watchdog timeouts */
				suspendDurationAllowanceSeconds: 5,
				numTrialsPerLogin:               2,
				numLoginTrials:                  2,
			},
		}},
	})
}

type clockDiffs struct {
	bootDiff time.Duration
	monoDiff time.Duration
}

type dutClockDiffs struct {
	host clockDiffs
	arc  clockDiffs
}

func readClocks(ctx context.Context, s *testing.State, params *arcpb.SuspendServiceParams) *arcpb.GetClockValuesResponse {
	// Establish an rpc connection again since RPC connections can be disconnected
	// when the DUT is suspended for a long time.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	service := arc.NewSuspendServiceClient(cl.Conn)
	res, err := service.GetClockValues(ctx, params)
	if err != nil {
		s.Fatal("SuspendService.GetClockValues returned an error: ", err)
	}
	return res
}

func calcClockDiffs(t0, t1 *arcpb.ClockValues) (clockDiffs, error) {
	t0Boottime, err := ptypes.Duration(t0.ClockBoottime)
	if err != nil {
		return clockDiffs{}, errors.Wrap(err, "failed to convert t0.ClockBoottime")
	}
	t1Boottime, err := ptypes.Duration(t1.ClockBoottime)
	if err != nil {
		return clockDiffs{}, errors.Wrap(err, "failed to convert t1.ClockBoottime")
	}
	t0Monotonic, err := ptypes.Duration(t0.ClockMonotonic)
	if err != nil {
		return clockDiffs{}, errors.Wrap(err, "failed to convert t0.ClockMonotonic")
	}
	t1Monotonic, err := ptypes.Duration(t1.ClockMonotonic)
	if err != nil {
		return clockDiffs{}, errors.Wrap(err, "failed to convert t1.ClockMonotonic")
	}
	return clockDiffs{
		bootDiff: time.Duration(t1Boottime - t0Boottime),
		monoDiff: time.Duration(t1Monotonic - t0Monotonic),
	}, nil
}

func calcDUTClockDiffs(t0, t1 *arcpb.GetClockValuesResponse) (dutClockDiffs, error) {
	host, err := calcClockDiffs(t0.Host, t1.Host)
	if err != nil {
		return dutClockDiffs{}, errors.Wrap(err, "failed to calc host diff")
	}
	arc, err := calcClockDiffs(t0.Arc, t1.Arc)
	if err != nil {
		return dutClockDiffs{}, errors.Wrap(err, "failed to calc arc diff")
	}
	return dutClockDiffs{
		host: host,
		arc:  arc,
	}, nil
}

func suspendDUT(ctx context.Context, s *testing.State, seconds int) {
	s.Logf("Suspending DUT for %d seconds", seconds)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds+30)*time.Second)
	defer cancel()
	if err := dut.SuspendDUT(ctx, s.DUT(), seconds); err != nil {
		s.Fatal("Failed to suspend: ", err)
	}
	s.Log("Resumed")
}

// Suspend tests ARC clock behavior around host's suspend / resume cycles
func Suspend(ctx context.Context, s *testing.State) {
	args := s.Param().(testArgsForSuspend)
	d := s.DUT()

	// Reboot first to make the host's suspend time zero
	d.Reboot(ctx)

	for k := 0; k < args.numLoginTrials; k++ {
		// Connect to the gRPC server on the DUT.
		cl, err := rpc.Dial(ctx, d, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(ctx)
		service := arc.NewSuspendServiceClient(cl.Conn)
		// Login to start ARC (re-login if it is logged in already)
		params, err := service.Prepare(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("SuspendService.Prepare returned an error: ", err)
		}
		defer func() {
			// Finalize may fail if the service connection is lost.
			// Resources used by the service will be freed up anyways, so ignoring the error here.
			_, err = service.Finalize(ctx, &empty.Empty{})
			if err != nil {
				s.Log("SuspendService.Finalize returned an error (ignorable): ", err)
			}
		}()
		for i := 0; i < args.numLoginTrials; i++ {
			s.Logf("Session %d Trial %d/%d", k+1, i+1, args.numTrialsPerLogin)

			t0 := readClocks(ctx, s, params)
			suspendDUT(ctx, s, args.suspendDurationSeconds)
			t1 := readClocks(ctx, s, params)

			diff, err := calcDUTClockDiffs(t0, t1)
			if err != nil {
				s.Fatal("Failed to calc clock diffs: ", err)
			}
			hostSuspendDuration := diff.host.bootDiff - diff.host.monoDiff
			s.Log("Host suspended ", hostSuspendDuration)
			arcSuspendDuration := diff.arc.bootDiff - diff.arc.monoDiff
			if math.Abs((arcSuspendDuration - hostSuspendDuration).Seconds()) > args.suspendDurationAllowanceSeconds {
				s.Fatalf("Suspend time was not injected to ARC properly, got %v, want %v", arcSuspendDuration, hostSuspendDuration)
			}
			s.Logf("OK: %v of suspend time was injected to ARC", arcSuspendDuration)
		}
	}

}
