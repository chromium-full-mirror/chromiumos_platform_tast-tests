// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package browserfixt provides a function for obtaining a Browser instance for
// a given tast fixture and browser type.
package browserfixt

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
)

// SetUp returns a Browser instance for a given browser type and a given existing ash-chrome instance.
// It also returns a closure to be called in order to close the browser instance,
// after which the instance should not be used any further.
func SetUp(ctx context.Context, cr *chrome.Chrome, bt browser.Type) (*browser.Browser, func(ctx context.Context) error, error) {
	switch bt {
	case browser.TypeAsh:
		return cr.Browser(), func(context.Context) error { return nil }, nil
	default:
		return nil, nil, errors.Errorf("unrecognized browser type %s", string(bt))
	}
}
