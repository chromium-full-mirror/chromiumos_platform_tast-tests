// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
)

// resetVirtualnetHook implements hook interface.
type resetVirtualnetHook struct {
	noErrorHandlersMixin

	m *shill.Manager
}

func (h *resetVirtualnetHook) name() string {
	return "reset_virtualnet"
}

func (h *resetVirtualnetHook) setUp(ctx context.Context) error {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create shill Manager proxy")
	}

	h.m = m
	if err := virtualnet.ResetEthernetProperties(ctx, h.m); err != nil {
		return errors.Wrap(err, "failed to reset Ethernet properties")
	}

	return nil
}

func (h *resetVirtualnetHook) tearDown(ctx context.Context, hasError func() bool) error {
	if err := virtualnet.ResetEthernetProperties(ctx, h.m); err != nil {
		return errors.Wrap(err, "failed to reset Ethernet properties")
	}
	return nil
}
