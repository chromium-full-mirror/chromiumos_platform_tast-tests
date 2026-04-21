// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package modemmanager

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Modem wraps a Modemmanager.Modem D-Bus object.
type Modem struct {
	*dbusutil.PropertyHolder
}

// NewModemFromPath creates a new PropertyHolder instance for the Modem object at the
// given D-Bus object path.
func NewModemFromPath(ctx context.Context, path dbus.ObjectPath) (*Modem, error) {
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerModemInterface, path)
	if err != nil {
		return nil, err
	}
	return &Modem{ph}, nil
}

// NewModem creates a new PropertyHolder instance for the Modem object.
func NewModem(ctx context.Context) (*Modem, error) {
	_, obj, err := dbusutil.ConnectNoTiming(ctx, DBusModemmanagerService, DBusModemmanagerPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to service %s", DBusModemmanagerService)
	}

	// It may take 30+ seconds for a Modem object to appear after an Inhibit or
	// a reset, so poll the managed objects for 60 seconds looking for a Modem.
	modemPath := dbus.ObjectPath("")
	if err := testing.Poll(ctx, func(ctx context.Context) (e error) {
		managed, err := dbusutil.ManagedObjects(ctx, obj)
		if err != nil {
			return errors.Wrap(err, "failed to get ManagedObjects")
		}
		for iface, paths := range managed {
			if iface == DBusModemmanagerModemInterface {
				if len(paths) > 0 {
					modemPath = paths[0]
				}
				break
			}
		}
		if modemPath == dbus.ObjectPath("") {
			return errors.Wrap(err, "failed to get Modem path")
		}
		return nil // success
	}, &testing.PollOptions{Timeout: 60 * time.Second}); err != nil {
		return nil, err
	}
	return NewModemFromPath(ctx, modemPath)
}

// getSimpleModem creates a PropertyHolder for the SimpleModem object.
func (m *Modem) getSimpleModem(ctx context.Context) (*Modem, error) {
	modemPath := dbus.ObjectPath(m.String())
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerSimpleModemInterface, modemPath)
	if err != nil {
		return nil, err
	}
	return &Modem{ph}, nil
}

// IsSAREnabled - checks if SAR is enabled
func (m *Modem) IsSAREnabled(ctx context.Context) (bool, error) {
	modemPath := dbus.ObjectPath(m.String())
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerSARInterface, modemPath)
	if err != nil {
		return false, err
	}
	sarProps, err := ph.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to read SAR properties")
	}

	sarState, err := sarProps.GetBool(mmconst.ModemSARState)
	if err != nil {
		return false, errors.Wrap(err, "failed to read SARState")
	}
	return sarState, nil
}

// EnableSAR - enable/disable MM SAR
func (m *Modem) EnableSAR(ctx context.Context, enable bool) error {
	modemPath := dbus.ObjectPath(m.String())
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerSARInterface, modemPath)
	if err != nil {
		return err
	}
	if err := ph.Call(ctx, mmconst.ModemSAREnable, enable).Err; err != nil {
		return errors.Wrap(err, "failed to enable/disable SAR")
	}
	return nil
}

// SetSARPowerLevel sets the SAR power level.
func (m *Modem) SetSARPowerLevel(ctx context.Context, power uint32) error {
	modemPath := dbus.ObjectPath(m.String())
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerSARInterface, modemPath)
	if err != nil {
		return err
	}
	if err := ph.Call(ctx, mmconst.ModemSARSetPowerLevel, power).Err; err != nil {
		return errors.Wrap(err, "failed to configure SAR power level")
	}
	return nil
}

// DeleteAllMessages deletes all sms messages
func (m *Modem) DeleteAllMessages(ctx context.Context) error {
	modemPath := dbus.ObjectPath(m.String())
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerMessageInterface, modemPath)
	if err != nil {
		return errors.Wrap(err, "failed to get message interface")
	}

	msgProps, err := ph.GetProperties(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get message properties")
	}

	msgList, err := msgProps.GetObjectPaths(mmconst.ModemPropertyMessages)
	if err != nil {
		return errors.Wrap(err, "failed to get message paths")
	}

	for _, path := range msgList {
		if err := ph.Call(ctx, mmconst.MessagesDelete, path).Err; err != nil {
			testing.ContextLog(ctx, err.Error())
		}
	}
	return nil
}

// ApplyCarrierLockConfig - apply specified signed carrier lock config to the modem
func (m *Modem) ApplyCarrierLockConfig(ctx context.Context, config []byte) error {
	err := m.Call(ctx, mmconst.ModemSetCarrierLock, config).Err
	if err != nil {
		return errors.Wrap(err, "failed to set carrier lock config")
	}
	return nil
}

// GetEquipmentIdentifier - get the identity of the device. This will be the IMEI number.
func (m *Modem) GetEquipmentIdentifier(ctx context.Context) (string, error) {
	modemProps, err := m.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read modem properties")
	}

	imei, err := modemProps.GetString(mmconst.ModemPropertyEquipmentIdentifier)
	if err != nil {
		return "", errors.Wrap(err, "failed to read EquipmentIdentifier")
	}
	return imei, nil
}

// getModem3gpp creates a PropertyHolder for the Modem3gpp object.
func (m *Modem) getModem3gpp(ctx context.Context) (*Modem, error) {
	modemPath := dbus.ObjectPath(m.String())
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanager3gppModemInterface, modemPath)
	if err != nil {
		return nil, err
	}
	return &Modem{ph}, nil
}

// GetSimProperties creates a PropertyHolder for the Sim object and returns the associated Properties.
func (m *Modem) GetSimProperties(ctx context.Context, simPath dbus.ObjectPath) (*dbusutil.Properties, error) {
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerSimInterface, simPath)
	if err != nil {
		return nil, err
	}
	return ph.GetProperties(ctx)
}

// GetBearerProperties creates a PropertyHolder for the Bearer object and returns the associated Properties.
func (m *Modem) GetBearerProperties(ctx context.Context, bearerPath dbus.ObjectPath) (*dbusutil.Properties, error) {
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerBearerInterface, bearerPath)
	if err != nil {
		return nil, err
	}
	return ph.GetProperties(ctx)
}

// WaitForState waits for the modem to reach a particular state
func (m *Modem) WaitForState(ctx context.Context, state mmconst.ModemState, timeout time.Duration) error {

	if err := testing.Poll(ctx, func(ctx context.Context) (e error) {
		props, err := m.GetProperties(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get modem properties")
		}
		s, err := props.GetInt32(mmconst.ModemPropertyState)
		if err != nil {
			return errors.Wrap(err, "failed to get modem state")
		}
		if s != int32(state) {
			return errors.Wrapf(err, "Modem did not reach expected state: got: %d, want: %d", s, state)
		}
		return nil // success
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return err
	}
	return nil
}

// GetSimSlots uses the Modem.SimSlots property to fetch SimProperties for each slot.
// Returns the array of SimProperties and the array index of the primary slot on success.
// If a slot path is empty, the entry for that slot will be nil.
func (m *Modem) GetSimSlots(ctx context.Context) (simProps []*dbusutil.Properties, primary uint32, err error) {
	modemProps, err := m.GetProperties(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to call Modem.GetProperties")
	}
	simSlots, err := modemProps.GetObjectPaths(mmconst.ModemPropertySimSlots)
	if err != nil {
		return nil, 0, errors.Wrap(err, "missing Modem.SimSlots property")
	}
	primarySlot, err := modemProps.GetUint32(mmconst.ModemPropertyPrimarySimSlot)
	if err != nil {
		return nil, 0, errors.Wrap(err, "missing Modem.PrimarySimSlot property")
	}
	if primarySlot < 1 {
		return nil, 0, errors.Wrap(err, "Modem.PrimarySimSlot < 1")
	}
	primary = primarySlot - 1

	// Gather Properties for each Modem.SimSlots entry.
	for _, simPath := range simSlots {
		var props *dbusutil.Properties
		if simPath != "/" {
			props, err = m.GetSimProperties(ctx, simPath)
			if err != nil {
				return nil, 0, errors.Wrap(err, "failed to call Sim.GetProperties")
			}
		}
		simProps = append(simProps, props)
	}

	return simProps, primary, nil
}

// PollModem polls for a new modem to appear on D-Bus. oldModem is the D-Bus path of the modem that should disappear.
func PollModem(ctx context.Context, oldModem string) (*Modem, error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		newModem, err := NewModem(ctx)
		if err != nil {
			return err
		}
		if oldModem == newModem.String() {
			return errors.New("old modem still exists")
		}
		return nil
	}, &testing.PollOptions{Timeout: mmconst.ModemPollTime}); err != nil {
		return nil, errors.Wrap(err, "modem or its properties not up")
	}
	return NewModem(ctx)
}

// NewModemWithSim returns a Modem and calls EnsureValidSIM.
func NewModemWithSim(ctx context.Context) (*Modem, error) {
	modem, err := NewModem(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create modem")
	}
	modem, err = modem.EnsureValidSIM(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure SIM")
	}
	return modem, nil
}

// EnsureValidSIM ensures that the primary SIM slot is not empty.
// Useful on dual SIM DUTs where only one SIM is available, and we want to
// select the slot with the active SIM.
// On starfish setups, it ensures that a PSIM slot is active (valid or not)
func (m *Modem) EnsureValidSIM(ctx context.Context, isStarfish bool) (*Modem, error) {
	props, err := m.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call GetProperties on modem")
	}

	primarySlot, err := props.GetUint32(mmconst.ModemPropertyPrimarySimSlot)
	// For non-starfish setups, primarySlot is used only as an optimization to avoid
	// redundant slot switches. If the property is missing, we proceed with primarySlot=0,
	// which simply means the switch will always be attempted for the first valid SIM found.
	if isStarfish && err != nil {
		return nil, errors.Wrap(err, "missing Modem.PrimarySimSlot property")
	}

	simSlots, err := props.GetObjectPaths(mmconst.ModemPropertySimSlots)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get simslots property")
	}

	if isStarfish {
		return m.ensureValidSIMStarfish(ctx, primarySlot, simSlots)
	}
	return m.ensureValidSIMStandard(ctx, primarySlot, simSlots, props)
}

// ensureValidSIMStandard handles SIM validation for standard setups.
// It iterates through available slots and selects the first one with a valid SIM profile.
func (m *Modem) ensureValidSIMStandard(ctx context.Context, primarySlot uint32, simSlots []dbus.ObjectPath, props *dbusutil.Properties) (*Modem, error) {
	// b/185479169: L850 modems always return an empty simSlots array.
	if len(simSlots) == 0 {
		simPath, err := props.GetObjectPath(mmconst.ModemPropertySim)
		if err != nil {
			return nil, errors.Wrap(err, "missing sim property")
		}
		simSlots = append(simSlots, simPath)
	}
	for s, path := range simSlots {
		slotIndex := uint32(s + 1)
		// We check if the SIM in the current slot is valid (e.g. has a profile).
		valid, err := m.isValidSIM(ctx, path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to check if sim is valid")
		}
		if !valid {
			continue
		}
		// If the current primary slot is already valid, no need to switch.
		// For modems with a single slot, there is no need to try to switch
		// slots.
		// For L850(which also has a single slot), the primary slot is slot 0,
		// which doesn't match the slot convention used by other modems, so we
		// cannot switch.
		if primarySlot == slotIndex || len(simSlots) < 2 {
			return m, nil
		}
		// Switch to the first valid SIM found.
		testing.ContextLog(ctx, "switching the primary slot to: ", slotIndex)
		newm, err := m.SetPrimarySimSlot(ctx, slotIndex)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to set primary SIM slot: %d", slotIndex)
		}
		return newm, nil
	}
	return nil, errors.New("failed to validate SIM: modemmanager D-Bus object has no valid SIMs")
}

// ensureValidSIMStarfish handles SIM validation for Starfish setups.
// It ensures that the physical SIM (PSIM) slot is active by switching away from eSIM slots.
func (m *Modem) ensureValidSIMStarfish(ctx context.Context, primarySlot uint32, simSlots []dbus.ObjectPath) (*Modem, error) {
	testing.ContextLog(ctx, "Primary SIM slot: ", primarySlot)
	if primarySlot == 0 {
		// for starfish setup in a single SIM scenario: nothing to do here.
		return m, nil
	}

	if len(simSlots) == 0 { // This should not be reached as primarySlot == 0 is handled above.
		return nil, errors.Errorf("unexpected number of simSlots: %d", len(simSlots))
	}
	if len(simSlots) != 2 {
		return nil, errors.Errorf("unsupported number of simSlots: %d", len(simSlots))
	}

	for s, path := range simSlots {
		slotIndex := uint32(s + 1)
		if path == mmconst.EmptySlotPath {
			continue
		}
		simProps, err := m.GetSimProperties(ctx, path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read sim properties in slot: %d", slotIndex)
		}
		if simProps == nil {
			testing.ContextLog(ctx, "No SIM properties in slot: ", slotIndex)
			continue
		}
		eid, err := simProps.GetString(mmconst.SimPropertySimEid)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read eid from sim")
		}
		// If the slot has an EID, it is an eSIM slot.
		if eid != "" {
			// If the current primary slot is NOT the eSIM slot, then it must be the
			// physical slot (assuming a 2-slot modem), so we are good.
			if primarySlot != slotIndex {
				testing.ContextLog(ctx, "PSIM is already the primary slot: ", primarySlot)
				return m, nil
			}
			// If the eSIM slot is primary, we switch to the other slot (the PSIM slot).
			targetSimSlot := uint32(1)
			if slotIndex == 1 {
				targetSimSlot = 2
			}
			testing.ContextLog(ctx, "switching the primary slot to: ", targetSimSlot)
			newm, err := m.SetPrimarySimSlot(ctx, targetSimSlot)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to set primary SIM slot: %d", targetSimSlot)
			}
			return newm, nil
		}
	}
	// If we didn't find an eSIM to switch away from, or no switch was needed.
	return m, nil
}

// isValidSIM checks if a simPath has a connectable sim card.
func (m *Modem) isValidSIM(ctx context.Context, simPath dbus.ObjectPath) (bool, error) {
	if simPath == mmconst.EmptySlotPath {
		return false, nil
	}
	simProps, err := m.GetSimProperties(ctx, simPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to read sim properties")
	}
	ESimStatus, err := simProps.GetUint32(mmconst.SimPropertyESimStatus)
	if err != nil {
		return false, errors.Wrap(err, "failed to get ESIMStatus property")
	}
	return ESimStatus != mmconst.ESimStatusNoProfile, nil
}

// IsEnabled checks modem state and returns a boolean.
func (m *Modem) IsEnabled(ctx context.Context) (bool, error) {
	props, err := m.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to call GetProperties on modem")
	}
	modemState, err := props.GetInt32(mmconst.ModemPropertyState)
	if err != nil {
		return false, errors.Wrap(err, "missing state property")
	}
	testing.ContextLogf(ctx, "modemState in IsEnabled is %d", modemState)
	states := [6]mmconst.ModemState{
		mmconst.ModemStateEnabled,
		mmconst.ModemStateSearching,
		mmconst.ModemStateRegistered,
		mmconst.ModemStateDisconnecting,
		mmconst.ModemStateConnecting,
		mmconst.ModemStateConnected}

	for _, value := range states {
		if int32(value) == modemState {
			return true, nil
		}
	}
	return false, nil
}

// IsDisabled checks modem state and returns a boolean.
func (m *Modem) IsDisabled(ctx context.Context) (bool, error) {
	props, err := m.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to call GetProperties on modem")
	}
	modemState, err := props.GetInt32(mmconst.ModemPropertyState)
	if err != nil {
		return false, errors.Wrap(err, "missing state property")
	}
	testing.ContextLogf(ctx, "modemState in IsDisabled is %d", modemState)
	return (modemState == int32(mmconst.ModemStateDisabled)), nil
}

// IsPowered checks modem powerstate and returns a boolean.
func (m *Modem) IsPowered(ctx context.Context) (bool, error) {
	props, err := m.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to call GetProperties on modem")
	}
	modemState, err := props.GetUint32(mmconst.ModemPropertyPowered)
	if err != nil {
		return false, errors.Wrap(err, "missing powerstate property")
	}
	testing.ContextLogf(ctx, "modemState in IsPowered is %d", modemState)
	if modemState == uint32(mmconst.ModemPowerStateOn) {
		return true, nil
	}
	return false, nil
}

// IsRegistered checks modem registration state and returns a boolean.
func (m *Modem) IsRegistered(ctx context.Context) (bool, error) {
	modem3gpp, err := m.getModem3gpp(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get modem3gpp object")
	}
	props, err := modem3gpp.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to call GetProperties on modem")
	}
	regState, err := props.GetUint32(mmconst.ModemModem3gppPropertyRegistrationState)
	if err != nil {
		return false, errors.Wrap(err, "missing registration state property")
	}
	return (regState == uint32(mmconst.ModemRegStateHome) || regState == uint32(mmconst.ModemRegStateRoaming)), nil
}

// IsConnected checks modem state and returns a boolean.
func (m *Modem) IsConnected(ctx context.Context) (bool, error) {
	props, err := m.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to call GetProperties on modem")
	}
	state, err := props.GetInt32(mmconst.ModemPropertyState)
	if err != nil {
		return false, errors.Wrap(err, "missing state property")
	}
	testing.ContextLogf(ctx, "modemState in IsConnected is %d", state)
	return (state == int32(mmconst.ModemStateConnecting) || state == int32(mmconst.ModemStateConnected)), nil
}

// EnableUnchecked sets the ModemEnable state to true.
func (m *Modem) EnableUnchecked(ctx context.Context) error {
	if err := m.Call(ctx, mmconst.ModemEnable, true).Err; err != nil {
		return errors.Wrap(err, "modem enable failed")
	}
	return nil
}

// Enable sets the ModemEnable state to true and calls EnsureEnabled.
func (m *Modem) Enable(ctx context.Context) error {
	if err := m.Call(ctx, mmconst.ModemEnable, true).Err; err != nil {
		return errors.Wrap(err, "modem enable failed")
	}
	if err := m.EnsureEnabled(ctx); err != nil {
		return errors.Wrap(err, "modem not enabled")
	}
	return nil
}

// DisableUnchecked sets the ModemEnable state to false.
func (m *Modem) DisableUnchecked(ctx context.Context) error {
	if err := m.Call(ctx, mmconst.ModemEnable, false).Err; err != nil {
		return errors.Wrap(err, "modem disable failed")
	}
	return nil
}

// Disable sets the ModemEnable state to false and calls EnsureDisabled.
func (m *Modem) Disable(ctx context.Context) error {
	if err := m.Call(ctx, mmconst.ModemEnable, false).Err; err != nil {
		return errors.Wrap(err, "modem disable failed")
	}
	if err := m.EnsureDisabled(ctx); err != nil {
		return errors.Wrap(err, "modem not disabled")
	}
	return nil
}

// EnsureEnabled polls for modem state property to be enabled.
func (m *Modem) EnsureEnabled(ctx context.Context) error {
	// Poll for expected power state as powered state change can take time
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isPowered, err := m.IsPowered(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to read modem powered state")
		}
		if !isPowered {
			return errors.New("still modem not powered")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  5 * time.Second,
		Interval: 200 * time.Millisecond,
	}); err != nil {
		return errors.Wrap(err, "failed to verify modem power state")
	}

	// Poll for expected modem state
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isEnabled, err := m.IsEnabled(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch enabled state")
		}
		if !isEnabled {
			return errors.New("modem not enabled")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 1 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to verify modem enabled")
	}
	return nil
}

// EnsureDisabled polls for modem state property to be disabled.
func (m *Modem) EnsureDisabled(ctx context.Context) error {
	// poll for expected modem state
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isEnabled, err := m.IsEnabled(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch enabled state")
		}
		if isEnabled {
			return errors.New("modem still enabled")
		}
		isDisabled, err := m.IsDisabled(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch disabled state")
		}
		if !isDisabled {
			return errors.New("modem still not disabled")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  120 * time.Second,
		Interval: 1 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to verify modem disabled")
	}
	return nil
}

// SetPrimarySimSlot switches primary SIM slot to a given slot
func (m *Modem) SetPrimarySimSlot(ctx context.Context, primary uint32) (*Modem, error) {
	if c := m.Call(ctx, "SetPrimarySimSlot", primary); c.Err != nil {
		return nil, errors.Wrapf(c.Err, "failed while switching the primary sim slot to: %d", primary)
	}
	newm, err := PollModem(ctx, m.String())
	if err != nil {
		return nil, errors.Wrapf(err, "could not find modem after switching the primary slot to: %d", primary)
	}
	return newm, nil
}

// EnsureConnectState polls for modem state to be connected or disconnected.
func (m *Modem) EnsureConnectState(ctx context.Context, expectedConnected bool) error {
	// poll for expected modem state
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := m.EnsureEnabled(ctx); err != nil {
			return errors.Wrap(err, "modem not enabled")
		}
		isConnected, err := m.IsConnected(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch connected state")
		}
		if expectedConnected != isConnected {
			return errors.Errorf("unexpected connect state, got %t, expected %t", isConnected, expectedConnected)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  60 * time.Second,
		Interval: 1 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to set modem connect state")
	}
	return nil
}

// WaitUntilRegistered polls for simple modem property m3gpp-registration-state until the modem is
// in registered state. This function does not check for preconditions, and will run until the modem
// is registered or it times out.
func (m *Modem) WaitUntilRegistered(ctx context.Context, timeout time.Duration) error {
	// poll for expected modem state
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isRegistered, err := m.IsRegistered(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch registration state")
		}
		if !isRegistered {
			return errors.New("modem not registered")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  timeout,
		Interval: 1 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to verify modem registration state")
	}
	return nil
}

// EnsureRegistered polls for simple modem property m3gpp-registration-state.
func (m *Modem) EnsureRegistered(ctx context.Context) error {
	if isPowered, err := m.IsPowered(ctx); err != nil {
		return errors.New("failed to read modem powered state")
	} else if !isPowered {
		return errors.New("modem not powered")
	}
	if isEnabled, err := m.IsEnabled(ctx); err != nil {
		return errors.New("failed to read modem enabled state")
	} else if !isEnabled {
		return errors.New("modem not enabled")
	}
	return m.WaitUntilRegistered(ctx, 120*time.Second)
}

// Connect calls the connect function on simple modem D-Bus and returns the bearer path if it connects successfully.
func (m *Modem) Connect(ctx context.Context, props map[string]interface{}) (dbus.ObjectPath, error) {
	bearerPath := dbus.ObjectPath("")
	simpleModem, err := m.getSimpleModem(ctx)
	if err != nil {
		return bearerPath, errors.Wrap(err, "could not get simpleModem object")
	}
	// Validate the apn settings for consistency.
	if _, ok := props[mmconst.BearerPropertyApnType]; !ok {
		props[mmconst.BearerPropertyApnType] = mmconst.BearerAPNTypeDefault
	}
	response := simpleModem.Call(ctx, mmconst.ModemConnect, props)
	if (response.Err != nil) && (strings.Contains(response.Err.Error(), "no-service")) {
		return bearerPath, errors.Wrap(response.Err, "failed to connect can be network issue")
	} else if response.Err != nil {
		return bearerPath, errors.Wrap(response.Err, "failed to connect")
	}
	if isConnected, err := m.IsConnected(ctx); err != nil {
		return bearerPath, errors.Wrap(err, "failed to fetch connected state")
	} else if !isConnected {
		return bearerPath, errors.Wrap(err, "modem not connected")
	}
	if len(response.Body) != 1 {
		return bearerPath, errors.Errorf("connect resulted in incorrect response len: %d", len(response.Body))
	}
	bearerPath, ok := response.Body[0].(dbus.ObjectPath)
	if !ok {
		return bearerPath, errors.New("could not parse bearer path")
	}
	return bearerPath, nil
}

// Disconnect calls the disconnect function on simple modem D-Bus.
func (m *Modem) Disconnect(ctx context.Context, path dbus.ObjectPath) error {
	simpleModem, err := m.getSimpleModem(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get simpleModem object")
	}
	if err := simpleModem.Call(ctx, mmconst.ModemDisconnect, path).Err; err != nil {
		return errors.Wrap(err, "modem disconnect failed")
	}
	return nil
}

// DisconnectAll calls the disconnect function on simple modem D-Bus to disconnect all bearers.
func (m *Modem) DisconnectAll(ctx context.Context) error {
	return m.Disconnect(ctx, dbus.ObjectPath("/"))
}

// InhibitModem inhibits the first available modem on DBus. Use the returned callback to uninhibit.
func InhibitModem(ctx context.Context) (func(ctx context.Context) error, error) {
	modem, err := NewModem(ctx)
	emptyUninhibit := func(ctx context.Context) error { return nil }
	if err != nil {
		return emptyUninhibit, errors.Wrap(err, "failed to create Modem")
	}
	props, err := modem.GetProperties(ctx)
	if err != nil {
		return emptyUninhibit, errors.Wrap(err, "failed to call GetProperties on Modem")
	}
	device, err := props.GetString(mmconst.ModemPropertyDevice)
	if err != nil {
		return emptyUninhibit, errors.Wrap(err, "missing Device property")
	}

	obj, err := dbusutil.NewDBusObject(ctx, DBusModemmanagerService, DBusModemmanagerInterface, DBusModemmanagerPath)
	if err != nil {
		return emptyUninhibit, errors.Wrap(err, "unable to connect to ModemManager1")
	}
	if err := obj.Call(ctx, "InhibitDevice", device, true).Err; err != nil {
		return emptyUninhibit, errors.Wrap(err, "inhibitDevice(true) failed")
	}

	uninhibit := func(ctx context.Context) error {
		if err := obj.Call(ctx, "InhibitDevice", device, false).Err; err != nil {
			return errors.Wrap(err, "inhibitDevice(false) failed")
		}
		return nil
	}
	return uninhibit, nil
}

// SwitchSlot switches from current slot to new slot on dual sim duts, returns new primary slot.
func SwitchSlot(ctx context.Context) (uint32, error) {

	modem, err := NewModem(ctx)
	if err != nil {
		return math.MaxUint32, errors.Wrap(err, "failed to create modem")
	}
	props, err := modem.GetProperties(ctx)
	if err != nil {
		return math.MaxUint32, errors.Wrap(err, "failed to call getproperties on modem")
	}
	_, err = props.GetObjectPath(mmconst.ModemPropertySim)
	if err != nil {
		return math.MaxUint32, errors.Wrap(err, "missing sim property")
	}
	simSlots, err := props.GetObjectPaths(mmconst.ModemPropertySimSlots)
	if err != nil {
		return math.MaxUint32, errors.Wrap(err, "failed to get simslots property")
	}
	primary, err := props.GetUint32(mmconst.ModemPropertyPrimarySimSlot)
	if err != nil {
		return math.MaxUint32, errors.Wrap(err, "missing primarysimslot property")
	}
	numSlots := len(simSlots)
	testing.ContextLogf(ctx, "Number of slots on dut: %d", numSlots)
	if numSlots < 2 {
		return primary, nil
	}

	// For dual sim devices
	var setSlot uint32 = 1
	if primary == setSlot {
		setSlot = 2
	}
	_, err = modem.SetPrimarySimSlot(ctx, setSlot)
	if err != nil {
		return math.MaxUint32, err
	}

	return uint32(setSlot), nil
}

// GetSimProperty gets current modem sim property.
func (m *Modem) GetSimProperty(ctx context.Context, propertyName string) (string, error) {
	modemProps, err := m.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to call getproperties on modem")
	}
	simPath, err := modemProps.GetObjectPath(mmconst.ModemPropertySim)
	if err != nil {
		return "", errors.Wrap(err, "failed to get modem sim property")
	}
	testing.ContextLogf(ctx, "SIM path =%s", simPath)
	simProps, err := m.GetSimProperties(ctx, simPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to read sim properties")
	}
	// Try to read a string, otherwise try to read an array of ints(GID1 is an array).
	if info, err := simProps.GetString(propertyName); err == nil {
		return info, nil
	}
	if info, err := simProps.GetUint8s(propertyName); err == nil {
		var hexStrings []string
		for _, num := range info {
			hexStrings = append(hexStrings, fmt.Sprintf("%02x", num))
		}
		return strings.Join(hexStrings, ""), nil
	}

	return "", errors.Wrapf(err, "error getting property %q", propertyName)

}

// GetEid gets current modem sim eid, return eid if esim is active.
func (m *Modem) GetEid(ctx context.Context) (string, error) {
	return m.GetSimProperty(ctx, mmconst.SimPropertySimEid)
}

// GetIMSI gets current modem sim IMSI, return IMSI if sim is active.
func (m *Modem) GetIMSI(ctx context.Context) (string, error) {
	return m.GetSimProperty(ctx, mmconst.SimPropertySimIMSI)
}

// GetOperatorIdentifier gets current modem sim Operator Identifier, return Operator Identifier if sim is active.
func (m *Modem) GetOperatorIdentifier(ctx context.Context) (string, error) {
	return m.GetSimProperty(ctx, mmconst.SimPropertySimOperatorIdentifier)
}

// GetSimIdentifier gets current modem sim Identifier, return Identifier if sim is active.
func (m *Modem) GetSimIdentifier(ctx context.Context) (string, error) {
	return m.GetSimProperty(ctx, mmconst.SimPropertySimIdentifier)
}

// GetGID1dentifier gets current modem sim GID1 Identifier, return GID1 Identifier if sim is active.
func (m *Modem) GetGID1dentifier(ctx context.Context) (string, error) {
	return m.GetSimProperty(ctx, mmconst.SimPropertySimGID1)
}

// GetOperatorAndGid1Identifiers gets current modem sim Operator and GID1 Identifiers, return values if sim is active.
func (m *Modem) GetOperatorAndGid1Identifiers(ctx context.Context) (string, string, error) {
	operatorIdentifier, err := m.GetOperatorIdentifier(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read Operator Identifier from modemmanager")
	}
	gid1Identifier, err := m.GetGID1dentifier(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read GID1 Identifier from modemmanager")
	}
	return operatorIdentifier, gid1Identifier, nil
}

// GetOperatorCode gets current operator code, return operator code if sim is active.
func (m *Modem) GetOperatorCode(ctx context.Context) (string, error) {
	modem3gpp, err := m.getModem3gpp(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get 3gpp modem")
	}
	modemProps, err := modem3gpp.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to call getproperties on modem3gpp")
	}
	operatorCode, err := modemProps.GetString(mmconst.ModemModem3gppPropertyOperatorCode)
	if err != nil {
		return "", errors.Wrapf(err, "error getting property %q", mmconst.ModemModem3gppPropertyOperatorCode)
	}

	return operatorCode, nil
}

// GetMaxActiveMultiplexedBearers gets the maximum number of active multiplexed bearers.
func (m *Modem) GetMaxActiveMultiplexedBearers(ctx context.Context, modem *Modem) (uint32, error) {
	modemPath := ObjectPath{dbus.ObjectPath(m.String()), nil}
	value := modemPath.GetPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerModemInterface).
		GetProperties(ctx).
		Get(mmconst.ModemPropertyMaxActiveMultiplexedBearers)

	if value.err != nil {
		return 0, errors.Wrap(value.err, "failed to read MaxActiveMultiplexedBearers")
	}
	return value.iface.(uint32), nil
}

// GetFwVersion gets the FW version on the modem. This is known in ModemManager as the Revision.
func (m *Modem) GetFwVersion(ctx context.Context, modem *Modem) (string, error) {
	modemPath := ObjectPath{dbus.ObjectPath(m.String()), nil}
	value := modemPath.GetPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerModemInterface).
		GetProperties(ctx).
		Get(mmconst.ModemPropertyRevision)

	if value.err != nil {
		return "", errors.Wrap(value.err, "failed to read Revision")
	}
	return value.iface.(string), nil
}

// SetInitialEpsBearerSettings sets the Attach APN.
func (m *Modem) SetInitialEpsBearerSettings(ctx context.Context, props map[string]interface{}) error {
	modem3gpp, err := m.getModem3gpp(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get 3gpp modem")
	}
	// Validate the apn settings for consistency.
	if _, ok := props[mmconst.BearerPropertyApnType]; !ok {
		props[mmconst.BearerPropertyApnType] = mmconst.BearerAPNTypeInitial
	}
	if c := modem3gpp.Call(ctx, "SetInitialEpsBearerSettings", props); c.Err != nil {
		return errors.Wrap(c.Err, "failed to set initial EPS bearer settings")
	}
	return nil
}

// GetInitialEpsBearerSettings gets the Attach APN.
func (m *Modem) GetInitialEpsBearerSettings(ctx context.Context, modem *Modem) (map[string]interface{}, error) {
	modemPath := ObjectPath{dbus.ObjectPath(m.String()), nil}
	apnPropsGet := modemPath.GetPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanager3gppModemInterface).
		GetProperties(ctx).
		GetObjectPath(mmconst.ModemModem3gppPropertyInitialEpsBearer).
		GetObjectProperties(ctx, DBusModemmanagerService, DBusModemmanagerBearerInterface).
		Get(mmconst.BearerPropertyProperties)

	if apnPropsGet.err != nil {
		return nil, errors.Wrap(apnPropsGet.err, "failed to read InitialEpsBearer properties")
	}
	bearerProps, ok := apnPropsGet.iface.(map[string]interface{})
	if !ok {
		return nil, errors.New("failed to parse InitialEpsBearer properties")
	}
	return bearerProps, nil
}

// GetFirstDataBearer gets the object path of the first data bearer.
// This function doesn't check the status of the bearer.
func (m *Modem) GetFirstDataBearer(ctx context.Context, apnType mmconst.BearerAPNType) (*Bearer, error) {
	modemPath := ObjectPath{dbus.ObjectPath(m.String()), nil}
	bearerPaths := modemPath.GetPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerModemInterface).
		GetProperties(ctx).
		GetObjectPaths(mmconst.ModemPropertyBearers)

	if bearerPaths.err != nil {
		return nil, errors.Wrap(bearerPaths.err, "failed to read Bearer paths")
	}
	for _, opath := range bearerPaths.objectPaths {
		bearer, err := NewBearer(ctx, opath, dbus.ObjectPath(m.String()))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Bearer object")
		}
		isAPNType, err := bearer.IsAPNType(ctx, apnType)
		if err != nil {
			return nil, err
		}
		if isAPNType {
			return bearer, nil
		}
	}
	return nil, errors.New("no data bearers found")
}

// GetFirstConnectedDataBearer gets the first connected bearer.
func (m *Modem) GetFirstConnectedDataBearer(ctx context.Context, apnType mmconst.BearerAPNType) (*Bearer, error) {
	modemPath := ObjectPath{dbus.ObjectPath(m.String()), nil}
	bearerPaths := modemPath.GetPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerModemInterface).
		GetProperties(ctx).
		GetObjectPaths(mmconst.ModemPropertyBearers)

	if bearerPaths.err != nil {
		return nil, errors.Wrap(bearerPaths.err, "failed to read Bearer paths")
	}
	for _, opath := range bearerPaths.objectPaths {
		bearer, err := NewBearer(ctx, opath, dbus.ObjectPath(m.String()))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Bearer object")
		}
		isAPNType, err := bearer.IsAPNType(ctx, apnType)
		if err != nil {
			return nil, err
		}
		if !isAPNType {
			continue
		}

		if bearer.Connected() {
			return bearer, nil
		}
	}
	return nil, errors.New("no bearers are connected")
}

// DeleteAllBearers deletes all data bearers.
func (m *Modem) DeleteAllBearers(ctx context.Context, modem *Modem) error {
	modemPath := ObjectPath{dbus.ObjectPath(m.String()), nil}
	bearerPaths := modemPath.GetPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerModemInterface).
		GetProperties(ctx).
		GetObjectPaths(mmconst.ModemPropertyBearers)

	if bearerPaths.err != nil {
		return errors.Wrap(bearerPaths.err, "failed to read Bearer paths")
	}
	for _, opath := range bearerPaths.objectPaths {
		if c := modem.Call(ctx, mmconst.ModemDeleteBearer, opath); c.Err != nil {
			return errors.Wrapf(c.Err, "failed to delete bearer: %q", opath)
		}
	}
	return nil
}

// SetModemmanagerLogLevel - Set the logging level of ModemManager to the specified level.
func SetModemmanagerLogLevel(ctx context.Context, level string) error {
	obj, err := dbusutil.NewDBusObject(ctx, DBusModemmanagerService, DBusModemmanagerInterface, DBusModemmanagerPath)
	if err != nil {
		return errors.Wrap(err, "failed to get modemmanager object")
	}
	if c := obj.Call(ctx, mmconst.ModemSetLogging, level); c.Err != nil {
		return errors.Wrap(c.Err, "failed to set modemmanager log level to DEBUG")
	}
	return nil
}

// GetProfiles fetches the list of 3GPP profiles from the profile manager interface.
func (m *Modem) GetProfiles(ctx context.Context) ([]map[string]interface{}, error) {
	modemPath := dbus.ObjectPath(m.String())
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerProfileManagerInterface, modemPath)
	if err != nil {
		return nil, err
	}

	response := ph.Call(ctx, mmconst.ModemProfileManagerList)
	if response.Err != nil {
		return nil, errors.Wrap(response.Err, "failed to list profiles")
	}
	if len(response.Body) != 1 {
		return nil, errors.Errorf("profile list call resulted in incorrect response len: %d", len(response.Body))
	}
	body, ok := response.Body[0].([]map[string]dbus.Variant)
	if !ok {
		return nil, errors.New("could not parse profiles")
	}

	// Unwrap the dbus.Variants into their underlying value.
	profiles := make([]map[string]interface{}, 0, len(body))
	for _, props := range body {
		profile := make(map[string]interface{})
		for key, value := range props {
			profile[key] = value.Value()
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}
