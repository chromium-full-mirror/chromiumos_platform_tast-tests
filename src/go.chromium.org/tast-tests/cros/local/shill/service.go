// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shill

import (
	"context"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dbusServiceInterface = "org.chromium.flimflam.Service"
)

// Service wraps a Service D-Bus object in shill.
type Service struct {
	*PropertyHolder
}

// NewService connects to a service in Shill.
func NewService(ctx context.Context, path dbus.ObjectPath) (*Service, error) {
	ph, err := NewPropertyHolder(ctx, dbusService, dbusServiceInterface, path)
	if err != nil {
		return nil, err
	}
	return &Service{PropertyHolder: ph}, nil
}

// ObjectPath returns Service's D-Bus object path.
func (s *Service) ObjectPath() dbus.ObjectPath {
	return s.PropertyHolder.ObjectPath()
}

// GetDevice returns the Device object corresponding to the Service object
func (s *Service) GetDevice(ctx context.Context) (*Device, error) {
	serviceProps, err := s.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	deviceObjPath, err := serviceProps.GetObjectPath(shillconst.ServicePropertyDevice)
	if err != nil {
		return nil, errors.Wrap(err, "no device associated with service")
	}
	device, err := NewDevice(ctx, deviceObjPath)
	if err != nil {
		return nil, err
	}
	return device, nil
}

// GetCurrentIPConfig returns the IPConfig for this Service.
func (s *Service) GetCurrentIPConfig(ctx context.Context) (*IPConfig, error) {
	serviceProps, err := s.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	ipConfigObjPath, err := serviceProps.GetObjectPath(shillconst.ServicePropertyIPConfig)
	if err != nil {
		return nil, errors.Wrap(err, "no device associated with service")
	}
	ipConfig, err := NewIPConfig(ctx, ipConfigObjPath)
	if err != nil {
		return nil, err
	}
	return ipConfig, nil
}

// GetIPConfigs returns the IPConfig objects list of the associated Device. Note
// that this is not the IPConfig object of the Service.
func (s *Service) GetIPConfigs(ctx context.Context) ([]*IPConfig, error) {
	device, err := s.GetDevice(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get associated Device for %s", s.ObjectPath())
	}
	props, err := device.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get Device properties for %s", s.ObjectPath())
	}
	paths, err := props.GetObjectPaths(shillconst.DevicePropertyIPConfigs)
	var ret []*IPConfig
	for _, path := range paths {
		ipconfig, err := NewIPConfig(ctx, path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create IPConfig from path %s", path)
		}
		ret = append(ret, ipconfig)
	}
	return ret, nil
}

// GetSignalStrength return the current signal strength
func (s *Service) GetSignalStrength(ctx context.Context) (uint8, error) {
	props, err := s.GetProperties(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get properties")
	}
	strength, err := props.GetUint8(shillconst.ServicePropertyStrength)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get strength from properties")
	}
	return strength, nil
}

// GetState returns the current service state.
func (s *Service) GetState(ctx context.Context) (string, error) {
	props, err := s.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrap(err, "unable to get properties")
	}
	state, err := props.GetString(shillconst.ServicePropertyState)
	if err != nil {
		return "", errors.Wrap(err, "unable to get service state from properties")
	}
	return state, nil
}

// GetType returns the current service type.
func (s *Service) GetType(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertyType)
}

// GetGUID returns the current service GUID.
func (s *Service) GetGUID(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertyGUID)
}

// GetName returns the current service name.
func (s *Service) GetName(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertyName)
}

// GetEid returns the current service EID (cellular only).
func (s *Service) GetEid(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertyCellularEID)
}

// GetIccid returns the current service ICCID (cellular only).
func (s *Service) GetIccid(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertyCellularICCID)
}

// GetSecurity returns the current service Security (WiFi only).
func (s *Service) GetSecurity(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertySecurity)
}

// GetEAPCertID returns the current service EAP certificate ID.
func (s *Service) GetEAPCertID(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertyEAPCertID)
}

// GetEAPKeyID returns the current service EAP key ID.
func (s *Service) GetEAPKeyID(ctx context.Context) (string, error) {
	return s.getStringProperty(ctx, shillconst.ServicePropertyEAPKeyID)
}

// IsConnected returns true if the the service is connected.
func (s *Service) IsConnected(ctx context.Context) (bool, error) {
	props, err := s.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "unable to get properties")
	}
	connected, err := props.GetBool(shillconst.ServicePropertyIsConnected)
	if err != nil {
		return false, errors.Wrap(err, "unable to get IsConnected from properties")
	}
	return connected, nil
}

// IsVisible returns true if the the service is visible.
func (s *Service) IsVisible(ctx context.Context) (bool, error) {
	props, err := s.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "unable to get properties")
	}
	visible, err := props.GetBool(shillconst.ServicePropertyVisible)
	if err != nil {
		return false, errors.Wrap(err, "unable to get IsVisible from properties")
	}
	return visible, nil
}

// getModemState is a helper which gets the underlying state of the MM modem object for
// a service. This allows us to see if a modem is registered.
func (s *Service) getModemState(ctx context.Context) (mmconst.ModemState, error) {
	device, err := s.GetDevice(ctx)
	if err != nil {
		return mmconst.ModemStateUnknown, errors.Wrap(err, "unable to get device for service")
	}

	props, err := device.GetProperties(ctx)
	if err != nil {
		return mmconst.ModemStateUnknown, err
	}

	modemPath, err := props.GetString(shillconst.DevicePropertyDBusObject)
	if err != nil {
		return mmconst.ModemStateUnknown, errors.Wrap(err, "unable to get MM modem for device")
	}

	modem, err := modemmanager.NewModemFromPath(ctx, dbus.ObjectPath(modemPath))
	if err != nil {
		return mmconst.ModemStateUnknown, err
	}

	modemProps, err := modem.GetProperties(ctx)
	if err != nil {
		return mmconst.ModemStateUnknown, err
	}

	state, err := modemProps.GetInt32(mmconst.ModemPropertyState)
	if err != nil {
		return mmconst.ModemStateUnknown, err
	}

	return mmconst.ModemState(state), nil
}

// WaitForConnectedOrError polls for either:
// * Service.IsConnected to be true, in which case nil is returned.
// * Service.Error to be set to an error value, in which case that is returned as an error.
// Any failure also returns an error.
func (s *Service) WaitForConnectedOrError(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		props, err := s.GetProperties(ctx)
		if err != nil {
			return err
		}
		connected, err := props.Get(shillconst.ServicePropertyIsConnected)
		if err != nil {
			return err
		}
		if connected.(bool) {
			return nil
		}
		errorVal, err := props.Get(shillconst.ServicePropertyError)
		if err != nil {
			return err
		}
		errorStr := errorVal.(string)
		// Treat "no-failure" Error values the same as the no error case.
		if errorStr != shillconst.ServiceErrorNoFailure {
			return testing.PollBreak(errors.New(errorStr))
		}

		// There is no error on the shill service. Our connection attempt
		// might still be pending, so we should make sure we know whether
		// the modem is at least registered.
		modemState, err := s.getModemState(ctx)
		if err != nil {
			return err
		}

		modemStateStrs := map[mmconst.ModemState]string{
			mmconst.ModemStateFailed:        "failed",
			mmconst.ModemStateUnknown:       "unknown",
			mmconst.ModemStateInitializing:  "initializing",
			mmconst.ModemStateLocked:        "locked",
			mmconst.ModemStateDisabled:      "disabled",
			mmconst.ModemStateDisabling:     "disabling",
			mmconst.ModemStateEnabling:      "enabling",
			mmconst.ModemStateEnabled:       "enabled",
			mmconst.ModemStateSearching:     "searching",
			mmconst.ModemStateRegistered:    "registered",
			mmconst.ModemStateDisconnecting: "disconnecting",
			mmconst.ModemStateConnecting:    "connecting",
			mmconst.ModemStateConnected:     "connected",
		}
		modemStateStr, ok := modemStateStrs[modemState]
		if !ok {
			modemStateStr = "invalid"
		}

		return errors.Errorf("not connected and no error. MM modem state: %v", modemStateStr)
	}, &testing.PollOptions{
		Timeout:  shillconst.DefaultTimeout,
		Interval: 100 * time.Millisecond,
	})
}

// Connect calls the Connect method on the service.
func (s *Service) Connect(ctx context.Context) error {
	return s.Call(ctx, "Connect").Err
}

// Disconnect calls the Disconnect method on the service.
func (s *Service) Disconnect(ctx context.Context) error {
	return s.Call(ctx, "Disconnect").Err
}

// Reconnect disconnects the service at first and then connect to it again.
func (s *Service) Reconnect(ctx context.Context) error {
	if err := s.Disconnect(ctx); err != nil {
		return errors.Wrap(err, "failed to disconnect")
	}
	// Shill will disable auto-connect for a service if it is disconnected
	// explicitly, but this may not work for Ethernet services. When a Ethernet
	// service is disconnected, a service reload might be triggered and as a
	// result the user-initiate-disconnect bit is cleared, and the service will be
	// reconnected immediately. In that case, an error will be returned when
	// Connect() is called.
	if err := s.Connect(ctx); err != nil {
		// The error messages are defined in Service::Connect() function in
		// platform2/shill/service.cc file.
		if strings.Contains(err.Error(), "already connected") || strings.Contains(err.Error(), "already connecting") {
			return nil
		}
		return errors.Wrap(err, "failed to reconnect")
	}
	return nil
}

// Remove calls the Remove method on the service.
func (s *Service) Remove(ctx context.Context) error {
	return s.Call(ctx, "Remove").Err
}

func (s *Service) getStringProperty(ctx context.Context, propertyName string) (string, error) {
	props, err := s.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read service properties")
	}
	value, err := props.GetString(propertyName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read service property %s", propertyName)
	}
	return value, nil
}
