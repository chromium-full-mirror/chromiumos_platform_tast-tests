// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"context"
	"encoding/hex"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/cellular"
	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/timing"
)

const (
	defaultTimeout = 30 * time.Second
	longTimeout    = 120 * time.Second
	resetTimeout   = 3 * time.Minute

	// ConnectTimeout is the timeout for connect to a cellular network.
	ConnectTimeout = longTimeout
)

// Helper fetches Cellular Device and Service properties.
type Helper struct {
	Manager            *shill.Manager
	Device             *shill.Device
	enableEthernetFunc func(ctx context.Context)
	enableWifiFunc     func(ctx context.Context)
	di                 *cellular.DUTInfo
}

const (
	simLockCsvHeader       = "serial,attesteddeviceid,profiletype,profileid,expiration,model,manufacturer\n"
	simLockUnLockCsvHeader = "serial,attesteddeviceid,profiletype,profileid,expiration,owner,model\n"
)

const (
	userMyFilesPath = "/home/chronos/user/MyFiles/"
)

const (
	// SimLockUnlockProfileID Profile ID used in the production sim lock portal for unlocking the device
	SimLockUnlockProfileID = "0"
	// SimLockWildcardProfileID Profile ID used in the production sim lock portal for vacation/wildcard
	SimLockWildcardProfileID = "50046"
	// SimLockNetworkSelectionImsiPrefixProfileID Profile ID used in the production sim lock portal to test IMSI prefix
	SimLockNetworkSelectionImsiPrefixProfileID = "50051"
	// SimLockNetworkSelectionSpnMatchProfileID Profile ID used in the production sim lock portal to test SPN match
	SimLockNetworkSelectionSpnMatchProfileID = "50053"
	// SimLockVzwProfileID Verizon profile id in the production sim lock portal
	SimLockVzwProfileID = "50045"
	// SimLockTMoProfileID TMobile profile id in the production sim lock portal
	SimLockTMoProfileID = "50063"
	// SimLockExcludeVzwProfileID Test profile to exclude Verizon MCC/MNCs in production sim lock portal
	SimLockExcludeVzwProfileID = "50048"
)

// NewHelper creates a Helper object and ensures that a Cellular Device is present.
func NewHelper(ctx context.Context) (*Helper, error) {
	ctx, st := timing.Start(ctx, "Helper.NewHelper")
	defer st.End()

	manager, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Manager object")
	}
	available, err := manager.IsAvailable(ctx, shill.TechnologyCellular)
	if err != nil || !available {
		return nil, errors.Wrap(err, "Cellular Technology not available")
	}
	device, err := manager.DeviceByType(ctx, shillconst.TypeCellular)
	if err != nil || device == nil {
		return nil, errors.Wrap(err, "failed to get Cellular Device")
	}
	helper := Helper{Manager: manager, Device: device}

	if err := helper.EnsureEnabled(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find default Cellular Service")
	}

	// Start collecting DBus logs
	if err := helper.CaptureDBusLogs(ctx); err != nil {
		testing.ContextLog(ctx, "Warning: Unable to start DBus log capture: ", err)
	}
	return &helper, nil
}

// ClearSIMLockFromHostInfo uses PIN and PUK values from the host info labels
// to clear any SIM lock.
func (h *Helper) ClearSIMLockFromHostInfo(ctx context.Context) error {
	// Ensure that the host info has been parsed.
	if h.di == nil {
		return errors.New("SIM info has not been populated")
	}

	// Get ICCID and PIN/PUK codes
	iccid, err := h.GetCurrentICCID(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get current ICCID")
	}
	currentPin, currentPuk, err := h.GetPINAndPUKForICCID(ctx, iccid)
	if err != nil {
		return errors.Wrap(err, "could not get Pin and Puk")
	}

	// Disable pin lock with default pin and puk with dut puk if locked.
	if err := h.ClearSIMLock(ctx, currentPin, currentPuk); err != nil {
		return errors.Wrap(err, "failed to unlock dut with default pin")
	}

	return nil
}

func isL850Verizon(ctx context.Context, modem *modemmanager.Modem) bool {
	modemType, err := GetModemType(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get modem type: ", err)
		return false
	}
	if modemType != cellularconst.ModemTypeL850 {
		return false
	}
	operatorID, gid1, err := modem.GetOperatorAndGid1Identifiers(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get operator and GID1 identifiers: ", err)
		return false
	}
	carrier, err := GetCarrier(operatorID, gid1)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get carrier: ", err)
		return false
	}
	return carrier == CarrierVerizon
}

// CheckIfL850VerizonAndFixDefaultAPN checks if the device has a L850GL modem with a verizon SIM card,
// and tries to fix the default APN in the modem. This is needed because there are 2 bugs
// in the modem FW that causes the modem to report the last used APN as provisioned by the carrier(b/289540816, b/289530609).
func CheckIfL850VerizonAndFixDefaultAPN(ctx context.Context) error {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get new modem")
	}

	if !isL850Verizon(ctx, modem) {
		return nil
	}

	testing.ContextLog(ctx, "Verizon L850 device: Try fixing the default APN")
	h, err := NewHelper(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create helper")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	// Ensure the bearer object will be removed.
	defer modem.DeleteAllBearers(cleanupCtx, modem)

	// Ignore errors. Do best effort to fix the modem
	if enabled, _ := h.Manager.IsEnabled(ctx, shill.TechnologyCellular); enabled {
		_, _ = h.Disable(ctx)
		defer h.Enable(cleanupCtx)
	}

	if err := modem.Enable(ctx); err != nil {
		return errors.Wrap(err, "failed to enable modem")
	}
	if err := modem.DisconnectAll(ctx); err != nil {
		return errors.Wrap(err, "failed to disconnect all bearers")
	}
	// We don't use the profile-id number here because we want to override the APN in the modem to fix an invalid APN if there is one.
	if _, err := modem.Connect(ctx, map[string]interface{}{"apn": "vzwinternet", "ip-type": mmconst.BearerIPFamilyIPv4v6}); err != nil {
		return errors.Wrap(err, "failed to connect to APN")
	}
	if err := modem.DisconnectAll(ctx); err != nil {
		return errors.Wrap(err, "failed to disconnect all bearers")
	}

	return nil
}

// RebootL850VerizonIfModemCanNoLongerConnect checks if the device has a L850GL modem with a verizon SIM card,
// and tries to fix the modem by rebooting it if the device is stuck and cannot connect to the default verizon profile(b/309953824),
// which is the class 3 APN stored in the modem, or the modb APN.
// Only returns an error if the modem object is not valid.
func RebootL850VerizonIfModemCanNoLongerConnect(ctx context.Context, modemPtr **modemmanager.Modem) error {
	if *modemPtr == nil {
		testing.ContextLog(ctx, "Invalid modem pointer")
		return errors.New("invalid modem pointer")
	}
	modem := *modemPtr
	if !isL850Verizon(ctx, modem) {
		return nil
	}
	h, err := NewHelper(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create helper")
	}
	// Ignore errors. Do best effort to fix the modem
	if enabled, _ := h.Manager.IsEnabled(ctx, shill.TechnologyCellular); enabled {
		_, _ = h.Disable(ctx)
		defer h.Enable(ctx)
	}
	if err := modem.Enable(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to enable: ", err)
	}
	// Delete all bearers to ensure the bearer error code is from the next connection attempt.
	modem.DeleteAllBearers(ctx, modem)

	testing.ContextLog(ctx, "Check if modem needs to be restarted on L850/Verizon")
	// Use the modb APN, unless there is a modem profile.
	apn := map[string]interface{}{"apn": "vzwinternet", "ip-type": mmconst.BearerIPFamilyIPv4v6}
	// Get modem profiles
	modemProfiles, err := modem.GetProfilesAsBearerProperties(ctx)
	if err == nil {
		for _, profile := range modemProfiles {
			profileID, err := profile.GetProfileID(ctx)
			if err == nil {
				apn = map[string]interface{}{mmconst.BearerPropertyProfileID: profileID}
				break
			}
		}
	}
	if _, err := modem.Connect(ctx, apn); err == nil {
		if err := modem.DisconnectAll(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to disconnect: ", err)
		}
		// Ensure we remove the bearer object created during the previous step.
		modem.DeleteAllBearers(ctx, modem)
		// Connected successfully. Nothing else to do here
		return nil
	}

	testing.ContextLog(ctx, "Failed to connect: ", err)
	bearer, err := modem.GetFirstDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get bearer: ", err)
		return nil
	}
	connectionError, err := bearer.ConnectionError().ToString()
	if err != nil {
		testing.ContextLogf(ctx, "Failed to get bearer error: %s", err)
		return nil
	}

	// Ensure we remove the bearer object created during the previous step.
	modem.DeleteAllBearers(ctx, modem)

	if connectionError == "org.freedesktop.ModemManager1.Error.MobileEquipment.NotAllowed" {
		testing.ContextLog(ctx, "Modem cannot connect to vzwinternet(b/309953824). Reset modem")
		if *modemPtr, err = ResetModemWithHelper(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to restart modem: %s", err)
			return err
		}
	} else if connectionError != "" {
		testing.ContextLogf(ctx, "Last bearer connection error: %s", connectionError)
	}

	return nil
}

// WaitForEnabledState polls for the specified enable state for cellular.
func (h *Helper) WaitForEnabledState(ctx context.Context, expected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		enabled, err := h.Manager.IsEnabled(ctx, shill.TechnologyCellular)
		if err != nil {
			return errors.Wrap(err, "failed to get enabled state")
		}
		if enabled != expected {
			return errors.Errorf("unexpected enabled state, got %t, expected %t", enabled, expected)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  4 * defaultTimeout,
		Interval: 500 * time.Millisecond,
	})
}

// EnsureEnabled attempts to ensure that Cellular is enabled.
func (h *Helper) EnsureEnabled(ctx context.Context) error {
	// Ensure Cellular is enabled.
	enabled, err := h.Manager.IsEnabled(ctx, shill.TechnologyCellular)
	if err != nil {
		return errors.Wrap(err, "error requesting enabled state")
	}
	if !enabled {
		if _, err := h.Enable(ctx); err != nil {
			return errors.Wrap(err, "unable to enable Cellular")
		}
	}
	return nil
}

// EnsureDefaultService attempts to ensure that Cellular is enabled and that a
// connectable default Service is available. If no Service is initially available,
// the modem will be reset and a second check for a connectable service is performed.
func (h *Helper) EnsureDefaultService(ctx context.Context) (*shill.Service, error) {
	if err := h.EnsureEnabled(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to ensure enabled state")
	}

	// Ensure a Cellular Service is available.
	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		// If not available, try resetting the modem.
		testing.ContextLog(ctx, "Cellular Service not available, resetting modem")
		if _, err = h.ResetModem(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to reset modem waiting for default cellular service")
		}
		if err = h.WaitForEnabledState(ctx, true); err != nil {
			return nil, errors.Wrap(err, "cellular not enabled after modem reset while waiting for default cellular service")
		}
		service, err = h.FindServiceForDevice(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "unable to find a default cellular service")
		}
	}
	return service, nil
}

// Enable calls Manager.EnableTechnology(cellular) and returns true if the enable succeeded, or an error otherwise.
func (h *Helper) Enable(ctx context.Context) (time.Duration, error) {
	ctx, st := timing.Start(ctx, "Helper.Enable")
	defer st.End()

	start := time.Now()
	h.Manager.EnableTechnology(ctx, shill.TechnologyCellular)

	if err := h.WaitForEnabledState(ctx, true); err != nil {
		return 0, err
	}
	if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyPowered, true, defaultTimeout); err != nil {
		return 0, err
	}
	if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyScanning, false, defaultTimeout); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

// Disable calls Manager.DisableTechnology(cellular) and returns true if the disable succeeded, or an error otherwise.
func (h *Helper) Disable(ctx context.Context) (time.Duration, error) {
	ctx, st := timing.Start(ctx, "Helper.Disable")
	defer st.End()

	start := time.Now()
	h.Manager.DisableTechnology(ctx, shill.TechnologyCellular)

	if err := h.WaitForEnabledState(ctx, false); err != nil {
		return 0, err
	}
	if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyPowered, false, defaultTimeout); err != nil {
		return 0, err
	}
	if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyScanning, false, defaultTimeout); err != nil {
		return 0, errors.Wrap(err, "expected scanning to become false, got true")
	}
	return time.Since(start), nil
}

// FindService returns the first connectable Cellular Service.
// If no such Cellular Service is available, returns a nil service and an error.
func (h *Helper) FindService(ctx context.Context) (*shill.Service, error) {
	ctx, st := timing.Start(ctx, "Helper.FindService")
	defer st.End()

	// Look for any connectable Cellular service.
	cellularProperties := map[string]interface{}{
		shillconst.ServicePropertyConnectable: true,
		shillconst.ServicePropertyType:        shillconst.TypeCellular,
	}
	return h.Manager.WaitForServiceProperties(ctx, cellularProperties, defaultTimeout)
}

// FindServiceForDeviceWithProps returns the first connectable Cellular Service matching the Device ICCID and the given props.
// If no such Cellular Service is available, returns a nil service and an error.
func (h *Helper) FindServiceForDeviceWithProps(ctx context.Context, props map[string]interface{}) (*shill.Service, error) {
	ctx, st := timing.Start(ctx, "Helper.FindServiceForDeviceWithProps")
	defer st.End()

	deviceProperties, err := h.Device.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Cellular Device properties")
	}
	deviceICCID, err := deviceProperties.GetString(shillconst.DevicePropertyCellularICCID)
	if err != nil {
		return nil, errors.Wrap(err, "device missing ICCID")
	}
	if deviceICCID == "" {
		return nil, errors.Wrap(err, "device has empty ICCID")
	}

	necessaryProps := map[string]interface{}{
		shillconst.ServicePropertyCellularICCID: deviceICCID,
		shillconst.ServicePropertyConnectable:   true,
		shillconst.ServicePropertyType:          shillconst.TypeCellular,
	}
	for k, v := range necessaryProps {
		props[k] = v
	}

	service, err := h.Manager.WaitForServiceProperties(ctx, props, longTimeout)
	if err != nil {
		return nil, errors.Wrapf(err, "Service not found for: %+v", props)
	}
	return service, nil
}

// FindServiceForDevice is a convenience function to call FindServiceForDeviceWithProps with an empty set of extra props.
func (h *Helper) FindServiceForDevice(ctx context.Context) (*shill.Service, error) {
	ctx, st := timing.Start(ctx, "Helper.FindServiceForDevice")
	defer st.End()

	return h.FindServiceForDeviceWithProps(ctx, make(map[string]interface{}))
}

// FindPSimService returns the first connectable PSim service.
func (h *Helper) FindPSimService(ctx context.Context) (*shill.Service, error) {
	props := map[string]interface{}{
		shillconst.ServicePropertyCellularEID: "",
		shillconst.ServicePropertyConnectable: true,
		shillconst.ServicePropertyType:        shillconst.TypeCellular,
	}
	service, err := h.Manager.WaitForServiceProperties(ctx, props, defaultTimeout)
	if err != nil {
		return nil, errors.Wrap(err, "PSim service not found")
	}
	return service, nil
}

// AutoConnectCleanupTime provides enough time for a successful dbus operation.
// If a timeout occurs during cleanup, the operation will fail anyway.
const AutoConnectCleanupTime = 1 * time.Second

// SetServiceAutoConnect sets the AutoConnect property of the Cellular Service
// associated with the Cellular Device to |autoConnect| if the current value
// does not match |autoConnect|.
// Returns true when Service.AutoConnect is set and the operation succeeds.
// Returns an error if any operation fails.
func (h *Helper) SetServiceAutoConnect(ctx context.Context, autoConnect bool) (bool, error) {
	ctx, st := timing.Start(ctx, "Helper.SetServiceAutoConnect")
	defer st.End()

	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get Cellular Service")
	}
	properties, err := service.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "unable to get properties")
	}
	curAutoConnect, err := properties.GetBool(shillconst.ServicePropertyAutoConnect)
	if err != nil {
		return false, errors.Wrap(err, "unable to get AutoConnect")
	}
	if autoConnect == curAutoConnect {
		return false, nil
	}
	if err := service.SetProperty(ctx, shillconst.ServicePropertyAutoConnect, autoConnect); err != nil {
		return false, errors.Wrap(err, "failed to set Service.AutoConnect")
	}
	return true, nil
}

// WaitForModemRegisteredAfterReset polls the modem and verifies that the modem stays in that state for some time.
// This is done to ensure the modem is registered after shill has updated the initial EPS bearer settings.
func (h *Helper) WaitForModemRegisteredAfterReset(ctx context.Context, timeout time.Duration) error {
	const pollPeriod = 100 * time.Millisecond
	window := 5 * time.Second
	registeredTime := time.Time{}
	var lastValue bool
	statePrinted := false
	registeredLongEnough := func() (bool, error) {
		modem, err := modemmanager.NewModem(ctx)
		if err != nil {
			registeredTime = time.Time{}
			return false, errors.Wrap(err, "failed to get modem info")
		}
		isRegistered, err := modem.IsRegistered(ctx)
		if err != nil {
			registeredTime = time.Time{}
			return isRegistered, errors.Wrap(err, "failed to fetch registration state")
		}
		if !isRegistered {
			registeredTime = time.Time{}
			return isRegistered, errors.New("modem not registered")
		}
		if registeredTime.IsZero() {
			registeredTime = time.Now()
		}

		if time.Since(registeredTime) < window {
			return isRegistered, errors.New("modem not yet registered for enough time")
		}
		return isRegistered, nil
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		isRegistered, err := registeredLongEnough()
		if !statePrinted || (isRegistered != lastValue) {
			testing.ContextLogf(ctx, "Modem is registered: %t", isRegistered)
			lastValue = isRegistered
			statePrinted = true
		}
		return err

	}, &testing.PollOptions{
		Timeout:  timeout,
		Interval: pollPeriod,
	})
}

// ConnectToDefault connects to the default Cellular Service.
func (h *Helper) ConnectToDefault(ctx context.Context) (time.Duration, error) {
	ctx, st := timing.Start(ctx, "Helper.ConnectToDefault")
	defer st.End()

	start := time.Now()
	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return 0, err
	}
	if err := h.ConnectToService(ctx, service); err != nil {
		return 0, err
	}

	return time.Since(start), nil
}

// ConnectToServiceWithTimeout connects to a Cellular Service with a specified timeout.
// It ensures that the connect attempt succeeds, repeating attempts if necessary.
// Otherwise an error is returned.
func (h *Helper) ConnectToServiceWithTimeout(ctx context.Context, service *shill.Service, timeout time.Duration) error {
	ctx, st := timing.Start(ctx, "Helper.ConnectToServiceWithTimeout")
	defer st.End()

	props, err := service.GetProperties(ctx)
	if err != nil {
		return err
	}
	name, err := props.GetString(shillconst.ServicePropertyName)
	if err != nil {
		return err
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// check if we are already connected, since services may choose to return an error if Connect is attempted on an already connected service.
		isConnected, err := service.IsConnected(ctx)
		if err != nil {
			return err
		}
		if isConnected {
			return nil
		}
		if err := service.Connect(ctx); err != nil {
			return err
		}
		waitCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		if err := service.WaitForConnectedOrError(waitCtx); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  timeout,
		Interval: 1 * time.Second,
	}); err != nil {
		return errors.Wrapf(err, "connect to %s failed", name)
	}
	return nil
}

// ConnectToService connects to a Cellular Service.
// It ensures that the connect attempt succeeds, repeating attempts if necessary.
// Otherwise an error is returned.
func (h *Helper) ConnectToService(ctx context.Context, service *shill.Service) error {
	ctx, st := timing.Start(ctx, "Helper.ConnectToService")
	defer st.End()

	// Connect requires a longer default timeout than other operations.
	return h.ConnectToServiceWithTimeout(ctx, service, defaultTimeout*6)
}

// ConnectWithTimeout connects to default service if the current cellular service is not connected specifying a timeout value,
// otherwise return an error
func (h *Helper) ConnectWithTimeout(ctx context.Context, connectTimeout time.Duration) (*shill.Service, error) {
	ctx, st := timing.Start(ctx, "Helper.ConnectWithTimeout")
	defer st.End()

	contextWithTimeoutWrapper := func(ctx context.Context) (*shill.Service, error) {
		service, err := h.FindServiceForDevice(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "unable to determine the default cellular service")
		}

		name, err := service.GetName(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get the name of the default service")
		}

		if isConnected, err := service.IsConnected(ctx); err != nil {
			return nil, errors.Wrapf(err, "unable to get the connected state for %q", name)
		} else if !isConnected {
			if err := h.ConnectToServiceWithTimeout(ctx, service, connectTimeout); err != nil {
				return nil, errors.Wrapf(err, "unable to connect to the default service %q", name)
			}
		}

		// Wait for the service state to become connected and online.
		if err := service.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, connectTimeout); err != nil {
			return nil, errors.Wrapf(err, "default service %q connected but failed to become online", name)
		}
		return service, nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	service, err := contextWithTimeoutWrapper(connectCtx)
	if err != nil {
		select {
		case <-connectCtx.Done():
			// Add more details to the error when the failure is due to connectTimeout.
			return nil, errors.Wrapf(err, "timed out after %v while connecting", connectTimeout)
		default:
			return nil, err
		}

	}

	return service, nil

}

// Connect to default service if the current cellular service is not connected, otherwise return an error
func (h *Helper) Connect(ctx context.Context) (*shill.Service, error) {
	return h.ConnectWithTimeout(ctx, ConnectTimeout)
}

// Disconnect from the Cellular Service and ensure that the disconnect succeeded, otherwise return an error.
func (h *Helper) Disconnect(ctx context.Context) (time.Duration, error) {
	ctx, st := timing.Start(ctx, "Helper.Disconnect")
	defer st.End()

	start := time.Now()
	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return 0, err
	}
	if err := service.Disconnect(ctx); err != nil {
		return 0, err
	}
	if err := service.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateIdle, defaultTimeout); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

// IsConnected returns an error if the default Service is not connected.
func (h *Helper) IsConnected(ctx context.Context) error {
	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return err
	}
	if err := service.WaitForProperty(ctx, shillconst.ServicePropertyIsConnected, true, longTimeout); err != nil {
		return err
	}
	return nil
}

// SetDeviceProperty sets a Device property and waits for the property to be set.
func (h *Helper) SetDeviceProperty(ctx context.Context, prop string, value interface{}, timeout time.Duration) error {
	ctx, st := timing.Start(ctx, "Helper.SetDeviceProperty")
	defer st.End()

	pw, err := h.Device.CreateWatcher(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create watcher")
	}
	defer pw.Close(ctx)

	// If the Modem is starting, SetProperty may fail, so poll while that is the case.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := h.Device.SetProperty(ctx, prop, value); err != nil {
			if strings.Contains(err.Error(), shillconst.ErrorModemNotStarted) {
				return err
			}
			return testing.PollBreak(err)
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrapf(err, "unable to set Device property: %s", prop)
	}

	expectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := pw.Expect(expectCtx, prop, value); err != nil {
		return errors.Wrapf(err, "%s not set", prop)
	}
	return nil
}

// InitDeviceProperty sets a device property and returns a function to restore the initial value.
func (h *Helper) InitDeviceProperty(ctx context.Context, prop string, value interface{}) (func(ctx context.Context) error, error) {
	ctx, st := timing.Start(ctx, "Helper.InitDeviceProperty")
	defer st.End()

	return initProperty(ctx, h.Device.PropertyHolder, prop, value)
}

// InitServiceProperty sets a service property and returns a function to restore the initial value.
func (h *Helper) InitServiceProperty(ctx context.Context, prop string, value interface{}) (func(ctx context.Context) error, error) {
	ctx, st := timing.Start(ctx, "Helper.InitServiceProperty")
	defer st.End()

	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cellular service")
	}
	return initProperty(ctx, service.PropertyHolder, prop, value)
}

// PropertyCleanupTime provides enough time for a successful dbus operation at the end of the test.
const PropertyCleanupTime = 1 * time.Second

// initProperty sets a property and returns a function to restore the initial value.
func initProperty(ctx context.Context, properties *shill.PropertyHolder, prop string, value interface{}) (func(ctx context.Context) error, error) {
	ctx, st := timing.Start(ctx, "Helper.initProperty")
	defer st.End()

	prevValue, err := properties.GetAndSetProperty(ctx, prop, value)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read and initialize property")
	}

	return func(ctx context.Context) error {
		if err := properties.SetProperty(ctx, prop, prevValue); err != nil {
			return errors.Wrapf(err, "failed to restore %s", prop)
		}
		return nil
	}, nil

}

// resetShill overrides default profile based on given path and restarts shill.
func (h *Helper) resetShill(ctx context.Context, path string) []error {
	var errs []error
	if err := upstart.StopJob(ctx, shill.JobName); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to stop shill"))
	}
	if path == "" {
		if err := os.Remove(shillconst.DefaultProfilePath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, errors.Wrap(err, "failed to remove default profile"))
		}
	} else {
		// Write path content to default.profile, to sideload default.profile.
		if err := fsutil.CopyFile(path, shillconst.DefaultProfilePath); err != nil {
			return append(errs, errors.Wrap(err, "failed to copy default profile file"))
		}
	}
	// GoBigSleepLint: TODO(b/340051988): prevent shill from starting before the previous disable command is finished.
	testing.Sleep(ctx, 3*time.Second)
	if err := upstart.RestartJob(ctx, shill.JobName, GetShillUpstartArgsForVerboseLogging()...); err != nil {
		// No more can be done if shill doesn't start
		return append(errs, errors.Wrap(err, "failed to restart shill"))
	}
	manager, err := shill.NewManager(ctx)
	if err != nil {
		// No more can be done if a manager interface cannot be created
		return append(errs, errors.Wrap(err, "failed to create new shill manager"))
	}
	// Disable Wifi to avoid log pollution
	manager.DisableTechnology(ctx, shill.TechnologyWifi)

	if err = manager.PopAllUserProfiles(ctx); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to pop all user profiles"))
	}

	// Wait for 6s to allow cellular connection becomes stable. But it is NOT
	// guaranteed it succeeds every time. Currently, 5s without registered state
	// change is considered stable registration
	h.WaitForModemRegisteredAfterReset(ctx, 6*time.Second)

	return errs
}

// ResetShill removes default profile and restarts shill.
func (h *Helper) ResetShill(ctx context.Context) []error {
	return h.resetShill(ctx, "")
}

// ResetShillAndSetProfile replaces the default profile with path and restarts shill.
func (h *Helper) ResetShillAndSetProfile(ctx context.Context, path string) []error {
	return h.resetShill(ctx, path)
}

// ResetShillAndAddFakeUserProfile restarts shill, and adds a fake user profile.
func (h *Helper) ResetShillAndAddFakeUserProfile(ctx context.Context) (func(), []error) {
	if errs := h.resetShill(ctx, ""); errs != nil {
		return nil, errs
	}
	deferRemoveFakeUser, err := h.CreateFakeUserProfile(ctx)
	if err != nil {
		return nil, []error{errors.Wrap(err, "failed to create fake user profile")}
	}
	return deferRemoveFakeUser, nil

}

// CreateFakeUserProfile clears all shill profiles and adds a fake user profile in shill.
// The function returns a closure to delete the profile.
func (h *Helper) CreateFakeUserProfile(ctx context.Context) (func(), error) {
	const profileName = "cellularTest"
	testing.ContextLog(ctx, "Create fake user profile")
	if _, err := h.Manager.CreateFakeUserProfile(ctx, profileName); err != nil {
		return nil, errors.Wrapf(err, "failed to create fake profile %q", profileName)
	}
	return func() {
		if err := h.Manager.RemoveFakeUserProfile(ctx, profileName); err != nil {
			testing.ContextLogf(ctx, "Failed to remove profile %q: %s", profileName, err)
		}
	}, nil
}

// CaptureDBusLogs - Capture DBus system logs
// Return nil if DBus log collection succeeds, else return error.
func (h *Helper) CaptureDBusLogs(ctx context.Context) error {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get out dir")
	}

	f, err := os.Create(filepath.Join(outDir, "dbus-system.txt"))
	if err != nil {
		return err
	}

	cmd := testexec.CommandContext(ctx, "/usr/bin/dbus-monitor", "--system", "sender=org.chromium.flimflam", "destination=org.chromium.flimflam", "path_namespace=/org/freedesktop/ModemManager1")
	if f != nil {
		cmd.Stdout = f
		cmd.Stderr = f
	}

	if err := cmd.Start(); err != nil {
		f.Close()
		return errors.Wrap(err, "failed to start command")
	}

	go func() {
		defer cmd.Kill()
		defer cmd.Wait()
		defer f.Close()
		<-ctx.Done()
	}()

	return nil
}

// ResetModem calls Device.ResetModem(cellular) and if reset fails it calls ResetModemWithHelper
// returns true if the reset succeeded, or an error otherwise.
func (h *Helper) ResetModem(ctx context.Context) (time.Duration, error) {
	ctx, st := timing.Start(ctx, "Helper.ResetModem")
	defer st.End()

	start := time.Now()

	modemType, err := GetModemType(ctx)

	// Device.Reset is failing on NL668 modem (mainly Zork boards are affected)
	if err == nil && modemType != cellularconst.ModemTypeNL668 {
		if err = h.Device.Reset(ctx); err != nil {
			// Try reset using helper if Device.Reset does not work.
			testing.ContextLog(ctx, "Modem reset with Device.Reset failed. Trying reset using helper")
		} else {
			testing.ContextLog(ctx, "Modem reset with Device.Reset succeeded")
		}
	}

	if err != nil || modemType == cellularconst.ModemTypeNL668 {
		if _, err := ResetModemWithHelper(ctx); err != nil {
			return time.Since(start), errors.Wrap(err, "Modem reset with ResetModemWithHelper failed")
		}
		testing.ContextLog(ctx, "Modem reset with ResetModemWithHelper succeeded")
	}

	if err := h.WaitForEnabledState(ctx, true); err != nil {
		return time.Since(start), errors.Wrap(err, "expected enabled to become true")
	}
	if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyPowered, true, defaultTimeout); err != nil {
		return time.Since(start), errors.Wrap(err, "expected powered to become true, got false")
	}
	if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyScanning, false, defaultTimeout); err != nil {
		return time.Since(start), errors.Wrap(err, "expected scanning to become false, got true")
	}
	// GoBigSleepLint: Sleep added as reset fibocom modem taking time for modem to register
	// and service to refresh. Not found any state/property to wait.
	// TODO(b/216176362) : Reset modem causes pin api calls failure if not waited 30 seconds.
	if err := testing.Sleep(ctx, 30*time.Second); err != nil {
		return time.Since(start), errors.Wrap(err, "failed to sleep after reset modem")
	}
	return time.Since(start), nil
}

// IsSimLockEnabled returns lockenabled value.
func (h *Helper) IsSimLockEnabled(ctx context.Context) bool {
	lockStatus, err := h.GetCellularSIMLockStatus(ctx)
	if err != nil {
		testing.ContextLog(ctx, "getcellularsimlockstatus -lock: ", err.Error())
	}

	lock := false
	lockEnabled := lockStatus[shillconst.DevicePropertyCellularSIMLockStatusLockEnabled]
	if lockEnabled != nil {
		testing.ContextLog(ctx, "lock enabled status: ", lockEnabled)
		lock = lockEnabled.(bool)
	}
	return lock
}

// IsSimPinLocked returns true if locktype value is 'sim-pin'
// locktype value is 'sim-pin2' for QC and 'none' when not locked.
func (h *Helper) IsSimPinLocked(ctx context.Context) bool {
	lockStatus, err := h.GetCellularSIMLockStatus(ctx)
	if err != nil {
		testing.ContextLog(ctx, "getcellularsimlockstatus -pin: ", err.Error())
	}

	lockType := lockStatus[shillconst.DevicePropertyCellularSIMLockStatusLockType]
	lock := ""
	if lockType != nil {
		lock = lockType.(string)
		testing.ContextLog(ctx, "pin lock type value: ", lock)
	}

	return lock == shillconst.DevicePropertyValueSIMLockTypePIN
}

// IsSimNetPinLocked returns true if locktype value is 'network-pin'
func (h *Helper) IsSimNetPinLocked(ctx context.Context) bool {
	lockStatus, err := h.GetCellularSIMLockStatus(ctx)
	if err != nil {
		testing.ContextLog(ctx, "getcellularsimlockstatus -pin: ", err.Error())
		return false
	}

	lockType := lockStatus[shillconst.DevicePropertyCellularSIMLockStatusLockType]
	if lockType == nil {
		return false
	}

	lock, ok := lockType.(string)
	if !ok {
		return false
	}
	testing.ContextLog(ctx, "pin lock type value: ", lock)
	return lock == shillconst.DevicePropertyValueSIMLockTypeNetPIN
}

// IsSimPukLocked returns true if locktype value is 'sim-puk'
// locktype value is 'sim-pin2' for QC and value 'none' when not locked.
func (h *Helper) IsSimPukLocked(ctx context.Context) bool {
	lockStatus, err := h.GetCellularSIMLockStatus(ctx)
	if err != nil {
		testing.ContextLog(ctx, "getcellularsimlockstatus -puk: ", err.Error())
	}

	lockType := lockStatus[shillconst.DevicePropertyCellularSIMLockStatusLockType]
	lock := ""
	if lockType != nil {
		lock = lockType.(string)
		testing.ContextLog(ctx, "puk locktype: ", lock)
	}

	return lock == shillconst.DevicePropertyValueSIMLockTypePUK
}

// GetRetriesLeft helps to get modem property UnlockRetries value.
func (h *Helper) GetRetriesLeft(ctx context.Context) (int32, error) {
	lockStatus, err := h.GetCellularSIMLockStatus(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "getcellularsimlockstatus failed reading retriesleft")
	}

	retriesLeft := lockStatus[shillconst.DevicePropertyCellularSIMLockStatusRetriesLeft]

	if retriesLeft == nil {
		return 0, errors.New("failed to get retriesLeft")
	}
	retries, ok := retriesLeft.(int32)
	if !ok {
		return 0, errors.New("int32 type assert failed for retriesleft property")
	}
	if retries < 0 {
		return 0, errors.New("negative retriesleft property")
	}
	testing.ContextLog(ctx, "retriesleft value : ", retries)

	return retries, nil
}

// In the case of [service].Error.PinError, the error message gives details:
// [interface].PinRequired
// [interface].PinBlocked
// [interface].IncorrectPin

// UnlockDut is to unlock sim pin before every test.
func (h *Helper) UnlockDut(ctx context.Context, currentPin, currentPuk string) error {
	// Check if pin enabled and locked/set.
	if h.IsSimLockEnabled(ctx) || h.IsSimPinLocked(ctx) {
		// Disable pin.
		if err := h.Device.RequirePin(ctx, currentPin, false); err != nil {
			return errors.Wrap(err, "failed to disable lock")
		}
	}

	return nil
}

// ClearSIMLock clears puk, pin lock if any of them enabled and locked.
func (h *Helper) ClearSIMLock(ctx context.Context, pin, puk string) error {

	if !h.IsSimLockEnabled(ctx) && !h.IsSimPukLocked(ctx) {
		return nil
	}

	// Clear puk lock if puk locked which is unusual.
	if h.IsSimPukLocked(ctx) {
		if len(puk) == 0 {
			return errors.New("ClearSIMLock needs PUK code to unlock SIM")
		}
		retriesLeft, err := h.GetRetriesLeft(ctx)
		if err != nil {
			if retriesLeft == 1 {
				return errors.New("Provided PUK code seems invalid. " +
					"SIM is PUK locked with only one chance to unlock!")
			}
			if retriesLeft == 0 {
				return errors.New("SIM card blocked and need to be replaced")
			}
		}
		if err := h.Device.UnblockPin(ctx, puk, pin); err != nil {
			return errors.Wrap(err, "failed to UnblockPin")
		}
	}

	// Clear pin lock, this can also happen after puk unlocked.
	if h.IsSimPinLocked(ctx) {
		if err := h.Device.EnterPin(ctx, pin); err != nil {
			testing.ContextLog(ctx, "At EnterPin in ClearSIMLock: ", err)

			// Block SIM with PUK lock and set new PIN code
			if err := h.PukLockSim(ctx, pin); err != nil {
				return errors.Wrap(err, "failed to lock SIM card with PukLockSim")
			}
			if err := h.Device.UnblockPin(ctx, puk, pin); err != nil {
				return errors.Wrap(err, "failed to clear PUK lock with UnblockPin")
			}
			if err := h.Device.EnterPin(ctx, pin); err != nil {
				return errors.Wrap(err, "failed to unlock SIM card with EnterPin")
			}
		}
	}

	// Disable sim lock.
	if err := h.Device.RequirePin(ctx, pin, false); err != nil {
		return errors.Wrap(err, "failed to clear pin lock with RequirePin")
	}

	testing.ContextLog(ctx, "clearsimlock disabled locks with pin: ", pin)

	return nil
}

// GetCellularSIMLockStatus dict gets Cellular.SIMLockStatus dictionary from shill properties.
func (h *Helper) GetCellularSIMLockStatus(ctx context.Context) (map[string]interface{}, error) {
	if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyScanning, false, defaultTimeout); err != nil {
		return nil, errors.Wrap(err, "expected scanning to become false, got true")
	}
	// Gather Shill Device properties.
	deviceProps, err := h.Device.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device properties")
	}

	// Verify Device.SimSlots.
	info, err := deviceProps.Get(shillconst.DevicePropertyCellularSIMLockStatus)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device cellularsimlockstatus property")
	}

	simLockStatus, ok := info.(map[string]interface{})
	if !ok {
		return nil, errors.Wrap(err, "invalid format for device cellularsimlockstatus")
	}

	testing.ContextLog(ctx, "simlockstatus: ", simLockStatus)

	return simLockStatus, nil
}

// Helper functions for SIM lock/unlock.

// random generates a random integer in given range.
func random(min, max int) int {
	return rand.Intn(max-min) + min
}

// BadPin obtains a pin that does not match the valid sim-pin.
func (h *Helper) BadPin(ctx context.Context, currentPin string) (string, error) {
	randomPin := random(1000, 9999)
	pin, _ := strconv.Atoi(currentPin)
	if randomPin == pin {
		randomPin++
	}

	return strconv.Itoa(randomPin), nil
}

// BadPuk obtains a puk that does not match the valid sim-puk.
func (h *Helper) BadPuk(ctx context.Context, currentPuk string) (string, error) {
	randomPuk := random(10000000, 99999999)
	puk, _ := strconv.Atoi(currentPuk)
	if randomPuk == puk {
		randomPuk++
	}

	return strconv.Itoa(randomPuk), nil
}

// PinLockSim is a helper method to pin-lock a sim, assuming nothing bad happens.
func (h *Helper) PinLockSim(ctx context.Context, newPin string) error {
	if h.IsSimPinLocked(ctx) {
		return nil
	}
	if err := h.Device.RequirePin(ctx, newPin, true); err != nil {
		return errors.Wrap(err, "failed to enable lock with new pin")
	}

	return nil
}

// PukLockSim is a helper method to puk-lock a SIM, assuming nothing bad happens.
func (h *Helper) PukLockSim(ctx context.Context, currentPin string) error {
	if err := h.PinLockSim(ctx, currentPin); err != nil {
		return errors.Wrap(err, "failed at pinlocksim")
	}

	// Reset modem to reflect if puk locked.
	if _, err := h.ResetModem(ctx); err != nil {
		return errors.Wrap(err, "reset modem failed after pin lock set")
	}

	locked := h.IsSimPinLocked(ctx)
	if locked {
		testing.ContextLog(ctx, "pinlocked with: ", currentPin)
	}
	locked = false
	retriesCnt := 0
	// Max incorrect retries before SIM card becomes PUK locked is usually 3
	for retriesCnt < 10 && !locked {
		retriesCnt++
		if err := h.EnterIncorrectPin(ctx, currentPin); err != nil {
			testing.ContextLog(ctx, "Failed to enter incorrect pin: ", err.Error())
			continue
		}
		if err := h.Device.WaitForProperty(ctx, shillconst.DevicePropertyScanning,
			false, defaultTimeout); err != nil {
			return errors.Wrap(err, "expected scanning to become false, got true")
		}
		locked = h.IsSimPukLocked(ctx)
	}

	if !locked {
		return errors.New("expected sim to be puk-locked")
	}

	return nil
}

// EnterIncorrectPin gets incorrect pin and tries to unlock.
func (h *Helper) EnterIncorrectPin(ctx context.Context, currentPin string) error {
	badPin, err := h.BadPin(ctx, currentPin)
	testing.ContextLog(ctx, "Created badpin is: ", badPin)
	if err != nil {
		return errors.Wrap(err, "failed to generate bad pin")
	}

	if err = h.Device.EnterPin(ctx, badPin); err == nil {
		return errors.Wrap(err, "random PIN was accepted by SIM card: "+badPin)
	}

	// For flimflam errors with org.chromium.flimflam.Error.IncorrectPin.
	if strings.Contains(err.Error(), shillconst.ErrorIncorrectPin) ||
		strings.Contains(err.Error(), shillconst.ErrorIncorrectPassword) ||
		strings.Contains(err.Error(), shillconst.ErrorPukRequired) ||
		strings.Contains(err.Error(), shillconst.ErrorPinBlocked) {
		return nil
	}

	// If puk locked with unknown error, ignoring error as intention is to do puk lock.
	locked := h.IsSimPukLocked(ctx)
	if locked {
		return nil
	}
	return errors.Wrap(err, "unusual pin error")
}

// EnterIncorrectPuk generates bad puk and tries to unlock.
func (h *Helper) EnterIncorrectPuk(ctx context.Context, currentPin, currentPuk string) error {
	badPuk, err := h.BadPuk(ctx, currentPuk)
	if err != nil {
		return errors.Wrap(err, "failed to generate bad puk")
	}

	if err = h.Device.UnblockPin(ctx, badPuk, currentPin); err == nil {
		return errors.Wrap(err, "failed to send bad puk: "+badPuk)
	}

	errorIncorrectPuk := errors.New("org.freedesktop.ModemManager1.Sim.Error.IncorrectPuk")
	if errors.Is(err, errorIncorrectPuk) {
		return nil
	}

	return errors.Wrap(err, "unusual puk error")
}

// getCellularDeviceDictProperty gets a shill device dictionary property
func (h *Helper) getCellularDeviceDictProperty(ctx context.Context, propertyName string) (map[string]string, error) {
	props, err := h.Device.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Device properties")
	}
	info, err := props.Get(propertyName)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting property %q", propertyName)
	}

	dictProp, ok := info.(map[string]string)
	if !ok {
		return nil, errors.Wrapf(err, "invalid format for %q", propertyName)
	}

	return dictProp, nil
}

// GetHomeProviderFromShill returns current home provider UUID and code from shill.
func (h *Helper) GetHomeProviderFromShill(ctx context.Context) (uuid, code string, err error) {
	ctx, st := timing.Start(ctx, "Helper.GetHomeProviderFromShill")
	defer st.End()
	homeProviderMap, err := h.getCellularDeviceDictProperty(ctx, shillconst.DevicePropertyCellularHomeProvider)
	if err != nil {
		return "", "", errors.New("invalid format for Home Provider property")
	}
	uuid, ok := homeProviderMap[shillconst.OperatorUUIDKey]
	if !ok {
		return "", "", errors.New("home provider UUID not found")
	}
	code, ok = homeProviderMap[shillconst.OperatorCode]
	if !ok {
		return "", "", errors.New("home provider operator code not found")
	}

	return uuid, code, nil
}

// GetServingOperatorFromShill returns current serving operator UUID and code from shill.
func (h *Helper) GetServingOperatorFromShill(ctx context.Context) (uuid, code string, err error) {
	ctx, st := timing.Start(ctx, "Helper.GetServingOperatorFromShill")
	defer st.End()
	servingOperatorMap, err := h.getCellularServiceDictProperty(ctx, shillconst.ServicePropertyCellularServingOperator)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get last good APN info")
	}
	uuid, ok := servingOperatorMap[shillconst.OperatorUUIDKey]
	if !ok {
		return "", "", errors.New("serving operator UUID not found")
	}
	code, ok = servingOperatorMap[shillconst.OperatorCode]
	if !ok {
		return "", "", errors.New("service operator code not found")
	}
	return uuid, code, nil
}

// getCellularDeviceProperty gets a shill device string property
func (h *Helper) getCellularDeviceProperty(ctx context.Context, propertyName string) (string, error) {
	props, err := h.Device.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get Device properties")
	}
	info, err := props.GetString(propertyName)
	if err != nil {
		return "", errors.Wrapf(err, "error getting property %q", propertyName)
	}

	return info, nil
}

// GetFirmwareRevisionFromShill gets the current modem firmware revision from shill.
func (h *Helper) GetFirmwareRevisionFromShill(ctx context.Context) (string, error) {
	return h.getCellularDeviceProperty(ctx, shillconst.DevicePropertyCellularFirmwareRevision)
}

// GetIMEIFromShill gets the current modem IMEI from shill.
func (h *Helper) GetIMEIFromShill(ctx context.Context) (string, error) {
	return h.getCellularDeviceProperty(ctx, shillconst.DevicePropertyCellularIMEI)
}

// GetIMSIFromShill gets the current modem IMSI from shill.
func (h *Helper) GetIMSIFromShill(ctx context.Context) (string, error) {
	return h.getCellularDeviceProperty(ctx, shillconst.DevicePropertyCellularIMSI)
}

// GetProviderRequiresRoamingFromShill gets current ProviderRequiresRoaming boolean from shill.
func (h *Helper) GetProviderRequiresRoamingFromShill(ctx context.Context) (bool, error) {
	props, err := h.Device.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get Device properties")
	}
	providerRequiresRoaming, err := props.GetBool(shillconst.DevicePropertyCellularProviderRequiresRoaming)
	if err != nil {
		return false, errors.Wrapf(err, "error getting property %q", shillconst.DevicePropertyCellularProviderRequiresRoaming)
	}

	return providerRequiresRoaming, nil
}

// SetServiceProvidersExclusiveOverride adds an override MODB to shill.
// The function returns a closure to delete the override file.
func SetServiceProvidersExclusiveOverride(ctx context.Context, sourceFile string) (func(), error) {
	input, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %q", sourceFile)
	}
	err = os.WriteFile(shillconst.ServiceProviderOverridePath, input, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to write %q", shillconst.ServiceProviderOverridePath)
	}
	return func() {
		os.Remove(shillconst.ServiceProviderOverridePath)
	}, nil
}

// getCellularServiceDictProperty gets a shill service dictionary property
func (h *Helper) getCellularServiceDictProperty(ctx context.Context, propertyName string) (map[string]string, error) {
	// Verify that a connectable Cellular service exists and ensure it is connected.
	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find Cellular Service for Device")
	}
	props, err := service.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error getting Service properties")
	}
	info, err := props.Get(propertyName)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting property %q", propertyName)
	}

	dictProp, ok := info.(map[string]string)
	if !ok {
		return nil, errors.Wrapf(err, "invalid format for %q", propertyName)
	}

	return dictProp, nil
}

// getCellularServiceProperty gets a shill service dictionary property
func (h *Helper) getCellularServiceProperty(ctx context.Context, propertyName string) (string, error) {
	// Verify that a connectable Cellular service exists and ensure it is connected.
	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return "", errors.Wrap(err, "unable to find Cellular Service for Device")
	}
	props, err := service.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrap(err, "error getting Service properties")
	}
	info, err := props.GetString(propertyName)
	if err != nil {
		return "", errors.Wrapf(err, "error getting property %q", propertyName)
	}

	return info, nil
}

// SetAPN sets the Custom/OTHER APN property `Cellular.APN`.
func (h *Helper) SetAPN(ctx context.Context, apn map[string]string) error {
	ctx, st := timing.Start(ctx, "Helper.SetApn")
	defer st.End()

	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Cellular Service")
	}
	if err := service.SetProperty(ctx, shillconst.ServicePropertyCellularAPN, apn); err != nil {
		return errors.Wrap(err, "failed to set Cellular.APN")
	}
	return nil
}

// SetCustomAPNList sets the new APN revamp UI property `Cellular.CustomAPNList`.
func (h *Helper) SetCustomAPNList(ctx context.Context, apns []map[string]string) error {
	ctx, st := timing.Start(ctx, "Helper.SetCustomAPNList")
	defer st.End()

	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Cellular Service")
	}
	if err := service.SetProperty(ctx, shillconst.ServicePropertyCellularCustomAPNList, dbus.MakeVariant(apns)); err != nil {
		return errors.Wrap(err, "failed to set Cellular.CustomAPNList")
	}
	return nil
}

// ClearCustomAPNList clears the new APN revamp UI property `Cellular.CustomAPNList`.
func (h *Helper) ClearCustomAPNList(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "Helper.ClearCustomAPNList")
	defer st.End()

	service, err := h.FindServiceForDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Cellular Service")
	}
	if err := service.ClearProperty(ctx, shillconst.ServicePropertyCellularCustomAPNList); err != nil {
		return errors.Wrap(err, "failed to clear Cellular.CustomAPNList")
	}
	return nil
}

// GetCellularLastAttachAPN gets Cellular.LastAttachAPN dictionary from shill properties.
func (h *Helper) GetCellularLastAttachAPN(ctx context.Context) (map[string]string, error) {
	return h.getCellularServiceDictProperty(ctx, shillconst.ServicePropertyCellularLastAttachAPN)
}

// GetCellularLastGoodAPN gets Cellular.LastAttachAPN dictionary from shill properties.
func (h *Helper) GetCellularLastGoodAPN(ctx context.Context) (map[string]string, error) {
	return h.getCellularServiceDictProperty(ctx, shillconst.ServicePropertyCellularLastGoodAPN)
}

// GetCurrentIPType returns current ip_type of LastGoodAPN.
func (h *Helper) GetCurrentIPType(ctx context.Context) (string, error) {
	apnInfo, err := h.getCellularServiceDictProperty(ctx, shillconst.ServicePropertyCellularLastGoodAPN)
	if err != nil {
		return "", errors.Wrap(err, "failed to get last good APN info")
	}

	ipType := apnInfo["ip_type"]
	if len(ipType) > 0 {
		return ipType, nil
	}
	return "ipv4", nil
}

// GetNetworkProvisionedCellularIPTypes returns the currently provisioned IP types
func (h *Helper) GetNetworkProvisionedCellularIPTypes(ctx context.Context) (ipv4, ipv6 bool, err error) {
	// Verify that a connectable Cellular service exists and ensure it is connected.
	service, err := h.Connect(ctx)
	if err != nil {
		return false, false, errors.Wrap(err, "unable to find Cellular Service")
	}

	configs, err := service.GetIPConfigs(ctx)
	if err != nil {
		return false, false, errors.Wrap(err, "failed to get IPConfigs from service")
	}

	ipv4Present := false
	ipv6Present := false
	for _, config := range configs {
		props, err := config.GetIPProperties(ctx)
		if err != nil {
			return false, false, errors.Wrap(err, "failed to get IPConfig properties")
		}
		testing.ContextLog(ctx, "Address :", props.Address)
		ip := net.ParseIP(props.Address)
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			testing.ContextLog(ctx, "IPv4 Address :", props.Address, " len(ip): ", len(ip))
			ipv4Present = true
		} else {
			testing.ContextLog(ctx, "IPv6 Address :", props.Address, " len(ip): ", len(ip))
			ipv6Present = true
		}
	}
	if !ipv4Present && !ipv6Present {
		return false, false, errors.New("no IP networks provisioned")
	}
	return ipv4Present, ipv6Present, nil
}

// GetCurrentEID gets current EID
func (h *Helper) GetCurrentEID(ctx context.Context) (string, error) {
	return h.getCellularServiceProperty(ctx, shillconst.ServicePropertyCellularEID)
}

// GetCurrentICCID gets current ICCID
func (h *Helper) GetCurrentICCID(ctx context.Context) (string, error) {
	return h.getCellularServiceProperty(ctx, shillconst.ServicePropertyCellularICCID)
}

// GetCurrentNetworkName gets current Network name
func (h *Helper) GetCurrentNetworkName(ctx context.Context) (string, error) {
	return h.getCellularServiceProperty(ctx, shillconst.ServicePropertyName)
}

// GetCurrentNetworkTechnology gets current NetworkTechnology
func (h *Helper) GetCurrentNetworkTechnology(ctx context.Context) (string, error) {
	return h.getCellularServiceProperty(ctx, shillconst.ServicePropertyCellularNetworkTechnology)
}

// GetCurrentRoamingState gets current RoamingState
func (h *Helper) GetCurrentRoamingState(ctx context.Context) (string, error) {
	return h.getCellularServiceProperty(ctx, shillconst.ServicePropertyCellularRoamingState)
}

// GetCurrentActivationState gets current ActivationState
func (h *Helper) GetCurrentActivationState(ctx context.Context) (string, error) {
	return h.getCellularServiceProperty(ctx, shillconst.ServicePropertyCellularActivationState)
}

// disableNonCellularInterfaceforTesting disable all non cellular interfaces
func (h *Helper) disableNonCellularInterfaceforTesting(ctx context.Context) error {
	ctx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime*2)
	defer cancel()
	// Disable Ethernet and/or WiFi if present and defer re-enabling.
	if enableFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet); err != nil {
		return errors.Wrap(err, "unable to disable Ethernet")
	} else if enableFunc != nil {
		h.enableEthernetFunc = enableFunc
	}
	if enableFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyWifi); err != nil {
		return errors.Wrap(err, "unable to disable Wifi")
	} else if enableFunc != nil {
		h.enableWifiFunc = enableFunc
	}
	return nil
}

// enablePreviouslyDisabledNonCellularInterfaceforTesting enable previously disabled interfaces
func (h *Helper) enablePreviouslyDisabledNonCellularInterfaceforTesting(ctx context.Context) {
	if h.enableEthernetFunc != nil {
		h.enableEthernetFunc(ctx)
	}
	if h.enableWifiFunc != nil {
		h.enableWifiFunc(ctx)
	}
	h.enableEthernetFunc = nil
	h.enableWifiFunc = nil
}

// RunTestOnCellularInterface setup the device for cellular tests.
func (h *Helper) RunTestOnCellularInterface(ctx context.Context, testBody func(ctx context.Context) error) error {
	if _, err := h.Connect(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to cellular service")
	}

	if err := h.disableNonCellularInterfaceforTesting(ctx); err != nil {
		return errors.Wrap(err, "failed to disable non cellular interface")
	}
	defer h.enablePreviouslyDisabledNonCellularInterfaceforTesting(ctx)

	if err := testBody(ctx); err != nil {
		LogL3Info(ctx)
		return err
	}

	return nil
}

// PrintSIMInfo prints SIM details
func (h *Helper) PrintSIMInfo(ctx context.Context) {
	if h.di != nil {
		h.PrintSIMInfo(ctx)
	}
}

// GetPINAndPUKForICCID returns the pin and puk info for the given iccid from host_info_label
func (h *Helper) GetPINAndPUKForICCID(ctx context.Context, iccid string) (string, string, error) {
	for _, s := range h.di.SimInfo {
		for _, p := range s.ProfileInfo {
			if p.Iccid == iccid {
				return p.SimPin, p.SimPuk, nil
			}
		}
	}
	return "", "", errors.Errorf("labels for ICCID %q not found", iccid)
}

// GetCarrierNameForICCID returns carrier name for the given iccid from host_info_label
func (h *Helper) GetCarrierNameForICCID(ctx context.Context, iccid string) (string, error) {
	for _, s := range h.di.SimInfo {
		for _, p := range s.ProfileInfo {
			if p.Iccid == iccid {
				return p.CarrierName.String(), nil
			}
		}
	}
	return "", errors.Errorf("carrier name for ICCID %q not found", iccid)
}

// SetDUTInfo sets the host-specific dut info for the helper.
func (h *Helper) SetDUTInfo(di *cellular.DUTInfo) {
	h.di = di
}

// GetLabelCarrierName return the current carrier name
func (h *Helper) GetLabelCarrierName(ctx context.Context) string {
	return h.di.CarrierName
}

// GetLabelStarfishSlotMapping return the starfish slot mapping label
func (h *Helper) GetLabelStarfishSlotMapping(ctx context.Context) string {
	return h.di.StarfishSlotMapping
}

// GetLabelOwnNumber return the current own number.
func (h *Helper) GetLabelOwnNumber(ctx context.Context, iccid string) string {
	if h.di == nil {
		testing.ContextLog(ctx, " DUT info is nil. Did you make sure SetDUTInfo was called?")
		return ""
	}
	for _, s := range h.di.SimInfo {
		for _, p := range s.ProfileInfo {
			if p.Iccid == iccid {
				return p.OwnNumber
			}
		}
	}
	return ""
}

// ConnectAndCheckSignalQuality verifies that cellular is connected and has sufficient signal coverage to run test cases
func (h *Helper) ConnectAndCheckSignalQuality(ctx context.Context) error {
	service, err := h.Connect(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to connect to cellular service")
	}
	// Poll for 65 seconds to find service's signal quality and scaled quality property update.
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		signalStrength, err := service.GetSignalStrength(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to get service SignalStrength")
		}
		testing.ContextLog(ctx, "SignalStrength: ", signalStrength)
		if signalStrength < shillconst.CellularServiceMinSignalStrength {
			return errors.Wrapf(err, "signal strength below minimum acceptable threshold - %d < %d ", signalStrength, shillconst.CellularServiceMinSignalStrength)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  65 * time.Second,
		Interval: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed SignalStrength check")
	}
	return nil
}

// GetProfileNickNameForIccid returns the profile nickname for the given cellular profile |iccid|
func GetProfileNickNameForIccid(ctx context.Context, iccid string) (string, error) {
	euicc, _, err := hermes.GetEUICC(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "could not get Hermes euicc")
	}

	profiles, err := euicc.InstalledProfiles(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "could not get Hermes installed profiles")
	}

	if len(profiles) == 0 {
		return "", errors.Wrap(err, "there are no installed profiles")
	}

	for _, profile := range profiles {
		props, _ := dbusutil.NewDBusProperties(ctx, profile.DBusObject)

		currentIccid, err := props.GetString(hermesconst.ProfilePropertyIccid)
		if err != nil {
			return "", errors.Wrapf(err, "failed to read iccid of profile: %s", profile.String())
		}

		if iccid == currentIccid {
			nickname, err := props.GetString(hermesconst.ProfilePropertyNickname)
			if err != nil {
				return "", errors.Wrapf(err, "failed to read nickname of profile: %s", profile.String())
			}
			return nickname, nil
		}
	}
	return "", errors.Wrapf(err, "no matched eSIM profile for iccid: %s", iccid)
}

// CreateCarrierLockCsvFile is a helper function to build CSV files to lock and unlock device in SimLock portal.
// Returns CSV file name
func (h *Helper) CreateCarrierLockCsvFile(ctx context.Context, profile string) (string, error) {
	imei, err := h.GetIMEIFromShill(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read EquipmentIdentifier")
	}
	bserial, err := exec.Command("vpd_get_value", "serial_number").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to read serial number")
	}
	serial := string(bserial)

	bmodel, err := exec.Command("vpd_get_value", "model_name").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to read model")
	}
	model := string(bmodel)

	bmanufacturer, err := exec.Command("cat", "/run/chromeos-config/v1/branding/oem-name").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to read manufacturer")
	}
	manufacturer := string(bmanufacturer)

	var conf []byte
	csvFileName := "carrier_lock_" + imei + ".csv"

	if profile == "0" {
		// Build the unlock csv file
		conf = []byte(simLockUnLockCsvHeader + serial + "," + serial + ",SIM_LOCK," + profile + ",0,0," + model + "\n")
	} else {
		// Build the lock csv file
		conf = []byte(simLockCsvHeader + serial + "," + serial + ",SIM_LOCK," + profile + ",0," + model + "," + manufacturer + "\n")
	}

	err = os.WriteFile(userMyFilesPath+csvFileName, conf, 0644)
	if err != nil {
		return "", errors.Wrap(err, "failed to create carrier lock CSV file")
	}

	return csvFileName, nil
}

// CreateAndUploadCarrierLockCsv is a helper API to build and upload the CSV file to the
// simlock portal.
func (h *Helper) CreateAndUploadCarrierLockCsv(ctx context.Context, gaiaCreds chrome.Creds, profile string) error {
	uiHelper, err := NewUIHelperWithFeatures(ctx, gaiaCreds.User, gaiaCreds.Pass, "CellularCarrierLock")
	if err != nil {
		return errors.Wrap(err, "failed to create cellular.NewUiHelper")
	}

	// Create the csv file
	csvfile, err := h.CreateCarrierLockCsvFile(ctx, profile)
	if err != nil {
		return errors.Wrap(err, "failed to create CSV file")
	}

	defer os.Remove(userMyFilesPath + csvfile)

	// Login to SimLock portal using owned test accounts and upload the CSV file
	err = uiHelper.UploadCsvSimLockPortal(ctx, csvfile)
	if err != nil {
		return errors.Wrap(err, "failed to upload unlock csv file")
	}
	return nil
}

// CarrierUnlockDevice is a helper API to unlock the device in SimLock portal
func (h *Helper) CarrierUnlockDevice(ctx context.Context, gaiaCreds chrome.Creds) error {
	testing.ContextLog(ctx, "Unlocking device in SimLock portal")
	err := h.CreateAndUploadCarrierLockCsv(ctx, gaiaCreds, SimLockUnlockProfileID)
	if err != nil {
		return errors.Wrap(err, "failed to create and upload unlock csv")
	}
	return nil
}

// ApplyCarrierLockConfig is a helper API to read signed carrier lock configuration
// from specified file and inject it to the modem manager. This is used for testing
// using pre-generated signed carrier lock configs, primarily negative configs.
func (h *Helper) ApplyCarrierLockConfig(ctx context.Context,
	modem *modemmanager.Modem, filepath string) error {
	b, err := os.ReadFile(filepath)
	if err != nil {
		return errors.Wrap(err, "failed to read carrier lock config file "+filepath)
	}
	config := string(b)
	decoded, _ := hex.DecodeString(config)
	if err := modem.ApplyCarrierLockConfig(ctx, decoded); err != nil {
		return errors.Wrap(err, "failed to apply carrier lock config "+filepath)
	}
	return nil
}

// GetKnownApns gets a list of the known APNs for the the operator of the current SIM.
func GetKnownApns(ctx context.Context) ([]KnownAPN, error) {
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new modem with SIM")
	}
	operatorID, gid1, err := modem.GetOperatorAndGid1Identifiers(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get the Operator and GID1 Identifiers")
	}
	testing.ContextLog(ctx, "operatorID: ", operatorID, " GID1: ", gid1)

	knownAPNs, err := GetKnownAPNsForOperator(operatorID, gid1)
	if err != nil {
		return nil, errors.Errorf("there are no APNs for operator %q", operatorID)
	}
	testing.ContextLog(ctx, "knownAPNs: ", knownAPNs)
	return knownAPNs, nil
}

// WaitForCarrierLock polls for sim lock type to change to network-pin or none.
func (h *Helper) WaitForCarrierLock(ctx context.Context, expected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		netPinLocked := h.IsSimNetPinLocked(ctx)
		if netPinLocked != expected {
			return errors.Errorf("unexpected carrier lock state, got %t, expected %t", netPinLocked, expected)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  4 * defaultTimeout,
		Interval: 500 * time.Millisecond,
	})
}
