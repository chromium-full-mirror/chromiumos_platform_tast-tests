// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	facade "go.chromium.org/tast-tests/cros/local/bluetooth/facade"
	facadecommon "go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var idleTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 10 * time.Minute}
var idleTracingTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 20 * time.Minute}
var idleFastTimeParams = power.TimeParams{Interval: 10 * time.Second, Total: 1 * time.Minute}

var displayOffBTOff = power.IdleParams{
	DisplayPower:   false,
	BluetoothPower: false,
	PSRState:       display.PSRDefault,
	IdleTimeParams: idleTimeParams}
var displayOnBTOff = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: false,
	PSRState:       display.PSRDefault,
	IdleTimeParams: idleTimeParams}

// By default, both display and bluetooth are on. So tests using this parameter
// can be seen as the default version of the idle test.
var displayOnBTOn = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: true,
	PSRState:       display.PSRDefault,
	IdleTimeParams: idleTimeParams}
var displayOffBTOn = power.IdleParams{
	DisplayPower:   false,
	BluetoothPower: true,
	PSRState:       display.PSRDefault,
	IdleTimeParams: idleTimeParams}

var defaultFast = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: true,
	PSRState:       display.PSRDefault,
	IdleTimeParams: idleFastTimeParams}

var tracingIdle = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: true,
	CollectTrace:   true,
	PSRState:       display.PSRDefault,
	IdleTimeParams: idleTracingTimeParams}

// Disable PSR to better understand the effect of display refresh rate on power.
var displayOnPSROff = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: false,
	PSRState:       display.PSRForceDisable,
	IdleTimeParams: idleTimeParams}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Idle,
		Desc:         "Collects data on idle with Chrome logged in",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-power-team@google.com", "jingmuli@google.com"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:    "display_off_bt_off_ash",
			Fixture: "powerAsh",
			Val:     displayOffBTOff,
			ExtraAttr: []string{
				"group:power",
				"power_regression",
				"group:release-health",
				"release-health_power",
			},
		}, {
			Name:    "display_on_bt_off_ash",
			Fixture: "powerAsh",
			Val:     displayOnBTOff,
		}, {
			Name:    "display_on_bt_on_ash",
			Fixture: "powerAsh",
			Val:     displayOnBTOn,
			ExtraAttr: []string{
				"group:power",
				"power_regression",
				"group:release-health",
				"release-health_power",
			},
		}, {
			Name:              "display_on_bt_on_ash_arc",
			Fixture:           "powerAshARC",
			Val:               displayOnBTOn,
			ExtraSoftwareDeps: []string{"arc"},
		}, {
			Name:    "display_off_bt_on_ash",
			Fixture: "powerAsh",
			Val:     displayOffBTOn,
		}, {
			Name:    "default_fast_ash",
			Fixture: "powerAsh",
			Val:     defaultFast,
			ExtraAttr: []string{
				"group:power",
				"power_daily",
				"power_weekly",
				"group:release-health",
				"release-health_power",
			},
		}, {
			Name:      "tracing_display_on_bt_on_ash",
			Fixture:   "powerAsh",
			Val:       tracingIdle,
			ExtraData: []string{tracing.TBMTracedProbesConfigFile},
		}, {
			Name:    "refresh_high_psr",
			Fixture: "powerAshHighRefresh",
			Val:     displayOnBTOff,
		}, {
			Name:    "refresh_low_psr",
			Fixture: "powerAshLowRefresh",
			Val:     displayOnBTOff,
		}, {
			Name:    "refresh_high",
			Fixture: "powerAshHighRefresh",
			Val:     displayOnPSROff,
		}, {
			Name:    "refresh_low",
			Fixture: "powerAshLowRefresh",
			Val:     displayOnPSROff,
		}, {
			Name:    "refresh_vrr",
			Fixture: "powerAshVRR",
			Val:     displayOnPSROff,
		}},
	})
}

func Idle(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	// Open a window with about:blank tab on the target browser.
	conn, err := cr.NewConn(ctx, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch())
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	var params = s.Param().(power.IdleParams)
	var interval = s.Param().(power.IdleParams).IdleTimeParams.Interval

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)

	s.Logf("Display is on %t, BT is on %t", params.DisplayPower, params.BluetoothPower)
	if params.DisplayPower {
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOn); err != nil {
			s.Fatal("Failed to turn on display: ", err)
		}
	} else {
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOff); err != nil {
			s.Fatal("Failed to turn off display: ", err)
		}
	}

	cleanupBluetooth, err := setBluetoothPower(ctx, params.BluetoothPower)
	if err != nil {
		s.Fatalf("Failed to set Bluetooth power to %t: %v", params.BluetoothPower, err)
	}
	defer func(ctx context.Context) {
		if err := cleanupBluetooth(ctx); err != nil {
			s.Log("Failed to restore Bluetooth power state: ", err)
		}
	}(cleanupCtx)

	if err := display.SetPSRState(params.PSRState); err != nil {
		s.Error("Failed to set psr state: ", err)
	}
	if params.PSRState != display.PSRDefault {
		defer display.SetPSRState(display.PSRDefault)
	}

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if params.CollectTrace {
		var session *tracing.Session
		traceConfigPath := s.DataPath(tracing.TBMTracedProbesConfigFile)
		traceDataPath := filepath.Join(s.OutDir(), "trace.perfetto-trace")
		session, err = tracing.StartSession(ctx, traceConfigPath, tracing.WithTraceDataPath(traceDataPath))
		if err != nil {
			s.Fatal("Failed to start tracing: ", err)
		}
		s.Log("Collecting Perfetto trace File at: ", session.TraceDataPath())

		defer session.Finalize(cleanupCtx)
		defer session.Stop(cleanupCtx)
	}

	// Triple the netdev budget to mitigate network noise.
	if err := testexec.CommandContext(ctx, "sysctl", "net.core.netdev_budget_usecs=6000", "net.core.netdev_budget=900").Run(); err != nil {
		s.Fatal("Can't set netdev budget: ", err)
	}
	defer testexec.CommandContext(cleanupCtx, "sysctl", "net.core.netdev_budget_usecs=2000", "net.core.netdev_budget=300").Run()

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, time.Duration(params.IdleTimeParams.Total)); err != nil {
		s.Fatal("Failed to wait idling: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}

// setBluetoothPower aims to set the Bluetooth power state of a device, handling
// potential errors and falling back to a default Bluetooth stack if necessary.
func setBluetoothPower(ctx context.Context, bluetoothPower bool) (func(context.Context) error, error) {
	btStack, err := facade.GetBluetoothStackType(ctx)
	if err != nil {
		// Log the error but continue with Bluez as a fallback.
		testing.ContextLog(ctx, "Failed to get Bluetooth stack type, using Bluez: ", err)
		btStack = facadecommon.BluetoothStackTypeBluez
	}

	cleanup := func(context.Context) error { return nil }
	btf, err := facade.NewBluetoothFacade(ctx, btStack)
	if err != nil {
		return cleanup, errors.Wrap(err, "failed to create Bluetooth facade")
	}

	poweredOn, err := btf.IsPoweredOn(ctx)
	if err != nil { // Handle potential error from IsPoweredOn.
		return cleanup, errors.Wrap(err, "failed to check if Bluetooth is powered on")
	}

	// Only set power state if it needs to be changed.
	if poweredOn != bluetoothPower {
		if err := btf.SetPowered(ctx, bluetoothPower); err != nil {
			return cleanup, errors.Wrapf(err, "failed to set the adapter enabled state from %t to %t", poweredOn, bluetoothPower)
		}
		cleanup = func(ctx context.Context) error { return btf.SetPowered(ctx, poweredOn) }
	}

	return cleanup, nil
}
