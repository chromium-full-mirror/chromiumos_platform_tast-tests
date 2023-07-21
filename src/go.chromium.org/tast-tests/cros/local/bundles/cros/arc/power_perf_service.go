// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/perf/perfpb"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	arcpb "go.chromium.org/tast-tests/cros/services/cros/arc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			arcpb.RegisterPowerPerfServiceServer(srv, &PowerPerfService{s: s})
		},
	})
}

type PowerPerfService struct {
	s        *testing.ServiceState
	metrics  []perf.TimelineDatasource
	cleanups []func(ctx context.Context, firstError *error)
}

func (c *PowerPerfService) appendCleanup(cleanup func(ctx context.Context) error) {
	c.cleanups = append(c.cleanups, func(ctx context.Context, firstError *error) {
		if err := cleanup(ctx); err != nil {
			if *firstError == nil {
				*firstError = errors.Wrap(err, "failed to clean up PowerPerfService")
				return
			}
			testing.ContextLog(ctx, "Multiple failures cleaning up PowerPerfService: ", err)
		}
	})
}

func (c *PowerPerfService) cleanup(ctx context.Context) (firstError error) {
	defer func() { c.cleanups = nil }()
	for _, c := range c.cleanups {
		// NB: deferred calls run in the reverse order, so cleanup routines are
		// executed in the reverse order that they are appended.
		defer c(ctx, &firstError)
	}
	return nil
}

func (c *PowerPerfService) Setup(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if c.cleanups != nil {
		// We didn't clean up from the last time, do it now.
		if err := c.cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failure while running unexpected cleanups: ", err)
		}
		return nil, errors.New("call Powercleanups before calling PowerSetup again")
	}

	success := false
	defer func(ctx context.Context) {
		if !success {
			testing.ContextLog(ctx, "Failure during PowerPerfService.Setup, cleaning up partially initialized components")
			if err := c.cleanup(ctx); err != nil {
				testing.ContextLog(ctx, "Failure while cleaning up after Setup failure: ", err)
			}
		}
	}(ctx)

	// Set up Chrome.
	opts := []chrome.Option{
		chrome.ARCEnabled(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
		chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess", "--disable-features=FirmwareUpdaterApp"),
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}
	c.appendCleanup(cr.Close)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API")
	}

	// Set up Android.
	td, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a temp dir")
	}
	c.appendCleanup(func(ctx context.Context) error {
		return os.RemoveAll(td)
	})

	a, err := arc.New(ctx, td)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start ARC")
	}
	c.appendCleanup(a.Close)

	// Configure DUT for power test.
	sup, cleanupPower := setup.New("power perf")
	c.appendCleanup(cleanupPower)

	discharge := false
	if batteryPath, err := power.SysfsBatteryPath(ctx); err == nil {
		discharge = true
		// There is a battery, make sure it's charged before starting the test.
		testing.ContextLog(ctx, "Waiting for battery to charge")
		if err := power.WaitForCharge(ctx, batteryPath, 0.75, 30*time.Minute); err != nil {
			return nil, err
		}
		testing.ContextLog(ctx, "Battery now charged")
	} else if errors.Is(err, power.ErrNoBattery) {
		// If it's ErrNoBattery, leave dischargeMode at NoBatteryDischarge.
		testing.ContextLog(ctx, "Unable to find battery, do not force discharge: ", err)
	} else {
		return nil, errors.Wrap(err, "failed to get battery path")
	}

	// Wait until CPU is cooled down and idle.
	_, err = cpu.WaitUntilStabilized(ctx, cpu.IdleCoolDownConfig())
	if err != nil {
		return nil, errors.Wrap(err, "CPU failed to stabilize")
	}
	if err := arc.CheckNoDex2Oat(td); err != nil {
		return nil, errors.Wrap(err, "failed to verify dex2oat was not running")
	}

	sup.Add(setup.PowerTest(ctx, tconn,
		setup.PowerTestOptions{Wifi: setup.DisableWifiInterfaces, NightLight: setup.DisableNightLight},
		setup.NewBatteryDischarge(discharge, true /*ignoreErr*/, setup.DefaultDischargeThreshold),
	))
	if err := sup.Check(ctx); err != nil {
		return nil, errors.Wrap(err, "power perf setup failed")
	}

	// GoBigSleepLint: Waiting a bit to settle before measurement.
	if err := testing.Sleep(ctx, 90*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to sleep to settle before measurement")
	}

	success = true
	return &emptypb.Empty{}, nil
}

func (c *PowerPerfService) StartMeasurement(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if c.metrics != nil {
		return nil, errors.New("already measuring metrics")
	}

	c.metrics = []perf.TimelineDatasource{
		power.NewCpuidleStateMetrics(),
		power.NewPackageCStatesMetrics(),
		power.NewRAPLPowerMetrics(),
	}
	for _, metric := range c.metrics {
		if err := metric.Setup(ctx, "", ""); err != nil {
			return nil, errors.Wrap(err, "failed to setup metric")
		}
	}
	for _, metric := range c.metrics {
		if err := metric.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start metric")
		}
	}

	return &emptypb.Empty{}, nil
}

func (c *PowerPerfService) StopMeasurement(ctx context.Context, _ *emptypb.Empty) (*perfpb.Values, error) {
	if c.metrics == nil {
		return nil, errors.New("no metrics to stop measuring")
	}
	defer func() { c.metrics = nil }()

	p := perf.NewValues()
	for _, metric := range c.metrics {
		if err := metric.Snapshot(ctx, p); err != nil {
			return nil, errors.Wrap(err, "failed to snapshot metric")
		}
	}
	for _, metric := range c.metrics {
		if err := metric.Stop(ctx, p); err != nil {
			return nil, errors.Wrap(err, "failed to stop metric")
		}
	}
	return p.Proto(), nil
}

func (c *PowerPerfService) Cleanup(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if c.metrics != nil {
		testing.ContextLog(ctx, "Warning, PowerPerfService.StartMeasurement but no StopMeasurement")
		c.metrics = nil
	}
	if c.cleanups == nil {
		return nil, errors.New("nothing to clean up")
	}
	if err := c.cleanup(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
