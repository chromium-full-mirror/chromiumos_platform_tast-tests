// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskmanager

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/webutil"
	"go.chromium.org/tast/core/errors"
)

// ProcessStatus defines the status of the process.
type ProcessStatus string

const (
	// ProcessAlive represents the status of the alive process.
	ProcessAlive ProcessStatus = "alive"
	// ProcessDead represents the status of the dead process.
	ProcessDead ProcessStatus = "dead"
	// ProcessUnknownStatus represents the unknown status of the process.
	ProcessUnknownStatus ProcessStatus = "unknown"
)

// Process defines the interface for the process.
type Process interface {
	// Open opens the process.
	Open(ctx context.Context, br *browser.Browser) error
	// Close closes the process.
	Close(ctx context.Context) error
	// Status returns the status of the process, e.g., alive, dead, and etc.
	Status(ctx context.Context) (ProcessStatus, error)
	// NameInTaskManager returns the process name displayed in the task manager.
	NameInTaskManager(ctx context.Context) (string, error)
}

// ChromeTab defines the struct for chrome tab.
type ChromeTab struct {
	// URL is the url of the tab.
	URL string

	// ID is the id of the tab.
	ID int `json:"id"`
	// Title is the title of the tab.
	Title string `json:"title"`
	// LoadingStatus is the loading status of the tab.
	LoadingStatus TabStatus `json:"status"`

	// openInNewWindow indicates if the tab will be opened in a new window.
	// This field needs to be set before opening the tab to open the tab in a new window.
	openInNewWindow bool

	// browserType is the browser type of the tab.
	browserType browser.Type
	// bTconn is Test API connection for browser.
	bTconn *chrome.TestConn

	// conn is the connection to the chrome tab.
	conn *chrome.Conn
}

// ChromeTabQuiescenceTimeout defines the maximum time duration to wait for a Chrome tab to achieve quiescence.
const ChromeTabQuiescenceTimeout = time.Minute

// NewChromeTabProcess returns an instance of ChromeTab.
func NewChromeTabProcess(url string, browserType browser.Type) *ChromeTab {
	return &ChromeTab{
		URL:         url,
		browserType: browserType,
	}
}

// SetOpenInNewWindow sets openInNewWindow to be true.
func (tab *ChromeTab) SetOpenInNewWindow() {
	tab.openInNewWindow = true
}

// BrowserType returns browser type of the tab.
func (tab *ChromeTab) BrowserType() browser.Type {
	return tab.browserType
}

// Open opens a new chrome tab in a single browser window.
func (tab *ChromeTab) Open(ctx context.Context, br *browser.Browser) (retErr error) {
	if tab.conn != nil {
		return errors.New("the tab is already opened")
	}

	var opts []browser.CreateTargetOption
	if tab.openInNewWindow {
		opts = append(opts, browser.WithNewWindow())
	}

	var err error
	if tab.conn, err = br.NewConn(ctx, tab.URL, opts...); err != nil {
		return errors.Wrapf(err, "failed to open %s", tab.URL)
	}

	defer func() {
		if retErr != nil {
			tab.Close(ctx)
		}
	}()

	if err := webutil.WaitForQuiescence(ctx, tab.conn, ChromeTabQuiescenceTimeout); err != nil {
		return errors.Wrap(err, "failed to wait for web page to finish loading")
	}

	expr := `async () => {
	    const tabs = await tast.promisify(chrome.tabs.query)({lastFocusedWindow: true, active: true});
		if (tabs.length !== 1) {
			throw new Error("unexpected number of tabs: got " + tabs.length)
		}
	    return tabs[0]
	   }`

	tab.bTconn, err = br.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get Test API connection for %v browser", tab.browserType)
	}

	if err := tab.bTconn.Call(ctx, &tab, expr); err != nil {
		return errors.Wrap(err, "failed to get current tab")
	}

	return nil
}

// Close closes the chrome tab.
func (tab *ChromeTab) Close(ctx context.Context) error {
	if tab.conn == nil {
		return errors.New("the tab is already closed")
	}

	if err := tab.conn.CloseTarget(ctx); err != nil {
		return errors.Wrapf(err, "failed to close the tab %q", tab.Title)
	}
	if err := tab.conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close connection")
	}

	tab.conn = nil
	return nil
}

// TabStatus defines the 3 types of TabStatus in Chrome API.
type TabStatus string

// Define 3 types of TabStatus.
// See: https://developer.chrome.com/docs/extensions/reference/tabs/#type-TabStatus
const (
	TabUnloaded TabStatus = "unloaded"
	TabLoading  TabStatus = "loading"
	TabComplete TabStatus = "complete"
)

// Status returns the ProcessStatus of the chrome tab process.
func (tab *ChromeTab) Status(ctx context.Context) (ProcessStatus, error) {
	if err := tab.UpdateInfo(ctx); err != nil {
		return ProcessUnknownStatus, err
	}

	switch tab.LoadingStatus {
	case TabLoading, TabComplete:
		return ProcessAlive, nil
	case TabUnloaded:
		return ProcessDead, nil
	default:
		return ProcessUnknownStatus, errors.New("unexpected status")
	}
}

// UpdateInfo updates the title and TabStatus of the chrome tab.
func (tab *ChromeTab) UpdateInfo(ctx context.Context) error {
	expr := `async (id) => {
	    const tab = tast.promisify(chrome.tabs.get)(id);
	    return tab
	   }`
	if err := tab.bTconn.Call(ctx, &tab, expr, tab.ID); err != nil {
		return errors.Wrap(err, "failed to query tab")
	}

	return nil
}

// NameInTaskManager returns the process name displayed in the task manager.
func (tab *ChromeTab) NameInTaskManager(ctx context.Context) (string, error) {
	// Tab name might dynamically change.
	// Update the tab information to ensure the latest title returned.
	if err := tab.UpdateInfo(ctx); err != nil {
		return "", errors.Wrap(err, "failed to update tab information")
	}

	name := "Tab: " + tab.Title
	if tab.browserType == browser.TypeLacros {
		name = fmt.Sprintf("Lacros: %s", name)
	}

	return name, nil
}
