// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package browser implements a layer of abstraction over Ash and Lacros Chrome
// instances.
package browser

import (
	"context"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"go.chromium.org/tast-tests/cros/local/chrome/internal/cdputil"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Browser consists primarily of a Chrome session.
type Browser struct {
	sess                     *driver.Session
	autotestPrivateSupported bool
}

// New creates a new Browser instance from an existing Chrome session.
func New(sess *driver.Session, autotestPrivateSupported bool) *Browser {
	return &Browser{sess, autotestPrivateSupported}
}

// NewTab creates a new Google Chrome tab.
// Lacros-Chrome initially starts with a new tab page (chrome://newtab/).
// If this new tab page exists, use the new tab to navigate to the URL.
func (b *Browser) NewTab(ctx context.Context, url string, opts ...CreateTargetOption) (*driver.Conn, error) {
	targets, err := b.sess.FindTargets(ctx, driver.MatchTargetURL("chrome://newtab/"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find new tab targets")
	}
	if len(targets) != 1 {
		// If there are no new tab or there are multiple new tabs,
		// create a new tab and return.
		return b.sess.NewConn(ctx, url, opts...)
	}

	return b.navigateToURLUsingNewTab(ctx, url)
}

// navigateToURLUsingNewTab finds the new tab and navigate to the given URL.
func (b *Browser) navigateToURLUsingNewTab(ctx context.Context, url string) (*driver.Conn, error) {
	conn, err := b.sess.NewConnForTarget(ctx, driver.MatchTargetURL("chrome://newtab/"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find new tab")
	}
	if err := conn.Navigate(ctx, url); err != nil {
		return nil, errors.Wrapf(err, "failed to navigate to %s, error", url)
	}

	return conn, nil
}

// CreateTargetOption is cpdutil.CreateTargetOption.
type CreateTargetOption = cdputil.CreateTargetOption

// WithNewWindow behaves like cpdutil.WithNewWindow.
func WithNewWindow() CreateTargetOption {
	return cdputil.WithNewWindow()
}

// TraceOption is cpdutil.TraceOption.
type TraceOption = cdputil.TraceOption

// DisableSystrace behaves like cpdutil.DisableSystrace.
func DisableSystrace() TraceOption {
	return cdputil.DisableSystrace()
}

// ReloadActiveTab reloads the active tab.
func (b *Browser) ReloadActiveTab(ctx context.Context) error {
	tconn, err := b.sess.TestAPIConn(ctx, b.autotestPrivateSupported)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	if err := tconn.Eval(ctx, "chrome.tabs.reload()", nil); err != nil {
		return errors.Wrap(err, "failed to reload tab")
	}
	if err := tconn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "failed to wait for the ready state")
	}
	return nil
}

// CloseWithURL finds all targets with the given url, closes them, and waits
// until they are closed. Note that if this closes all lacros pages, lacros will
// exit, and we won't be able to verify closing was done successfully.
// If this turns out to cause flakes, we can additionally poll to see if
// the lacros process still exists, and if it does then poll each target
// to see if it closed.
func (b *Browser) CloseWithURL(ctx context.Context, url string) error {
	targets, err := b.sess.FindTargets(ctx, driver.MatchTargetURL(url))
	if err != nil {
		return errors.Wrap(err, "failed to query for about:blank pages")
	}

	allPages, err := b.sess.FindTargets(ctx, func(t *target.Info) bool { return t.Type == "page" })
	if err != nil {
		return errors.Wrap(err, "failed to query for all pages")
	}

	for _, info := range targets {
		if err := b.sess.CloseTarget(ctx, info.TargetID); err != nil {
			return err
		}
	}

	if len(targets) != len(allPages) {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			targets, err := b.sess.FindTargets(ctx, driver.MatchTargetURL(url))
			if err != nil {
				return testing.PollBreak(err)
			}
			if len(targets) != 0 {
				return errors.New("not all about:blank targets were closed")
			}

			return nil
		}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
			return err
		}
	}

	return nil

}
