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
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfaillog"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/errors"
)

// SetUp returns a Browser instance for a given browser type and a given existing ash-chrome instance.
// It also returns a closure to be called in order to close the browser instance,
// after which the instance should not be used any further.
func SetUp(ctx context.Context, cr *chrome.Chrome, bt browser.Type) (*browser.Browser, func(ctx context.Context) error, error) {
	switch bt {
	case browser.TypeAsh:
		return cr.Browser(), func(context.Context) error { return nil }, nil
	case browser.TypeLacros:
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to connect to test API")
		}
		l, err := lacros.Launch(ctx, tconn)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to launch lacros-chrome")
		}
		return l.Browser(), l.Close, nil
	default:
		return nil, nil, errors.Errorf("unrecognized browser type %s", string(bt))
	}
}

// SetUpWithURL can be thought of as a combination of SetUp and NewConn that
// avoids the extra default new tab page in the case of Lacros. The caller is
// responsible for closing the returned connection via its Close() method prior
// to calling the returned closure.
func SetUpWithURL(ctx context.Context, cr *chrome.Chrome, bt browser.Type, url string) (*chrome.Conn, *browser.Browser, func(ctx context.Context) error, error) {
	switch bt {
	case browser.TypeAsh:
		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to connect to ash-chrome")
		}
		return conn, cr.Browser(), func(context.Context) error { return nil }, nil

	case browser.TypeLacros:
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to connect to test API")
		}

		l, conn, err := lacros.LaunchWithURL(ctx, tconn, url)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to launch lacros-chrome")
		}
		return conn, l.Browser(), l.Close, nil

	default:
		return nil, nil, nil, errors.Errorf("unrecognized browser type %s", string(bt))
	}
}

// SetUpWithNewChrome returns a new ash-chrome instance and a Browser instance as well.
// This is useful when no fixture is used but the new chrome needs to be instantiated in test for a fresh UI restart between tests.
// It also returns a closure to be called in order to close the browser instance.
// The caller is responsible for calling the closure first, then Close() on the chrome instance for cleanup.
// Note that it opens an extra default tab page for Lacros, but not for ash-chrome.
// TODO(crbug.com/1357886): Fix the behavior inconsistency of SetUp and SetUpWithNewChrome for both browsers.
func SetUpWithNewChrome(ctx context.Context, bt browser.Type, cfg *lacrosfixt.Config, opts ...chrome.Option) (_ *chrome.Chrome, _ *browser.Browser, _ func(ctx context.Context) error, errRet error) {
	switch bt {
	case browser.TypeAsh:
		cr, err := chrome.New(ctx, opts...)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to connect to ash-chrome")
		}
		return cr, cr.Browser(), func(context.Context) error { return nil }, nil

	case browser.TypeLacros:
		lacrosOpts, err := cfg.Opts()
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to get default options")
		}
		opts = append(opts, lacrosOpts...)

		cr, err := chrome.New(ctx, opts...)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to connect to ash-chrome")
		}
		defer func() {
			if errRet != nil {
				cr.Close(ctx)
			}
		}()

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to connect to ash-chrome test API")
		}

		l, err := lacros.Launch(ctx, tconn)
		if err != nil {
			lacrosfaillog.Save(ctx, tconn)
			return nil, nil, nil, errors.Wrap(err, "failed to launch lacros-chrome")
		}
		return cr, l.Browser(), l.Close, nil

	default:
		return nil, nil, nil, errors.Errorf("unrecognized browser type %s", string(bt))
	}
}

// NewChrome is basically SetUpWithNewChrome without the SetUp part.
// It restarts Chrome with, depending on the browser type, either just the
// given opts or the given opts plus those provided by the Lacros
// configuration. This is useful for situations where the browser will be
// launched via some UI interaction, for example.
func NewChrome(ctx context.Context, bt browser.Type, cfg *lacrosfixt.Config, opts ...chrome.Option) (*chrome.Chrome, error) {
	if bt == browser.TypeLacros {
		lacrosOpts, err := cfg.Opts()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get Lacros options")
		}
		opts = append(opts, lacrosOpts...)
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to restart Chrome")
	}
	return cr, nil
}

// Connect connects to a running browser instance. It returns a closure for
// freeing resources when the connection is no longer needed (note that the
// closure does not close the browser).
func Connect(ctx context.Context, cr *chrome.Chrome, bt browser.Type) (*browser.Browser, func(ctx context.Context), error) {
	switch bt {
	case browser.TypeAsh:
		return cr.Browser(), func(context.Context) {}, nil
	case browser.TypeLacros:
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to connect to ash-chrome test API")
		}
		l, err := lacros.Connect(ctx, tconn)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to connect to lacros-chrome")
		}
		cleanUp := func(ctx context.Context) {
			l.CloseResources(ctx) // Ignore error.
		}
		return l.Browser(), cleanUp, nil
	default:
		return nil, nil, errors.Errorf("unrecognized Chrome type %s", string(bt))
	}
}

// ConnectAndOwn is like Connect but the returned closure also closes the browser (just like SetUp's).
func ConnectAndOwn(ctx context.Context, cr *chrome.Chrome, bt browser.Type) (*browser.Browser, func(ctx context.Context), error) {
	switch bt {
	case browser.TypeAsh:
		return cr.Browser(), func(context.Context) {}, nil
	case browser.TypeLacros:
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to connect to ash-chrome test API")
		}
		l, err := lacros.Connect(ctx, tconn)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to connect to lacros-chrome")
		}
		brClose := func(ctx context.Context) {
			l.Close(ctx) // Ignore error.
		}
		return l.Browser(), brClose, nil
	default:
		return nil, nil, errors.Errorf("unrecognized Chrome type %s", string(bt))
	}
}

// SetUpBrowserOrUseCurrent either connects to the existing running browser
// instance or create a new one if it can't connect. Besides the browser itself,
// the return values also includes 2 callbacks, `closeResource` to close the
// tab but not the browser, `closeBrowser` will close the whole browser.
func SetUpBrowserOrUseCurrent(ctx context.Context, cr *chrome.Chrome, bt browser.Type) (*browser.Browser, func(ctx context.Context), func(ctx context.Context) error, error) {
	switch bt {
	case browser.TypeAsh:
		// For Ash, it's okay to call SetUp even if there's an existing running browser.
		br, closeBrowser, err := SetUp(ctx, cr, bt)
		return br, func(ctx context.Context) {}, closeBrowser, err
	case browser.TypeLacros:
		// For Lacros we can't call Connect if there's no running browsers, it will
		// hang there until context deadline exceeded. We need to check first.
		running, err := lacros.IsLacrosRunning(ctx, cr)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to check if Lacros is running or not before trying to connect")
		}
		// Lacros is running.
		if running {
			br, closeResource, err := Connect(ctx, cr, bt)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "failed to connect to the existing running Lacros")
			}
			return br, closeResource, func(ctx context.Context) error { return nil }, err
		}
		// Lacros is not running.
		br, closeBrowser, err := SetUp(ctx, cr, bt)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to set up a new Lacros")
		}
		return br, func(ctx context.Context) {}, closeBrowser, nil
	default:
		return nil, nil, nil, errors.Errorf("unrecognized browser type %s", string(bt))
	}
}

// MaybeConnectBrowserAndClose connects and own the running browser if there's any,
// and then close the whole browser. It's mainly used in the cleanup phase.
func MaybeConnectBrowserAndClose(ctx context.Context, cr *chrome.Chrome, bt browser.Type) error {
	// For Lacros we can't call Connect if there's no running browsers, it will
	// hang there until context deadline exceeded. We need to check first.
	if bt == browser.TypeLacros {
		running, err := lacros.IsLacrosRunning(ctx, cr)
		if err != nil {
			return errors.Wrap(err, "failed to check if Lacros is running or not before trying to connect")
		}
		if !running {
			return nil
		}
	}
	_, closeBrowser, _ := ConnectAndOwn(ctx, cr, bt)
	if closeBrowser != nil {
		closeBrowser(ctx)
	}
	return nil
}
