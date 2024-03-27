// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fwupd

import (
	"context"
	"reflect"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

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
	// GetRemotesMethod - Method name to get remotes
	GetRemotesMethod = "GetRemotes"
	// GetUpdatesMethod - Method name to get updates
	GetUpdatesMethod = "GetUpgrades"
	// QuitMethod - Method name to stop fwupd
	QuitMethod = "Quit"
)

// Fwupd structure maintains the auxiliary data needed for package methods.
type Fwupd struct {
	conn *dbus.Conn
	obj  dbus.BusObject
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
	RemoteId   string // NOLINT
	TrustFlags uint64
	Version    string
	Uri        string // NOLINT
}

// New opens connection to DBus and do any other initialization if needed.
// This methud must be called prior to other methods related to D-Bus.
func New() (fwupd *Fwupd, err error) {
	fwupd = new(Fwupd)

	// Don't close the shared connection.
	fwupd.conn, err = dbusutil.SystemBus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to system bus")
	}

	fwupd.obj = fwupd.conn.Object(DbusName, DbusPath)

	return fwupd, nil
}

func (fwupd *Fwupd) createDeviceFromRaw(ctx context.Context, rawDevice map[string]dbus.Variant) (device *Device, err error) {
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

func (fwupd *Fwupd) createReleaseFromRaw(ctx context.Context, rawRelease map[string]dbus.Variant) (release *Release, err error) {
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

func (fwupd *Fwupd) callGetDevices() ([]map[string]dbus.Variant, error) {
	var devices []map[string]dbus.Variant

	if err := fwupd.obj.Call(DbusInterface+"."+GetDevicesMethod, 0).Store(&devices); err != nil {
		return nil, errors.Wrap(err, "failed to call "+GetDevicesMethod)
	}

	return devices, nil
}

func (fwupd *Fwupd) releasesFromDbusCall(dbusMethod, deviceID string) ([]map[string]dbus.Variant, error) {
	var releases []map[string]dbus.Variant

	if err := fwupd.obj.Call(DbusInterface+"."+dbusMethod, 0, deviceID).Store(&releases); err != nil {
		return nil, errors.Wrap(err, "failed to call "+dbusMethod)
	}

	return releases, nil
}

// RestartDaemon restarts fwupd with the dbus method Quit call,
// proces should be restarted by service automatically.
func (fwupd *Fwupd) RestartDaemon(ctx context.Context) error {
	// Stop daemon.
	if call := fwupd.obj.Call(DbusInterface+"."+QuitMethod, 0); call.Err != nil {
		return errors.Wrap(call.Err, "failed to stop fwupd")
	}

	// Wait for daemon restart.
	if err := dbusutil.WaitForService(ctx, fwupd.conn, DbusName); err != nil {
		return errors.Wrap(err, "failed to start fwupd")
	}

	return nil
}

// DeviceByGUID returns a fwupd Device as known to fwupd that has a GUID
// matching the provided one.
func (fwupd *Fwupd) DeviceByGUID(ctx context.Context, expectedGUID string) (*Device, error) {
	devices, err := fwupd.callGetDevices()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get devices")
	}

	// Scan all devices to locate one with the expected GUID.
	for _, rawDevice := range devices {
		device, err := fwupd.createDeviceFromRaw(ctx, rawDevice)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the device")
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
func (fwupd *Fwupd) DeviceByID(ctx context.Context, expectedID string) (*Device, error) {
	devices, err := fwupd.callGetDevices()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get devices")
	}
	// Scan all devices to locate one with the expected GUID.
	for _, rawDevice := range devices {
		device, err := fwupd.createDeviceFromRaw(ctx, rawDevice)
		if device == nil || err != nil {
			return nil, errors.Wrap(err, "failed to create the device")
		}

		if device.DeviceId == expectedID {
			testing.ContextLog(ctx, "Found device: ", device)
			return device, nil
		}
	}

	return nil, errors.New("No device found with ID " + expectedID)
}

// DeviceDowngradeVersion returns the first available version to downgrade.
func (fwupd *Fwupd) DeviceDowngradeVersion(ctx context.Context, deviceID string) (string, error) {
	var downgrades []map[string]dbus.Variant
	if err := fwupd.obj.Call(DbusInterface+".GetDowngrades", 0, deviceID).Store(&downgrades); err != nil {
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
func (fwupd *Fwupd) Version() (string, error) {
	var version dbus.Variant
	var err error

	if version, err = fwupd.obj.GetProperty(DbusInterface + ".DaemonVersion"); err != nil {
		return "", errors.Wrap(err, "failed to get FWUPD version")
	}

	return version.String(), nil
}

// createReleasesFromRaw creates the array of releases from raw D-Bus data.
// Returns an error if even one release can not be created correctly.
func (fwupd *Fwupd) createReleasesFromRaw(ctx context.Context, rawReleases []map[string]dbus.Variant) (result []*Release, err error) {
	for _, rawRelease := range rawReleases {
		release, err := fwupd.createReleaseFromRaw(ctx, rawRelease)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create the release: %s", rawRelease)
		}
		result = append(result, release)
	}

	return result, nil
}

// UpdatesForDeviceID returns the Releases available for the given DeviceID
func (fwupd *Fwupd) UpdatesForDeviceID(ctx context.Context, deviceID string) (releases []*Release, err error) {
	updateMap, err := fwupd.releasesFromDbusCall(GetUpdatesMethod, deviceID)
	if err != nil {
		return nil, err
	}
	return fwupd.createReleasesFromRaw(ctx, updateMap)
}

// ReleasesForDeviceID returns all the Releases available for the given DeviceID
func (fwupd *Fwupd) ReleasesForDeviceID(ctx context.Context, deviceID string) (releases []*Release, err error) {
	releaseMap, err := fwupd.releasesFromDbusCall(GetReleasesMethod, deviceID)
	if err != nil {
		return nil, err
	}
	return fwupd.createReleasesFromRaw(ctx, releaseMap)
}
