// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/perf/perfpb"
	"go.chromium.org/tast-tests/cros/local/cpu"
	pow "go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Default metrics service parameters
const (
	defaultMetricsServiceInterval = 5 // default interval for MetricsService rpc in seconds.
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			power.RegisterMetricsServiceServer(srv, &MetricsService{s: s})
		},
	})
}

// MetricsService implements tast.cros.power.MetricsService.
type MetricsService struct {
	s        *testing.ServiceState
	timeline *perf.Timeline
	cleanup  setup.CleanupCallback
}

func (m *MetricsService) Setup(ctx context.Context, req *power.SetupRequest) (*empty.Empty, error) {
	if m.cleanup != nil {
		// We didn't clean up from the last time, do it now.
		if err := m.cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failure while running unexpected cleanup: ", err)
		}
		m.cleanup = nil
		return nil, errors.New("call cleanup before calling Setup again")
	}

	success := false
	cleanupCtx := ctx
	defer func(ctx context.Context) {
		if !success {
			testing.ContextLog(ctx, "Failure during MetricsService.Setup, cleaning up partially initialized components")
			if err := m.cleanup(ctx); err != nil {
				testing.ContextLog(ctx, "Failure while cleaning up after Setup failure: ", err)
			}
			m.cleanup = nil
		}
	}(cleanupCtx)

	su, cleanup := setup.New(req.TestName)
	m.cleanup = cleanup

	noUINoWifi := setup.PowerTestOptions{
		Wifi:               setup.DisableWifiInterfaces,
		UI:                 setup.DisableUI,
		Backlight:          setup.SetBacklightToZero,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}

	noUIWifi := setup.PowerTestOptions{
		UI:                 setup.DisableUI,
		Backlight:          setup.SetBacklightToZero,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}

	noUINoWifiBT := setup.PowerTestOptions{
		UI:                 setup.DisableUI,
		Backlight:          setup.SetBacklightToZero,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
		Wifi:               setup.DisableWifiInterfaces,
		Bluetooth:          setup.DoNotChangeBluetooth,
	}

	powerOptions := noUINoWifi

	if req.Fixture == power.SetupRequest_NO_UI_WIFI {
		powerOptions = noUIWifi
	}

	if req.Fixture == power.SetupRequest_NO_UI_NO_WIFI_BT {
		powerOptions = noUINoWifiBT
	}

	su.Add(setup.PowerTest(ctx, nil,
		powerOptions,
		setup.NewBatteryDischarge(true, /*discharge*/
			true, /*ignoreErr*/
			setup.DefaultDischargeThreshold),
	))
	if err := su.Check(ctx); err != nil {
		return nil, errors.Wrap(err, "Power test setup failed")
	}

	// Wait until CPU is cooled down and idle.
	err := cpu.Cooldown(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "CPU failed to cool down")
	}

	interval := req.GetIntervalSecond()
	if interval == 0 {
		interval = defaultMetricsServiceInterval
	}
	m.timeline, err = perf.NewTimeline(ctx, pow.TestMetrics(),
		perf.Interval(time.Duration(interval)*time.Second))
	if err != nil {
		return nil, err
	}

	success = true
	return &empty.Empty{}, nil
}

func (m *MetricsService) Start(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {

	// Cooldown before each repeated measurement
	err := cpu.Cooldown(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to cooldown: ", err)
	}

	if m.timeline == nil {
		return nil, errors.New("no timeline")
	}
	if err := m.timeline.Start(ctx); err != nil {
		return nil, err
	}
	if err := m.timeline.StartRecording(m.s.ServiceContext()); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

func (m *MetricsService) Finish(ctx context.Context, req *power.FinishRequest) (*perfpb.Values, error) {
	var err error

	p, err := m.timeline.StopRecording(m.s.ServiceContext())
	if err != nil {
		return nil, err
	}

	outDir := req.GetOutDir()
	if outDir != "" && filepath.IsAbs(outDir) {
		if _, err := os.Stat(outDir); os.IsNotExist(err) {
			dirs := strings.Split(outDir, "/")
			if dirs[1] == "tmp" {
				// The folder does not exist in local disk, create it in /tmp.
				err = os.MkdirAll(outDir, 0755)
				if err != nil {
					testing.ContextLog(ctx, "Fail to create folder: ", err)
				}
			}
		}
		if err == nil {
			err = pow.GeneratePowerLogAndSaveToCrosbolt(ctx,
				outDir,
				req.GetTestName(),
				perf.NewValuesFromProto(p.Proto()))
			if err != nil {
				testing.ContextLog(ctx, "Failed to upload: ", err)
			}
		}
	}
	return p.Proto(), nil
}

func (m *MetricsService) Cleanup(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if m.cleanup == nil {
		return nil, errors.New("nothing to clean up")
	}
	if err := m.cleanup(ctx); err != nil {
		return nil, err
	}
	m.cleanup = nil
	return &empty.Empty{}, nil
}
