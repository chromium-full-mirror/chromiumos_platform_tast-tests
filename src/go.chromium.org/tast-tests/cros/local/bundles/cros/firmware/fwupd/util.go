// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fwupd

import (
	"context"
	"reflect"
	"regexp"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ReleaseURI contains the release URI of the test webcam device in the system.
const ReleaseURI = "https://storage.googleapis.com/chromeos-localmirror/lvfs/test/3fab34cfa1ef97238fb24c5e40a979bc544bb2b0967b863e43e7d58e0d9a923f-fakedevice124.cab"

// ChargingStateTimeout has the time needed for polling battery charging state changes.
// It takes Brya about 3 minutes for the state to change from fully charged to discharging.
const ChargingStateTimeout = 10 * time.Minute

// FakeWebcamDeviceID is the DeviceID of the Fakecam installed on test devices
const FakeWebcamDeviceID string = "08d460be0f1f9f128413f816022a6439e0078018"

// FakeWebcamGUID is the GUID of the Fakecam installed on test devices
const FakeWebcamGUID string = "b585990a-003e-5270-89d5-3705a17f9a43"

// FakeWebcamName is the name of the Fakecam installed on test devices
const FakeWebcamName string = "Integrated Webcam™"

// FakeWebcamReleaseName is the name of all the releases of the Fakecam installed on test devices
const FakeWebcamReleaseName string = "FakeDevice"

// FakeWebcamVersion is the version of the Fakecam installed on test devices
const FakeWebcamVersion string = "1.2.2"

const (
	// This is a string that appears when the computer is discharging.
	dischargeString = `uint32 [0-9]\s+uint32 2`
)

// SetFwupdChargingState sets the battery charging state and polls for
// the appropriate change to be registered by powerd via its dbus
// method.
func SetFwupdChargingState(ctx context.Context, charge bool) (setup.CleanupCallback, error) {
	var localCleanup setup.CleanupCallback

	// Local cleanup function in case polling fails below
	defer func() {
		if localCleanup == nil {
			return
		}

		if err := localCleanup(ctx); err != nil {
			testing.ContextLog(ctx, "WARNING Failed to re-enable AC power: ", err)
		}
	}()

	if charge {
		if err := setup.AllowBatteryCharging(ctx); err != nil {
			return nil, err
		}

		// Return a no-op function to avoid a `cleanup != nil` check for the callers.
		localCleanup = func(ctx context.Context) error {
			return nil
		}
	} else {
		var err error
		if localCleanup, err = setup.SetBatteryDischarge(ctx, 20.0); err != nil {
			return nil, err
		}
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// fwupd is checking for the battery state to signal `discharging` instead
		// of checking if the AC power is disconnected. Some batteries won't
		// immediately change their state to `discharging` once the AC is
		// disconnected. Instead they will remain in the `fully charged` state
		// until the battery has discharged past some unknown threshold.
		// This call is here to force the battery to discharge enough so the
		// battery state changes. Ideally fwupd would use the presence of AC
		// instead of the battery state. If it did, we could them remove this
		// workaround.
		if !charge {
			cmd := testexec.CommandContext(ctx, "stressapptest", "-s", "5")
			testing.ContextLog(ctx, "Draining battery using: ", cmd)
			if err := cmd.Run(); err != nil {
				return err
			}
		}

		cmd := testexec.CommandContext(ctx, "dbus-send", "--print-reply", "--system", "--type=method_call",
			"--dest=org.chromium.PowerManager", "/org/chromium/PowerManager",
			"org.chromium.PowerManager.GetBatteryState")
		output, err := cmd.Output(testexec.DumpLogOnError)
		if err != nil {
			return err
		}

		if discharging, err := regexp.Match(dischargeString, output); err != nil {
			return err
		} else if (charge && !discharging) || (!charge && discharging) {
			return nil
		}

		return errors.New("powerd has not registered a battery state change")
	}, &testing.PollOptions{Timeout: ChargingStateTimeout}); err != nil {
		return nil, errors.Wrap(err, "battery polling was unsuccessful")
	}

	retCleanup := localCleanup
	// Disable the local cleanup function above
	localCleanup = nil

	return retCleanup, nil
}

// Device represents a hardware device supported by fwupd.
// Names are aligned with dbus properties for reflections below.
// See https://github.com/fwupd/fwupd/blob/main/libfwupd/fwupd-enums-private.h
type Device struct {
	Guid          []string // NOLINT
	DeviceId      string   // NOLINT
	Name          string
	InstanceIds   []string
	Plugin        string
	Problems      uint64
	UpdateError   string
	Version       string
	VersionFormat uint32
}

// Release represents a release of a hardware device supported by fwupd.
// Names are aligned with dbus properties for reflections below.
// See https://github.com/fwupd/fwupd/blob/main/libfwupd/fwupd-enums-private.h
//
// More values exist but for the purpose of these tests we only need these
// values.
type Release struct {
	Name       string
	TrustFlags uint64
	Version    string
}

const (
	// DbusName bus
	DbusName = "org.freedesktop.fwupd"
	// DbusPath object path
	DbusPath = "/"
	// DbusInterface interface
	DbusInterface = "org.freedesktop.fwupd"
	// GetDevicesMethod - Method name to get devices
	GetDevicesMethod = "GetDevices"
	// GetReleasesMethod - Method name to get releases
	GetReleasesMethod = "GetReleases"
	// GetUpgradesMethod - Method name to get updates
	GetUpgradesMethod = "GetUpgrades"

	// TrustedReportsReleaseFlagBit (9th bit) represents Trusted Reports value
	// in TrustFlags of the Release struct
	// Defined here: https://github.com/fwupd/fwupd/blob/main/libfwupd/fwupd-enums.h
	TrustedReportsReleaseFlagBit = 1 << 8
)

func inspectDevice(ctx context.Context, rawDevice map[string]dbus.Variant) (device *Device, err error) {
	testing.ContextLog(ctx, "Inspecting device: ", rawDevice)
	device = new(Device)
	devst := reflect.ValueOf(device).Elem()
	if !devst.CanAddr() {
		return nil, errors.New("cannot assign to the item passed, item must be a pointer in order to assign")
	}

	for i := 0; i < devst.NumField(); i++ {
		name := devst.Type().Field(i).Name
		if value, ok := rawDevice[name]; ok {
			fieldT := reflect.ValueOf(device).Elem().Field(i)
			fieldT.Set(reflect.ValueOf(value.Value()))
		}
	}

	return device, err
}

func inspectRelease(ctx context.Context, rawRelease map[string]dbus.Variant) (release *Release, err error) {
	testing.ContextLog(ctx, "Inspecting release: ", rawRelease)
	release = new(Release)
	relst := reflect.ValueOf(release).Elem()
	if !relst.CanAddr() {
		return nil, errors.New("cannot assign to the item passed, item must be a pointer in order to assign")
	}

	for i := 0; i < relst.NumField(); i++ {
		name := relst.Type().Field(i).Name
		if value, ok := rawRelease[name]; ok {
			fieldT := reflect.ValueOf(release).Elem().Field(i)
			fieldT.Set(reflect.ValueOf(value.Value()))
		}
	}

	return release, err
}

func getDevices() ([]map[string]dbus.Variant, error) {
	var devices []map[string]dbus.Variant
	// Don't close the shared connection.
	conn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to system bus")
	}

	fwupd := conn.Object(DbusName, DbusPath)

	if err = fwupd.Call(DbusInterface+"."+GetDevicesMethod, 0).Store(&devices); err != nil {
		return nil, errors.Wrap(err, "failed to call "+GetDevicesMethod)
	}

	return devices, nil
}

// getReleases returns the list of releases from the given device id;
func getReleases(ctx context.Context, deviceID string) ([]map[string]dbus.Variant, error) {
	var releases []map[string]dbus.Variant
	// Don't close the shared connection.
	conn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to system bus")
	}

	fwupd := conn.Object(DbusName, DbusPath)

	if err = fwupd.Call(DbusInterface+"."+GetReleasesMethod, 0, deviceID).Store(&releases); err != nil {
		return nil, errors.Wrap(err, "failed to call "+GetReleasesMethod)
	}

	return releases, nil
}

// DeviceByGUID returns a fwupd Device as known to fwupd that has a GUID
// matching the provided one.
func DeviceByGUID(ctx context.Context, expectedGUID string) (*Device, error) {
	devices, err := getDevices()
	if err != nil {
		return nil, err
	}

	// Scan all devices to locate one with the expected GUID.
	for _, rawDevice := range devices {
		device, err := inspectDevice(ctx, rawDevice)
		if device == nil {
			testing.ContextLogf(ctx, "Failed to inspect the device: %s, Error: %v", rawDevice, err)
			continue
		}
		if err != nil {
			return nil, err
		}

		for _, guid := range device.Guid {
			if guid == expectedGUID {
				testing.ContextLog(ctx, "Found device: ", device)
				return device, nil
			}
		}
	}

	return nil, errors.New("No device found with GUID " + expectedGUID)
}

// DeviceByID returns a fwupd Device ID matching the provided one.
func DeviceByID(ctx context.Context, expectedID string) (*Device, error) {
	devices, err := getDevices()
	if err != nil {
		return nil, err
	}
	// Scan all devices to locate one with the expected GUID.
	for _, rawDevice := range devices {
		device, err := inspectDevice(ctx, rawDevice)
		if device == nil {
			testing.ContextLogf(ctx, "Failed to inspect the device: %s, Error: %v", rawDevice, err)
			continue
		}

		if err != nil {
			return nil, err
		}

		if device.DeviceId == expectedID {
			testing.ContextLog(ctx, "Found device: ", device)
			return device, nil
		}
	}

	return nil, errors.New("No device found with ID " + expectedID)
}

// DeviceDowngradeVersion returns the first available version to downgrade.
func DeviceDowngradeVersion(ctx context.Context, deviceID string) (string, error) {
	// Don't close the shared connection.
	conn, err := dbusutil.SystemBus()
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to system bus")
	}
	fwupd := conn.Object(DbusName, DbusPath)

	var downgrades []map[string]dbus.Variant
	if err := fwupd.Call(DbusInterface+".GetDowngrades", 0, deviceID).Store(&downgrades); err != nil {
		return "", errors.Wrap(err, "error fetching downgrades for device "+deviceID)
	}

	// Using the first available downgrade version.
	for _, downgrade := range downgrades {
		testing.ContextLog(ctx, "Downgrade version:", downgrade["Version"])
		if _, ok := downgrade["Version"]; ok {
			var version string
			if err := dbus.Store([]interface{}{downgrade["Version"]}, &version); err != nil {
				return "", errors.Wrap(err, "failed to read version for downgrade")
			}
			return version, nil
		}
	}

	return "", errors.New("No usable updates found for " + deviceID)
}

// Version returns the version of fwupd daemon.
func Version(ctx context.Context) (string, error) {
	// Don't close the shared connection.
	conn, err := dbusutil.SystemBus()
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to system bus")
	}
	fwupd := conn.Object(DbusName, DbusPath)

	var version dbus.Variant
	if version, err = fwupd.GetProperty(DbusInterface + ".DaemonVersion"); err != nil {
		return "", errors.Wrap(err, "failed to get FWUPD version")
	}

	return version.String(), nil
}

// ReleasesForDeviceID returns the Releases available for the given DeviceID
func ReleasesForDeviceID(ctx context.Context, deviceID string) (releases []*Release, err error) {
	rawReleases, err := getReleases(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	for _, rawRelease := range rawReleases {
		release, err := inspectRelease(ctx, rawRelease)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to inspect the release: %s", rawRelease)
		}
		releases = append(releases, release)
	}

	return releases, nil
}
