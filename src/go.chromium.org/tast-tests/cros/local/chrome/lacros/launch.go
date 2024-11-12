// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
//
// The functions in this file assume that Ash-Chrome has been set up for Lacros
// testing. The various Lacros fixtures, e.g. "lacros", take care of this, but
// it can also be done manually by passing a computed list of options to
// chrome.New():
//
//   opts, err := lacrosfixt.NewConfig().Opts()
//   if err != nil {
//      ...
//   }
//   cr, err := chrome.New(ctx, opts...)
//
// See the lacrosfixt package for how to tweak the configuration by passing
// arguments to NewConfig.
//
// See also the browser and browserfixt packages, which provide abstractions for
// writing tests in a browser-generic way so that they can work for both Ash and
// Lacros.

package lacros

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
)

// Setup runs lacros-chrome if indicated by the given browser.Type and returns some objects and interfaces
// useful in tests. If the browser.Type is Lacros, it will return a non-nil Lacros instance or an error.
// If the browser.Type is Ash it will return a nil Lacros instance.
// TODO(crbug.com/1315088): Replace f with just the HasChrome interface.
func Setup(ctx context.Context, f interface{}, bt browser.Type) (*chrome.Chrome, *Lacros, ash.ConnSource, error) {
	if _, ok := f.(chrome.HasChrome); !ok {
		return nil, nil, nil, errors.Errorf("unrecognized FixtValue type: %v", f)
	}
	cr := f.(chrome.HasChrome).Chrome()

	switch bt {
	case browser.TypeAsh:
		return cr, nil, cr, nil
	case browser.TypeLacros:
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to get TestConn")
		}
		l, err := Launch(ctx, tconn)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to launch lacros-chrome")
		}
		return cr, l, l, nil
	default:
		return nil, nil, nil, errors.Errorf("unrecognized Chrome type %s", string(bt))
	}
}

// Launch launches lacros. Note that this function expects lacros to be closed
// as a precondition.
func Launch(ctx context.Context, tconn *chrome.TestConn) (l *Lacros, retErr error) {
	return nil, errors.New("unsupported")
}

// LaunchWithURL launches lacros-chrome and ensures there is one page open
// with the given URL. Note that this function expects lacros to be closed
// as a precondition.
func LaunchWithURL(ctx context.Context, tconn *chrome.TestConn, url string) (*Lacros, *chrome.Conn, error) {
	return nil, nil, errors.New("unsupported")
}
