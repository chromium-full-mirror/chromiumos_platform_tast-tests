// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package setup contains helpers to set up a DUT for a power test.
package setup

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/power"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CleanupCallback cleans up a single setup item.
type CleanupCallback func(context.Context) error

// PowerTestSetup is a util function that allows power fixtures and other
// fixtures / tests to set up the DUT for power testing.
func PowerTestSetup(ctx context.Context, name string, tconn *chrome.TestConn, powerTestOptions *PowerTestOptions) (cleanup CleanupCallback, err error) {
	su, cleanup := New(name)

	discharge := false
	if _, err := power.SysfsBatteryPath(ctx); err == nil {
		discharge = true
	} else if errors.Is(err, power.ErrNoBattery) {
		// If it's ErrNoBattery, leave dischargeMode at NoBatteryDischarge.
		testing.ContextLog(ctx, "Unable to find battery, do not force discharge: ", err)
	} else {
		testing.ContextLog(ctx, "Unable to determine if a battery exists, do not force discharge: ", err)
	}

	su.Add(PowerTest(ctx, tconn, *powerTestOptions, NewBatteryDischarge(discharge, true /*ignoreErr*/, DefaultDischargeThreshold)))
	if err := su.Check(ctx); err != nil {
		cleanup(ctx)
		return nil, errors.Wrap(err, "power test options setup failed for fixture "+name)
	}
	return cleanup, nil
}

// Nested is used by setup items that have multiple stages that need separate
// cleanup callbacks.
func Nested(ctx context.Context, name string, nestedSetup func(s *Setup) error) (CleanupCallback, error) {
	s, callback := New(name)
	succeeded := false
	defer func() {
		if !succeeded {
			callback(ctx)
		}
	}()
	testing.ContextLogf(ctx, "Setting up %q", name)
	if err := nestedSetup(s); err != nil {
		return nil, err
	}
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	succeeded = true
	return callback, nil
}

// Setup accumulates the results of setup items so that their results can be
// checked, errors logged, and cleaned up.
type Setup struct {
	name      string
	callbacks []CleanupCallback
	errs      []error
}

// New creates a Setup object to collect the results of setup items, and a
// cleanup function that should be immediately deferred to make sure cleanup
// callbacks are called.
func New(name string) (*Setup, CleanupCallback) {
	s := &Setup{
		name:      name,
		callbacks: nil,
		errs:      nil,
	}
	cleanedUp := false
	return s, func(ctx context.Context) error {
		if cleanedUp {
			return errors.Errorf("cleanup %q has already been called", name)
		}
		cleanedUp = true

		if count, err := s.cleanUp(ctx); err != nil {
			return errors.Wrapf(err, "cleanup %q had %d items fail, first failure", name, count)
		}
		return nil
	}
}

// cleanUp is a helper that runs all cleanup callbacks and logs any failures.
// Returns true if all cleanup
func (s *Setup) cleanUp(ctx context.Context) (errorCount int, firstError error) {
	for _, c := range s.callbacks {
		// Defer cleanup calls so that if any of them panic, the rest still run.
		defer func(callback CleanupCallback) {
			if err := callback(ctx); err != nil {
				errorCount++
				if firstError == nil {
					firstError = err
				}
				testing.ContextLogf(ctx, "Cleanup %q failed: %s", s.name, err)
			}
		}(c)
	}
	return 0, nil
}

// Add adds a result to be checked or cleaned up later.
func (s *Setup) Add(callback CleanupCallback, err error) {
	if callback != nil {
		s.callbacks = append(s.callbacks, callback)
	}
	if err != nil {
		s.errs = append(s.errs, err)
	}
}

// Check checks if any Result shows a failure happened. All failures are logged,
// and a summary of failures is returned.
func (s *Setup) Check(ctx context.Context) error {
	for _, err := range s.errs {
		testing.ContextLogf(ctx, "Setup %q failed: %s", s.name, err)
	}
	if len(s.errs) > 0 {
		return errors.Wrapf(s.errs[0], "setup %q had %d items fail, first failure", s.name, len(s.errs))
	}
	return nil
}

// PowerdMode indicates what powerd setup is needed for a test.
type PowerdMode int

const (
	// DisablePowerd indicates that powerd should be disabled.
	DisablePowerd PowerdMode = iota
	// DoNotChangePowerd indicates that powerd should be left in the same state.
	DoNotChangePowerd
)

// BatteryDischargeMode what setup is needed for a test
type BatteryDischargeMode int

const (
	// NoBatteryDischarge requests setup not to try forcing discharge of battery.
	NoBatteryDischarge BatteryDischargeMode = iota
	// ForceBatteryDischarge requests setup to force discharging battery during a test.
	ForceBatteryDischarge
)

// BatteryDischarge contains the information used for battery discharge.
type BatteryDischarge struct {
	// discharge indicates if battery discharging needs to be performed.
	discharge bool
	// threshold is the maximum battery capacity percentage allowed before discharging.
	threshold float64
	// ignoreErr indicates to ignore discharge error when doing PowerTest setup.
	ignoreErr bool
	// err stores the battery discharging error if any.
	err error
}

// DefaultDischargeThreshold is the default battery discharge threshold.
const DefaultDischargeThreshold = 15.0

// fulfill performs the battery discharge during power test setup.
func (battery *BatteryDischarge) fulfill(ctx context.Context, s *Setup) {
	if !battery.discharge {
		return
	}
	var cleanup CleanupCallback
	cleanup, battery.err = SetBatteryDischarge(ctx, battery.threshold)
	if battery.ignoreErr {
		if battery.err != nil {
			testing.ContextLog(ctx, "Failed to set battery discharge: ", battery.err)
		}
		// Don't add err into Setup procedure.
		s.Add(cleanup, nil)
	} else {
		s.Add(cleanup, battery.err)
	}

	// If force battery fails, change the flag to reflect this failure.
	if battery.err != nil {
		battery.discharge = false
	}
}

// Err returns the battery discharge error.
func (battery *BatteryDischarge) Err() error {
	return battery.err
}

// NewBatteryDischarge returns a new *BatteryDischarge based on the parameters.
func NewBatteryDischarge(discharge, ignoreErr bool, threshold float64) *BatteryDischarge {
	return &BatteryDischarge{discharge: discharge, ignoreErr: ignoreErr, threshold: threshold}
}

// UpdateEngineMode indicates what update engine setup is needed for a test.
type UpdateEngineMode int

const (
	// DisableUpdateEngine indicates that update engine should be disabled.
	DisableUpdateEngine UpdateEngineMode = iota
	// DoNotChangeUpdateEngine indicates that update engine should be left in the same state.
	DoNotChangeUpdateEngine
)

// VNCMode indicates what VNC setup is needed for a test.
type VNCMode int

const (
	// DisableVNC indicates that vnc should be disabled.
	DisableVNC VNCMode = iota
	// DoNotChangeVNC indicates that vnc should be left in the same state.
	DoNotChangeVNC
)

// AvahiMode indicates what Avahi setup is needed for a test.
type AvahiMode int

const (
	// DisableAvahi indicates that Avahi should be disabled to stop multicast.
	DisableAvahi AvahiMode = iota
	// DoNotChangeAvahi indicates that Avahi should be left in the same state.
	DoNotChangeAvahi
)

// DPTFMode indicates what  DPTF setup is needed for a test.
type DPTFMode int

const (
	// DisableDPTF indicates that dptf should be disabled.
	DisableDPTF DPTFMode = iota
	// DoNotChangeDPTF indicates that dptf should be left in the same state.
	DoNotChangeDPTF
)

// BacklightMode indicates what backlight setup is needed for a test.
type BacklightMode int

const (
	// SetBacklight indicates that backlight should be set to a default value.
	SetBacklight BacklightMode = iota
	// DoNotChangeBacklight indicates that backlight should be left in the same state.
	DoNotChangeBacklight
	// SetBacklightToZero indicates that display backlight should be set to 0.
	SetBacklightToZero
)

// KbBrightnessMode indicates what keyboard brightness setup is needed for a test.
type KbBrightnessMode int

const (
	// SetKbBrightness indicates that backlight should be set to a default value.
	SetKbBrightness KbBrightnessMode = iota
	// DoNotChangeKbBrightness indicates that backlight should be left in the same state.
	DoNotChangeKbBrightness
	// SetKbBrightnessToZero indicates that keyboard backlight should be set to 0.
	SetKbBrightnessToZero
)

// WifiInterfacesMode describes how to setup WiFi interfaces for a test.
type WifiInterfacesMode int

const (
	// DoNotChangeWifiInterfaces indicates that WiFi interfaces should be left in the same state.
	DoNotChangeWifiInterfaces WifiInterfacesMode = iota
	// DisableWifiInterfaces indicates that WiFi interfaces should be disabled.
	DisableWifiInterfaces
)

// MulticastMode describes how to setup interface multicast for a test.
type MulticastMode int

const (
	// DisableMulticast indicates that multicast on ethernet/wlan should be disabled.
	DisableMulticast MulticastMode = iota
	// DoNotChangeMulticast indicates that multicast on ethernet/wlan should be left in the same state.
	DoNotChangeMulticast
)

// NightLightMode indicates what night light setup is needed for a test.
type NightLightMode int

const (
	// DoNotDisableNightLight indicates that Night Light should be left in the same state.
	DoNotDisableNightLight NightLightMode = iota
	// DisableNightLight indicates that Night Light should be disabled.
	DisableNightLight
	// EnableNightLight indicates that Night Light should be enabled.
	EnableNightLight
)

// DarkThemeMode indicates what dark theme setup is needed for a test.
type DarkThemeMode int

// If EnableDarkTheme or EnableLightTheme were to be used, make sure the test is on desktop
// when performing power setup at the start.
//
// Currently there is not a good way to cleanup the theme set.
// If EnableDarkTheme or EnableLightTheme were to be used, the device would remain in the
// theme until next log-in or manual inputs.
const (
	// DoNotChangeTheme indicates that OS theme should be left in the same state.
	DoNotChangeTheme DarkThemeMode = iota
	// EnableDarkTheme indicates that dark theme should be enabled.
	EnableDarkTheme
	// EnableLightTheme indicates that light theme should be enabled.
	EnableLightTheme
)

// AudioMode indicates what audio setup is needed for a test.
type AudioMode int

const (
	// Mute indicates that audio should be muted.
	Mute AudioMode = iota
	// DoNotChangeAudio indicates that audio should be left in the same state.
	DoNotChangeAudio
)

// BluetoothMode indicates what Bluetooth setup is needed for a test.
type BluetoothMode int

const (
	// DisableBluetoothInterfaces indicates that bluetooth should be disabled.
	DisableBluetoothInterfaces BluetoothMode = iota
	// DoNotChangeBluetooth indicates that bluetooth should be left in the same state.
	DoNotChangeBluetooth
)

// UIMode indicates whether upstart ui job should be on or off for a test.
type UIMode int

const (
	// DoNotChangeUI indicates that the test should not make changes to upstart ui job.
	DoNotChangeUI UIMode = iota
	// DisableUI indicates that upstart ui job should be disabled.
	DisableUI
)

// RamfsMode indicates whether to setup ramfs for local data.
type RamfsMode int

const (
	// DoNotSetupRamfs indicates that test should not setup ramfs.
	DoNotSetupRamfs RamfsMode = iota
	// SetupRamfs indicates that test should setup ramfs for local data.
	SetupRamfs
)

// PowerTestOptions describes how to set up a power test.
type PowerTestOptions struct {
	// The default value of the following options is not to perform any changes.
	Wifi       WifiInterfacesMode
	NightLight NightLightMode
	DarkTheme  DarkThemeMode
	UI         UIMode
	Ramfs      RamfsMode

	// The default value of the following options is to perform the actions.
	Powerd             PowerdMode
	UpdateEngine       UpdateEngineMode
	VNC                VNCMode
	Avahi              AvahiMode
	DPTF               DPTFMode
	Backlight          BacklightMode
	KeyboardBrightness KbBrightnessMode
	Audio              AudioMode
	Bluetooth          BluetoothMode
	Multicast          MulticastMode
}

// PowerTest configures a DUT to run a power test by disabling features that add
// noise, and consistently configuring components that change power draw.
func PowerTest(ctx context.Context, c *chrome.TestConn, options PowerTestOptions, batteryDischarge *BatteryDischarge) (CleanupCallback, error) {
	return Nested(ctx, "power test", func(s *Setup) error {
		if options.Powerd == DisablePowerd {
			s.Add(DisableService(ctx, "powerd"))
		}
		if options.UpdateEngine == DisableUpdateEngine {
			s.Add(DisableServiceIfExists(ctx, "update-engine"))
		}
		if options.VNC == DisableVNC {
			s.Add(DisableServiceIfExists(ctx, "vnc"))
		}
		if options.Avahi == DisableAvahi {
			s.Add(DisableServiceIfExists(ctx, "avahi"))
		}
		if options.DPTF == DisableDPTF {
			s.Add(DisableServiceIfExists(ctx, "dptf"))
		}
		if options.Backlight == SetBacklight {
			s.Add(SetBacklightLux(ctx, 150, batteryDischarge.discharge))
		}
		if options.Backlight == SetBacklightToZero {
			s.Add(SetBacklightBrightnessLinearPercent(ctx, 0))
		}
		if options.KeyboardBrightness == SetKbBrightness {
			s.Add(SetKbBrightnessHoverALSLux(ctx, 0))
		}
		if options.KeyboardBrightness == SetKbBrightnessToZero {
			s.Add(SetKeyboardBrightness(ctx, 0))
		}
		if options.Audio == Mute {
			s.Add(MuteAudio(ctx))
		}
		// avahi needs to be disabled before disabling multicast
		if options.Multicast == DisableMulticast {
			s.Add(DisableAllMulticast(ctx))
		}
		if options.Wifi == DisableWifiInterfaces {
			s.Add(DisableWiFiAdaptors(ctx))
		}
		if batteryDischarge != nil {
			batteryDischarge.fulfill(ctx, s)
		}
		if options.Bluetooth == DisableBluetoothInterfaces {
			s.Add(DisableBluetooth(ctx))
		}
		if options.NightLight == DisableNightLight {
			s.Add(TurnOffNightLight(ctx, c))
		}
		if options.NightLight == EnableNightLight {
			s.Add(TurnOnNightLight(ctx, c))
		}
		if options.DarkTheme == EnableDarkTheme {
			s.Add(TurnOnDarkTheme(ctx, c))
		}
		if options.DarkTheme == EnableLightTheme {
			s.Add(TurnOnLightTheme(ctx, c))
		}
		if options.UI == DisableUI {
			s.Add(DisableServiceIfExists(ctx, "ui"))
		}
		if options.Ramfs == SetupRamfs {
			s.Add(setUpRamfs(ctx))
		}
		return nil
	})
}
