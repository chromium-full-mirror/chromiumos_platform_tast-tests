// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shill

import (
	"context"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast/core/errors"
)

const (
	dbusProfileInterface = "org.chromium.flimflam.Profile"
)

// Profile wraps a Profile D-Bus object in shill.
type Profile struct {
	*PropertyHolder
}

// NewProfile connects to a profile in Shill.
func NewProfile(ctx context.Context, path dbus.ObjectPath) (*Profile, error) {
	ph, err := NewPropertyHolder(ctx, dbusService, dbusProfileInterface, path)
	if err != nil {
		return nil, err
	}
	return &Profile{PropertyHolder: ph}, nil
}

// GetEntry calls the GetEntry method on the profile.
func (p *Profile) GetEntry(ctx context.Context, entryID string) (map[string]interface{}, error) {
	var entryProps map[string]interface{}
	if err := p.Call(ctx, "GetEntry", entryID).Store(&entryProps); err != nil {
		return nil, errors.Wrapf(err, "failed to get entry %s", entryID)
	}
	return entryProps, nil
}

// DeleteEntry calls the DeleteEntry method on the profile.
func (p *Profile) DeleteEntry(ctx context.Context, entryID string) error {
	return p.Call(ctx, "DeleteEntry", entryID).Err
}

// SetAlwaysOnVPN sets the AlwaysOnVpnMode and AlwaysOnVpnService properties on
// the profile. If mode is "off", svc must be nil.
func (p *Profile) SetAlwaysOnVPN(ctx context.Context, mode shillconst.AlwaysOnVPNMode, svc *Service) error {
	if (mode == shillconst.AlwaysOnVPNModeOff) != (svc == nil) {
		return errors.Errorf("mode %v and service %v does not match", mode, svc)
	}
	if svc != nil {
		if err := p.SetProperty(ctx, shillconst.ProfilePropertyAlwaysOnVPNService, svc.ObjectPath()); err != nil {
			return errors.Wrap(err, "failed to set Always-on VPN service")
		}
	}
	if err := p.SetProperty(ctx, shillconst.ProfilePropertyAlwaysOnVPNMode, mode); err != nil {
		return errors.Wrap(err, "failed to set Always-on VPN mode")
	}
	return nil
}
