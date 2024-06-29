// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testhooks

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
)

// togglePortalDetectionHook implements hook interface.
type togglePortalDetectionHook struct {
	noErrorHandlersMixin

	enable bool

	restoreFunc func(ctx context.Context)
}

func (h *togglePortalDetectionHook) name() string {
	if h.enable {
		return "enable_portal_detection"
	}
	return "disable_portal_detection"
}

func (h *togglePortalDetectionHook) setUp(ctx context.Context) error {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create shill Manager proxy")
	}

	if h.enable {
		h.restoreFunc, err = m.EnablePortalDetectionWithRestore(ctx)
	} else {
		h.restoreFunc, err = m.DisablePortalDetectionWithRestore(ctx)
	}
	if err != nil {
		return errors.Wrap(err, "failed to toggle portal detection")
	}

	return nil
}

func (h *togglePortalDetectionHook) tearDown(ctx context.Context, hasError func() bool) error {
	if h.restoreFunc != nil {
		h.restoreFunc(ctx)
	}
	return nil
}
