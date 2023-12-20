// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wpasupplicant provides utilities to interact with wpa_supplicant
// via dbus.
package wpasupplicant

import (
	"context"
	"crypto/rand"
	"net"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/testing"
)

const (
	dbusBasePath              = "/fi/w1/wpa_supplicant1"
	dbusBaseInterface         = "fi.w1.wpa_supplicant1"
	dbusBaseGetIfaceMethod    = "GetInterface"
	dbusCreateInterfaceMethod = "CreateInterface"
	dbusRemoveInterfaceMethod = "RemoveInterface"
)

// Supplicant is the object to interact with wpa_supplicant's
// fi.w1.wpa_supplicant1 interface.
type Supplicant struct {
	dbus *dbusutil.DBusObject
}

// NewSupplicant creates a Supplicant object.
func NewSupplicant(ctx context.Context) (*Supplicant, error) {
	d, err := dbusutil.NewDBusObject(ctx, dbusBaseInterface, dbusBaseInterface, dbusBasePath)
	if err != nil {
		return nil, err
	}
	return &Supplicant{dbus: d}, nil
}

// GetInterface calls fi.w1.wpa_supplicant1.GetInterface to get the object path of the
// interface with name and return the Interface object with the object path.
func (s *Supplicant) GetInterface(ctx context.Context, name string) (*Interface, error) {
	var path dbus.ObjectPath
	if err := s.dbus.Call(ctx, dbusBaseGetIfaceMethod, name).Store(&path); err != nil {
		return nil, err
	}
	return NewInterface(ctx, path)
}

// CreateInterface calls fi.w1.wpa_supplicant1.CreateInterface to include particular interface
// under supplicant control.
func (s *Supplicant) CreateInterface(ctx context.Context, name, driver, cfg string) error {
	var empty dbus.ObjectPath

	ifaceCfg := map[string]dbus.Variant{"Ifname": dbus.MakeVariant(name),
		"Driver":     dbus.MakeVariant(driver),
		"ConfigFile": dbus.MakeVariant(cfg)}

	// We don't need to return path, just check if it's correct. We can always get it via GetInterface() if needed.
	return s.dbus.Call(ctx, dbusCreateInterfaceMethod, ifaceCfg).Store(&empty)
}

func (s *Supplicant) GenerateRandMAC(ctx context.Context) net.HardwareAddr {
	const macBitLocal = 0x2
	const macBitMulticast = 0x1

	mac := make(net.HardwareAddr, 6)
	if _, err := rand.Read(mac); err != nil {
		testing.ContextLog(ctx, "failed to generate a random MAC address")
	}
	mac[0] = (mac[0] &^ macBitMulticast) | macBitLocal
	return mac
}

// CreateNewInterface calls fi.w1.wpa_supplicant1.CreateInterface to create a new interface
// and include it under supplicant control.
func (s *Supplicant) CreateNewInterface(ctx context.Context, name, driver, cfg, ifaceType string) error {
	var empty dbus.ObjectPath

	ifaceCfg := map[string]dbus.Variant{"Ifname": dbus.MakeVariant(name),
		"Driver":     dbus.MakeVariant(driver),
		"ConfigFile": dbus.MakeVariant(cfg),
		"Create":     dbus.MakeVariant(true),
		"Type":       dbus.MakeVariant(ifaceType),
		"Address":    dbus.MakeVariant(s.GenerateRandMAC(ctx).String())}

	// We don't need to return path, just check if it's correct. We can always get it via GetInterface() if needed.
	return s.dbus.Call(ctx, dbusCreateInterfaceMethod, ifaceCfg).Store(&empty)
}

// RemoveInterface calls fi.w1.wpa_supplicant1.RemoveInterface to remove particular interface
// from supplicant control.
func (s *Supplicant) RemoveInterface(ctx context.Context, name string) error {
	var path dbus.ObjectPath
	if err := s.dbus.Call(ctx, dbusBaseGetIfaceMethod, name).Store(&path); err != nil {
		return err
	}
	return s.dbus.Call(ctx, dbusRemoveInterfaceMethod, path).Err
}
