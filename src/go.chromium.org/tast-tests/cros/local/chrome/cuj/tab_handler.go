// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TabConn holds the tab URL and the tab connection.
type TabConn struct {
	URL  string
	Conn *chrome.Conn
}

var urls = []string{
	"https://www.google.com/intl/en/drive/",
	"https://www.google.com/photos/about/",
	"https://news.google.com/?hl=en-US&gl=US&ceid=US:en",
	"https://www.google.com/maps/@37.4150659,-122.0788224,15z",
	"https://docs.google.com/document/",
	"https://docs.google.com/spreadsheets/",
	"https://docs.google.com/presentation/",
	"https://www.youtube.com/",
	"https://www.nytimes.com/",
	"https://www.whitehouse.gov/",
	"https://www.wsj.com/",
	"https://www.newsweek.com/",
	"https://www.washingtonpost.com/",
	"https://www.nbc.com/",
	"https://www.npr.org/",
	"https://www.amazon.com/",
	"https://www.walmart.com/",
	"https://www.target.com/",
	"https://www.facebook.com/",
	"https://bleacherreport.com/",
	"https://chrome.google.com/webstore/category/extensions",
	"https://www.google.com/travel/",
}

// NewTabs generates |numTabs| new tabs by cycling through a predefined
// list of urls, and opening them up sequentially. If |individualWindows|
// is true, each tab will be placed in a separate window. If
// |individualWindows| is false, new tabs will be opened in the first
// browser window that was opened during the test.
func NewTabs(ctx context.Context, cs ash.ConnSource, individualWindows bool, numTabs int) ([]TabConn, error) {
	var tabs []TabConn
	for i := 0; i < numTabs; i++ {
		url := urls[i%len(urls)]
		tab, err := NewTabByURL(ctx, cs, individualWindows, url)
		if err != nil {
			return nil, err
		}
		tabs = append(tabs, *tab)
	}
	return tabs, nil
}

// NewTabsByURLs is similar to NewTabs, except it generates new tabs based
// the list of |urls|.
func NewTabsByURLs(ctx context.Context, cs ash.ConnSource, individualWindows bool, urls []string) ([]TabConn, error) {
	var tabs []TabConn
	for _, url := range urls {
		tab, err := NewTabByURL(ctx, cs, individualWindows, url)
		if err != nil {
			return nil, err
		}
		tabs = append(tabs, *tab)
	}
	return tabs, nil
}

// NewTabByURL generates a new tab for a single URL. If |individualWindow|
// is true, a new window is created for this tab. This function waits for
// the tab to quiesce, but if the tab does not quiesce in the allotted
// time, the error is only logged (not returned).
func NewTabByURL(ctx context.Context, cs ash.ConnSource, individualWindow bool, url string) (*TabConn, error) {
	var conn *chrome.Conn
	var err error
	if individualWindow {
		conn, err = cs.NewConn(ctx, url, browser.WithNewWindow())
	} else {
		conn, err = cs.NewConn(ctx, url)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open %s", url)
	}

	t := &TabConn{
		Conn: conn,
		URL:  url,
	}

	t.WaitForQuiescence(ctx, 20*time.Second)

	return t, nil
}

// WaitForQuiescence waits for the tab to quiesce by timeout.
// This does not return an error, even if waiting times out.
func (t *TabConn) WaitForQuiescence(ctx context.Context, timeout time.Duration) {
	start := time.Now()
	if err := webutil.WaitForQuiescence(ctx, t.Conn, timeout); err != nil {
		testing.ContextLogf(ctx, "Ignoring tab quiesce timeout (%v): %v", timeout, err)
	} else {
		testing.ContextLog(ctx, "Tab quiescence time: ", time.Now().Sub(start))
	}
}

// Reconnect reconnects to the tab.
func (t *TabConn) Reconnect(ctx context.Context, br *browser.Browser) error {
	var err error
	if t.Conn, err = br.NewConnForTarget(ctx, chrome.MatchTargetURL(t.URL)); err != nil {
		return errors.Wrapf(err, "failed to reconnect to target %q", t.URL)
	}

	testing.ContextLogf(ctx, "Target (tab: [%s]) was successfully reconnected", t.URL)
	return nil
}

// IsAlive checks if the tab connection is still alive.
func (t *TabConn) IsAlive(ctx context.Context) (bool, error) {
	var url string
	err := t.Conn.Eval(ctx, "window.location.href", &url)
	if err != nil {
		re := regexp.MustCompile(`rpcc: the connection is closing: session: detach failed for session [0-9A-F]{32}: cdp.Target: DetachFromTarget: rpc error: No session with given id`)
		if re.MatchString(err.Error()) {
			testing.ContextLog(ctx, "Tab has been discarded/killed")
			return false, nil
		}
		return false, err
	}
	testing.ContextLogf(ctx, "The connection is alive for %s", url)
	return true, nil
}
