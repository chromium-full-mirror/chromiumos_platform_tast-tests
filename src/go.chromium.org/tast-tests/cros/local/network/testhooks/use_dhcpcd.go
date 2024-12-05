// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
)

// DhcpcdVersion describes the major version of dhcpcd.
type DhcpcdVersion int

const (
	// Dhcpcd7 corresponds to dhcpcd 7.x.x.
	Dhcpcd7 DhcpcdVersion = iota
	// Dhcpcd10 corresponds to dhcpcd 10.x.x.
	Dhcpcd10
)

// setDhcpcdVersionHook implements hook interface.
type setDhcpcdVersionHook struct {
	noErrorHandlersMixin

	version     DhcpcdVersion
	restoreFunc func(ctx context.Context) error
}

func (h *setDhcpcdVersionHook) name() string {
	switch h.version {
	case Dhcpcd7:
		return "set_dhcpcd7"
	case Dhcpcd10:
		return "set_dhcpcd10"
	}
	return "undefined"
}

func (h *setDhcpcdVersionHook) setUp(ctx context.Context) error {
	shillManager, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create shill client")
	}
	p, err := shillManager.GetProperties(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get shill properties")
	}
	useLegacyDhcpcd, err := p.GetBool(shillconst.ManagerPropertyUseLegacyDHCPCD)
	if err != nil {
		return errors.Wrap(err, "failed to get shill property UseLegacyDHCPCD")
	}

	newUseLegacyDhcpcd := h.version == Dhcpcd7
	// If the current version of dhcpcd is exactly what we want, do nothing;
	// otherwise switch the dhcpcd version and recover it in tearDown.
	if newUseLegacyDhcpcd == useLegacyDhcpcd {
		return nil
	}
	h.restoreFunc = setUseLegacyDHCPCDProperty(useLegacyDhcpcd)
	return setUseLegacyDHCPCDProperty(newUseLegacyDhcpcd)(ctx)
}

func (h *setDhcpcdVersionHook) tearDown(ctx context.Context, hasError func() bool) error {
	if h.restoreFunc == nil {
		return nil
	}
	return h.restoreFunc(ctx)
}

func setUseLegacyDHCPCDProperty(useLegacyDHCPCD bool) func(context.Context) error {
	return func(ctx context.Context) error {
		shillManager, err := shill.NewManager(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create shill client")
		}
		if err := shillManager.SetProperty(ctx, shillconst.ManagerPropertyUseLegacyDHCPCD, useLegacyDHCPCD); err != nil {
			return errors.Wrap(err, "failed to set shill manager property")
		}
		return nil
	}
}
