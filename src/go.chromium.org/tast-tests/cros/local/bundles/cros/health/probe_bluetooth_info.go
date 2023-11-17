// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"sort"

	"github.com/godbus/dbus/v5"
	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast-tests/cros/local/set"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type deviceInfo struct {
	Address           string            `json:"address"`
	Name              *string           `json:"name"`
	Type              *string           `json:"type"`
	Appearance        *uint16           `json:"appearance"`
	Modalias          *string           `json:"modalias"`
	MTU               *uint16           `json:"mtu"`
	RSSI              *int16            `json:"rssi"`
	UUIDs             *[]string         `json:"uuids"`
	BatteryPercentage *uint8            `json:"battery_percentage"`
	Class             *jsontypes.Uint32 `json:"bluetooth_class"`
}

type adapterInfo struct {
	Address             string           `json:"address"`
	Name                string           `json:"name"`
	NumConnectedDevices jsontypes.Uint32 `json:"num_connected_devices"`
	Powered             bool             `json:"powered"`
	ConnectedDevices    []deviceInfo     `json:"connected_devices"`
	Discoverable        bool             `json:"discoverable"`
	Discovering         bool             `json:"discovering"`
	UUIDs               []string         `json:"uuids"`
	Modalias            string           `json:"modalias"`
	ServiceAllowList    []string         `json:"service_allow_list"`
}

type bluetoothInfo struct {
	Adapters []adapterInfo `json:"adapters"`
}

type bluetoothInfoTestParams struct {
	// If true, validate Bluetooth info via Bluez. Otherwise, via Floss.
	BluezValidation bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeBluetoothInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can fetch Bluetooth info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Params: []testing.Param{{
			Name: "",
			Val: bluetoothInfoTestParams{
				BluezValidation: true,
			},
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			Name: "bluez",
			Val: bluetoothInfoTestParams{
				BluezValidation: true,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithBlueZ",
			// TODO(b/303370425): Promote tast to critical
			ExtraAttr: []string{"informational"},
		}, {
			Name: "floss",
			Val: bluetoothInfoTestParams{
				BluezValidation: false,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/303370425): Promote tast to critical
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}},
	})
}

var targetAllowedServices = []string{"1108", "110b", "110c", "110d", "110e", "111e", "1200", "1800", "1801", "180a", "180f", "1812"}

// resetBluetoothAdapterData clean the preset properties in adapter.
func resetBluetoothAdapterData(ctx context.Context) error {
	// Clear allowed services.
	if b, err := testexec.CommandContext(ctx, "bluetoothctl", "admin.allow", "clear").Output(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "clear allowed service failed: %s", string(b))
	}
	return nil
}

// initiateBluetoothAdapterData setup the required data we want to validate.
// Because serviceAllowList is always an empty list in lab device, we set the
// policy before the testing.
func initiateBluetoothAdapterData(ctx context.Context) error {
	if err := resetBluetoothAdapterData(ctx); err != nil {
		return err
	}
	// Set the allowed services for validation.
	args := append([]string{"admin.allow"}, targetAllowedServices...)
	if b, err := testexec.CommandContext(ctx, "bluetoothctl", args...).Output(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "set allowed service failed: %s", string(b))
	}
	return nil
}

func validateBluetoothAdapterData(ctx context.Context, info *bluetoothInfo) error {
	// Get Bluetooth adapter values to compare to the output of cros_healthd.
	adapters, err := bluez.Adapters(ctx)
	if err != nil {
		return err
	}

	if len(info.Adapters) != len(adapters) {
		return errors.Errorf("unexpected Bluetooth adapters count: got %d; want %d", len(info.Adapters), len(adapters))
	}

	if len(info.Adapters[0].ConnectedDevices) != int(info.Adapters[0].NumConnectedDevices) {
		return errors.Errorf("inconsistent number of connected Bluetooth devices: got %d; want %d",
			len(info.Adapters[0].ConnectedDevices),
			info.Adapters[0].NumConnectedDevices)
	}

	if err := validateAdapter(ctx, info, adapters[0]); err != nil {
		return err
	}

	if err := validateAdminPolicy(ctx, info, adapters[0]); err != nil {
		return err
	}

	return nil
}

// validateAdapter validate the data from Adapter1 interface.
func validateAdapter(ctx context.Context, info *bluetoothInfo, adapter *bluez.Adapter) error {
	if name, err := adapter.Name(ctx); err != nil {
		return err
	} else if info.Adapters[0].Name != name {
		return errors.Errorf("invalid name: got %s; want %s", info.Adapters[0].Name, name)
	}

	if address, err := adapter.Address(ctx); err != nil {
		return err
	} else if info.Adapters[0].Address != address {
		return errors.Errorf("invalid address: got %s; want %s", info.Adapters[0].Address, address)
	}

	if powered, err := adapter.Powered(ctx); err != nil {
		return err
	} else if info.Adapters[0].Powered != powered {
		return errors.Errorf("invalid powered value: got %v; want %v", info.Adapters[0].Powered, powered)
	}

	if discoverable, err := adapter.Discoverable(ctx); err != nil {
		return err
	} else if info.Adapters[0].Discoverable != discoverable {
		return errors.Errorf("invalid discoverable value: got %v; want %v", info.Adapters[0].Discoverable, discoverable)
	}

	if discovering, err := adapter.Discovering(ctx); err != nil {
		return err
	} else if info.Adapters[0].Discovering != discovering {
		return errors.Errorf("invalid discovering value: got %v; want %v", info.Adapters[0].Discovering, discovering)
	}

	if uuids, err := adapter.UUIDs(ctx); err != nil {
		return err
	} else if len(set.DiffStringSlice(info.Adapters[0].UUIDs, uuids)) != 0 {
		return errors.Errorf("invalid uuids value: got %v; want %v", info.Adapters[0].UUIDs, uuids)
	}

	if modalias, err := adapter.Modalias(ctx); err != nil {
		return err
	} else if info.Adapters[0].Modalias != modalias {
		return errors.Errorf("invalid modalias value: got %v; want %v", info.Adapters[0].Modalias, modalias)
	}

	return nil
}

// validateAdminPolicy validate the data from AdminPolicyStatus1 interface.
func validateAdminPolicy(ctx context.Context, info *bluetoothInfo, adapter *bluez.Adapter) error {
	if serviceAllowList, err := adapter.ServiceAllowList(ctx); err != nil {
		return err
	} else if len(serviceAllowList) != len(targetAllowedServices) {
		return errors.Errorf("unexpected allowed services count: got %d; want %d", len(serviceAllowList), len(targetAllowedServices))
	} else if len(set.DiffStringSlice(info.Adapters[0].ServiceAllowList, serviceAllowList)) != 0 {
		return errors.Errorf("invalid serviceAllowList value: got %v; want %v", info.Adapters[0].ServiceAllowList, serviceAllowList)
	}

	return nil
}

// validateConnectedDevices validate the property of connected devices of adapter.
func validateConnectedDevices(ctx context.Context, got []deviceInfo) error {
	// Get Bluetooth device values to compare to the output of cros_healthd.
	devices, err := bluez.Devices(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get bluez devices info")
	}

	// Get battery percentage for each D-Bus object.
	batteryPercentages := make(map[dbus.ObjectPath]uint8)
	batteries, err := bluez.Batteries(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get bluez batteries info")
	}
	for _, battery := range batteries {
		percentage, err := battery.Percentage(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get battery percentage")
		}
		batteryPercentages[battery.Path()] = percentage
	}

	expected := make([]deviceInfo, 0)
	for _, device := range devices {
		if connected, err := device.Connected(ctx); err != nil {
			// Handle unreleased device objects after stopping discovery.
			if dbusutil.IsDBusError(err, dbusutil.DBusErrorUnknownObject) {
				continue
			}
			return errors.Wrap(err, "failed to get device connected status")
		} else if !connected {
			continue
		}

		// The following are required properties.
		address, err := device.Address(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get device address")
		}
		info := deviceInfo{Address: address}

		// The following are optional properties.
		if name, err := device.Name(ctx); err == nil {
			info.Name = &name
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device name")
		}
		if deviceType, err := device.Type(ctx); err == nil {
			info.Type = &deviceType
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device type")
		}
		if appearance, err := device.Appearance(ctx); err == nil {
			info.Appearance = &appearance
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device appearance")
		}
		if modalias, err := device.Modalias(ctx); err == nil {
			info.Modalias = &modalias
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device modalias")
		}
		if mtu, err := device.MTU(ctx); err == nil {
			info.MTU = &mtu
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device MTU")
		}
		if rssi, err := device.RSSI(ctx); err == nil {
			info.RSSI = &rssi
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device RSSI")
		}
		if uuids, err := device.UUIDs(ctx); err == nil {
			info.UUIDs = &uuids
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device UUIDs")
		}
		if class, err := device.Class(ctx); err == nil {
			bluetoothClass := jsontypes.Uint32(class)
			info.Class = &bluetoothClass
		} else if !dbusutil.IsDBusError(err, dbusutil.DBusErrorInvalidArgs) {
			return errors.Wrap(err, "failed to get device class")
		}

		// Checks if the battery percentage exists for the D-Bus object path.
		if percentage, ok := batteryPercentages[device.Path()]; ok {
			info.BatteryPercentage = &percentage
		}
		expected = append(expected, info)
	}

	sort.Slice(expected, func(i, j int) bool { return expected[i].Address < expected[j].Address })
	sort.Slice(got, func(i, j int) bool { return got[i].Address < got[j].Address })

	if diff := cmp.Diff(expected, got); diff != "" {
		return errors.Errorf("connected devices attributes mismatch (-expected + got): %s", diff)
	}

	return nil
}

// ProbeBluetoothInfo is the main function of this tast test.
func ProbeBluetoothInfo(ctx context.Context, s *testing.State) {
	bluezValidation := s.Param().(bluetoothInfoTestParams).BluezValidation
	if bluezValidation {
		if err := initiateBluetoothAdapterData(ctx); err != nil {
			s.Fatal("Failed to initiate bluetooth adapter data: ", err)
		}
	}

	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryBluetooth}
	var info bluetoothInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &info); err != nil {
		s.Fatal("Failed to get Bluetooth telemetry info: ", err)
	}

	if len(info.Adapters) == 0 {
		s.Fatal("Failed to get Bluetooth adapter data: empty adapters slice")
	}

	if bluezValidation {
		if err := validateBluetoothAdapterData(ctx, &info); err != nil {
			s.Fatal("Failed to validate bluetooth adapter data: ", err)
		}

		if err := resetBluetoothAdapterData(ctx); err != nil {
			s.Fatal("Failed to reset bluetooth adapter data: ", err)
		}

		// Note that this validation doesn't mean we have test coverage of Bluetooth
		// devices. There are currently no Bluetooth devices available in the lab.
		if err := validateConnectedDevices(ctx, info.Adapters[0].ConnectedDevices); err != nil {
			s.Fatal("Failed to validate bluetooth device data: ", err)
		}
	}
	// TODO(b/303370425): Validate Bluetooth info via Floss.
}
