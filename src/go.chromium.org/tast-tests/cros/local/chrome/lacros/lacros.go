// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/cdputil"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
	"go.chromium.org/tast-tests/cros/local/chrome/jslog"
	"go.chromium.org/tast/core/errors"
)

// Lacros contains all state associated with a lacros-chrome instance
// that has been launched. Must call Close() to release resources.
type Lacros struct {
	agg    *jslog.Aggregator
	sess   *driver.Session  // Debug session connected lacros-chrome.
	ctconn *chrome.TestConn // Ash TestConn.
}

// Browser returns a Browser instance.
func (l *Lacros) Browser() *browser.Browser {
	return browser.New(l.sess, false)
}

// CloseResources closes lacros resources without closing targets.
// TODO(crbug.com/1318180): Instead we may want to change Lacros to use ResetState and Close fn
// like Chrome, or provide these functions on the Browser().
func (l *Lacros) CloseResources(ctx context.Context) {
	l.sess.Close(ctx)
	l.sess = nil
	l.agg.Close()
	l.agg = nil
}

// Close closes all lacros chrome targets and the dev session.
func (l *Lacros) Close(ctx context.Context) (retErr error) {
	return errors.New("unsupported")
}

// NewConn creates a new Chrome renderer and returns a connection to it.
// If url is empty, an empty page (about:blank) is opened. Otherwise, the page
// from the specified URL is opened. You can assume that the page loading has
// been finished when this function returns.
// This must not be called after Close().
func (l *Lacros) NewConn(ctx context.Context, url string, opts ...cdputil.CreateTargetOption) (*chrome.Conn, error) {
	return l.sess.NewConn(ctx, url, opts...)
}

// TestAPIConn returns a new chrome.TestConn instance for the lacros browser.
// This must not be called after Close().
func (l *Lacros) TestAPIConn(ctx context.Context) (*chrome.TestConn, error) {
	return l.Browser().TestAPIConn(ctx)
}
