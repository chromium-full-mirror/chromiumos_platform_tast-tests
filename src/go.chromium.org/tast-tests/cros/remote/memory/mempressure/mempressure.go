// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mempressure

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// This package depends on the following services.
// "tast.cros.browser.ChromeService", "tast.cros.browser.LacrosService", "tast.cros.ui.ConnService", "tast.cros.ui.TconnService"

// tabURLs is a list of URLs to visit in the test.
var tabURLs = []string{
	// Start with a few chapters of War And Peace.
	"https://docs.google.com/document/d/19R_RWgGAqcHtgXic_YPQho7EwZyUAuUZyBq4n_V-BJ0/edit?usp=sharing",
	// And a spreadsheet.
	"https://docs.google.com/spreadsheets/d/1oLBzYb41xxXtn5yoeaqACJ6t2bb3IIN03ug8U7e6uBo/edit?usp=sharing",
	"https://www.google.com/intl/en/drive/",
	"https://www.google.com/photos/about/",
	"https://news.google.com/?hl=en-US&gl=US&ceid=US:en",
	"https://plus.google.com/discover",
	"https://www.google.com/maps/@37.4150659,-122.0788224,15z",
	"https://play.google.com/store",
	"https://play.google.com/music/listen",
	"https://www.youtube.com/",
	"https://www.nytimes.com/",
	"https://www.whitehouse.gov/",
	"https://www.wsj.com/",
	"https://www.newsweek.com/",
	"https://www.washingtonpost.com/",
	"https://www.foxnews.com/",
	"https://www.nbc.com/",
	"https://www.npr.org/",
	"https://www.amazon.com/",
	"https://www.walmart.com/",
	"https://www.target.com/",
	"https://www.facebook.com/",
	"https://www.cnn.com/",
	"https://www.cnn.com/us",
	"https://www.cnn.com/world",
	"https://www.cnn.com/politics",
	"https://www.cnn.com/business",
	"https://www.cnn.com/opinions",
	"https://www.cnn.com/health",
	"https://www.cnn.com/entertainment",
	"https://www.cnn.com/business/tech",
	"https://www.cnn.com/travel",
	"https://www.cnn.com/style",
	"https://bleacherreport.com/",
	"https://chrome.google.com/webstore/category/extensions",
}

var dataTabURL string = `data:text/html,<html><body><h1>Hello, World!</h1></body></html>`

const (
	tabLoadTimeout       = 20 * time.Second
	initialTabCount      = 10
	cycleTabInterval     = 10
	defaultRenderTimeout = 30 * time.Second
)

// RemoteMemoryPressure represents a memory pressure task
type RemoteMemoryPressure struct {
	lacros    bool
	lastIndex int
	conn      ui.ConnServiceClient
	tconn     ui.TconnServiceClient
	tabs      []*remoteTab
}

// remoteTab represents the
type remoteTab struct {
	id       uint32
	tabID    int
	targetID string
	lacros   bool
	pinned   bool
	closed   bool
	conn     ui.ConnServiceClient
}

// wiggle scrolls the main window down in short steps, then jumps back up.
// If the main window is not scrollable, it does nothing.
func (t *remoteTab) wiggle(ctx context.Context) error {
	const (
		scrollCount  = 50
		scrollAmount = 100
	)

	for i := 0; i < scrollCount; i++ {
		if _, err := t.conn.Call(ctx, &ui.ConnCallRequest{
			Id:   t.id,
			Fn:   `(dy) => window.scrollBy(0, dy)`,
			Args: []*structpb.Value{value(scrollAmount)},
		}); err != nil {
			return errors.Wrap(err, "scroll down failed")
		}
	}
	if _, err := t.conn.Call(ctx, &ui.ConnCallRequest{
		Id:   t.id,
		Fn:   `(dy) => window.scrollBy(0, dy)`,
		Args: []*structpb.Value{value(-scrollAmount * scrollCount)},
	}); err != nil {
		return errors.Wrap(err, "scroll up failed")
	}
	return nil
}

func value(val interface{}) *structpb.Value {
	result, err := structpb.NewValue(val)
	if err != nil {
		return structpb.NewNullValue()
	}
	return result
}

// waitForQuiescence is copied from webutil.WaitForQuiescence.
func (t *remoteTab) waitForQuiescence(ctx context.Context, timeout time.Duration) error {
	_, err := t.conn.WaitForExpr(ctx, &ui.ConnWaitForExprRequest{
		Id: t.id,
		Expr: ` (function() {
			if (document.readyState !== 'complete') {
				return false;
			}

			const QUIESCENCE_TIMEOUT_MS = 2000;
			let lastEventTime = performance.timing.loadEventEnd -
					performance.timing.navigationStart;
			const resourceTimings = performance.getEntriesByType('resource');
			lastEventTime = resourceTimings.reduce(
					(current, timing) => Math.max(current, timing.responseEnd),
					lastEventTime);
			return performance.now() >= lastEventTime + QUIESCENCE_TIMEOUT_MS;
		})()`,
		TimeoutSecs: uint32(timeout.Seconds()),
		FailOnErr:   true,
	})
	return err
}

// waitForRender is copied from webutil.WaitForRender but update to use WaitForExpr.
// waitForRender waits for the tab connected to the given chrome.Conn is rendered.
func (t *remoteTab) waitForRender(ctx context.Context, timeout time.Duration) error {
	// We wait for two calls to requestAnimationFrame. When the first
	// requestAnimationFrame is called, we know that a frame is in the
	// pipeline. When the second requestAnimationFrame is called, we know that
	// the first frame has reached the screen.
	_, err := t.conn.WaitForExpr(ctx, &ui.ConnWaitForExprRequest{
		Id: t.id,
		Expr: `(async () => {
			await new Promise(window.requestAnimationFrame);
			await new Promise(window.requestAnimationFrame);
		})()`,
		TimeoutSecs: uint32(timeout.Seconds()),
		FailOnErr:   true,
	})
	return err
}

func (m *RemoteMemoryPressure) newTab(ctx context.Context, url string) (*remoteTab, error) {
	res, err := m.conn.NewConn(ctx, &ui.NewConnRequest{Url: url, CallOnLacros: m.lacros})
	if err != nil {
		return nil, errors.Wrap(err, "failed to open new Tab")
	}
	tab := &remoteTab{id: res.Id, targetID: res.TargetId, lacros: m.lacros, conn: m.conn}

	tab.tabID, err = m.getCurrentActiveTabID(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current tab ID")
	}

	if err := tab.waitForQuiescence(ctx, tabLoadTimeout); err != nil {
		return nil, errors.Wrap(err, "failed to wait for quiesence")
	}
	tab.wiggle(ctx)
	m.tabs = append(m.tabs, tab)
	return tab, nil
}

func (m *RemoteMemoryPressure) pinTab(ctx context.Context, tab *remoteTab) error {
	if _, err := m.tconn.Call(ctx, &ui.CallRequest{
		Fn:           `(id) => tast.promisify(chrome.tabs.update)(id, {pinned: true})`,
		Args:         []*structpb.Value{value(tab.tabID)},
		CallOnLacros: m.lacros}); err != nil {
		return errors.Wrap(err, "failed to pin a tab")
	}
	tab.pinned = true
	return nil
}

func (t *remoteTab) disconnect(ctx context.Context) error {
	if t.closed {
		return nil
	}
	if _, err := t.conn.Close(ctx, &ui.CloseRequest{Id: t.id}); err != nil {
		return errors.Wrap(err, "failed to disconnect a tab")
	}
	t.closed = true
	return nil
}

func (t *remoteTab) reconnect(ctx context.Context) error {
	if !t.closed {
		return nil
	}
	res, err := t.conn.NewConnForTarget(ctx, &ui.NewConnForTargetRequest{
		CallOnLacros: t.lacros,
		TargetId:     t.targetID,
	})
	if err != nil {
		return errors.Wrap(err, "failed to call NewConnForTarget")
	}
	t.id = res.Id
	t.closed = false
	return nil
}

func (t *remoteTab) activate(ctx context.Context) error {
	if t.closed {
		if err := t.reconnect(ctx); err != nil {
			return errors.Wrap(err, "failed to reconnect to the tab")
		}
	}
	if _, err := t.conn.ActivateTarget(ctx, &ui.ActivateTargetRequest{Id: t.id}); err != nil {
		return errors.Wrap(err, "failed to activate tab")
	}
	if err := t.waitForRender(ctx, defaultRenderTimeout); err != nil {
		return errors.Wrap(err, "failed to wait for rendering")
	}
	if err := t.waitForQuiescence(ctx, tabLoadTimeout); err != nil {
		return errors.Wrap(err, "failed to wait for quiesence")
	}
	t.wiggle(ctx)
	return nil
}

// queryTestConnTabIDs queries the tab IDs which is used for TconnService.
func (m *RemoteMemoryPressure) queryTestConnTabIDs(ctx context.Context, condition string) ([]int, error) {
	res, err := m.tconn.Call(ctx, &ui.CallRequest{
		Fn: `async () => {
			let tabs = await tast.promisify(chrome.tabs.query)({` + condition + `});
			return tabs.map((tab) => tab.id);
		  }`,
		Args:         []*structpb.Value{},
		CallOnLacros: m.lacros,
	})
	if err != nil {
		return nil, errors.Wrap(err, "cannot query tab list")
	}
	ids := res.AsInterface().([]interface{})
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id.(float64))
	}
	return out, nil
}

// getNonDiscardedTabIDs returns a list of non-discarded tab IDs.
func (m *RemoteMemoryPressure) getNonDiscardedTabIDs(ctx context.Context) ([]int, error) {
	return m.queryTestConnTabIDs(ctx, `discarded: false`)
}

// getDiscardedTabIDs returns a list of discarded tab IDs.
func (m *RemoteMemoryPressure) getDiscardedTabIDs(ctx context.Context) ([]int, error) {
	return m.queryTestConnTabIDs(ctx, `discarded: true`)
}

// getAllTabIDs returns a list of all tab IDs.
func (m *RemoteMemoryPressure) getAllTabIDs(ctx context.Context) ([]int, error) {
	return m.queryTestConnTabIDs(ctx, ``)
}

// getCurrentActiveTabID returns the current active tab ID.
func (m *RemoteMemoryPressure) getCurrentActiveTabID(ctx context.Context) (int, error) {
	out, err := m.queryTestConnTabIDs(ctx, `active: true, currentWindow: true`)
	if err != nil {
		return 0, err
	}
	if len(out) != 1 {
		return 0, errors.New("failed to get single current active tab ID")
	}
	return out[0], nil
}

// CycleTabs cyclically activates the pinned tabs.
func (m *RemoteMemoryPressure) CycleTabs(ctx context.Context, cycleCount int) error {
	alives, err := m.getNonDiscardedTabIDs(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get discardeds tab IDs")
	}
	isDiscarded := func(id int) bool {
		for _, i := range alives {
			if id == i {
				return false
			}
		}
		return true
	}
	count := 0
	for {
		for _, t := range m.tabs {
			if isDiscarded(t.tabID) || !t.pinned {
				continue
			}
			if err := t.activate(ctx); err != nil {
				return err
			}
			count++
			if count == cycleCount {
				return nil
			}
		}
	}
}

// ReactivateLastTab activates last opened tab.
func (m *RemoteMemoryPressure) ReactivateLastTab(ctx context.Context) error {
	id := len(m.tabs) - 1
	if m.tabs[id].closed {
		return errors.New("failed to activate discarded tab")
	}
	return m.tabs[id].activate(ctx)
}

// AddTab adds a tab.
func (m *RemoteMemoryPressure) AddTab(ctx context.Context) error {
	urlIndex := (m.lastIndex + 1) % len(tabURLs)
	t, err := m.newTab(ctx, tabURLs[urlIndex])
	if err != nil {
		return errors.Wrap(err, "failed to add new tab")
	}
	// Leaving the debug connection for a target attached results in
	// PageDiscardingHelper treating the tab as protected. So we close the
	// connection after verifying that the allocation complete.
	t.disconnect(ctx)
	m.lastIndex = urlIndex
	return nil
}

// OpenedTabs returns the number of opened tabs.
func (m *RemoteMemoryPressure) OpenedTabs() int {
	return len(m.tabs)
}

// DiscardedTabs returns the number of discarded tabs.
func (m *RemoteMemoryPressure) DiscardedTabs(ctx context.Context) int {
	ids, err := m.getDiscardedTabIDs(ctx)
	if err != nil {
		return -1
	}
	return len(ids)
}

// Disconnect closes conns
func (m *RemoteMemoryPressure) Disconnect(ctx context.Context) {
	m.conn.CloseAll(ctx, &emptypb.Empty{})
	for _, t := range m.tabs {
		t.closed = true
	}
}

func contains(slice []int, target int) bool {
	for _, value := range slice {
		if value == target {
			return true
		}
	}
	return false
}

// Reconnect reconnects to the running mempressure browser session.
// Note that you need to call ConnectTestEnv() or equivalent function before calling this.
func (m *RemoteMemoryPressure) Reconnect(ctx context.Context, cc grpc.ClientConnInterface) error {
	m.conn = ui.NewConnServiceClient(cc)
	m.tconn = ui.NewTconnServiceClient(cc)

	testing.ContextLog(ctx, "Check test connection : DiscardedTabs ", m.DiscardedTabs(ctx))
	aliveTabs, err := m.getNonDiscardedTabIDs(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get non discarded tabs")
	}

	for _, tab := range m.tabs {
		if !contains(aliveTabs, tab.tabID) {
			testing.ContextLog(ctx, "Skipping discarded tab: ", tab.targetID)
			continue
		}
		// Update tab's conn because ClientConnInterface can be updated.
		tab.conn = m.conn
		if err := tab.reconnect(ctx); err != nil {
			testing.ContextLog(ctx, "Skipping lost tab: ", tab.targetID, err)
		}
	}
	return nil
}

// Run runs a basic memory pressure loop.
// This opens tabs at least minTab, and until it reaches maxTab or
// a tab discard happens. Also, if cycleTab is not 0, it activates
// initial tabs every time cycleTab tabs are opened.
func (m *RemoteMemoryPressure) Run(ctx context.Context, maxTab, minTab, cycleTab int) error {
	for {
		if maxTab != 0 && m.OpenedTabs() == maxTab {
			return nil
		}
		if minTab < m.OpenedTabs() && m.DiscardedTabs(ctx) != 0 {
			return nil
		}
		if err := m.AddTab(ctx); err != nil {
			return errors.Wrap(err, "failed to add a tab ")
		}
		if cycleTab > 0 && m.OpenedTabs()%cycleTab == 0 {
			m.CycleTabs(ctx, initialTabCount)
		}
	}
}

// OpenCycleTabs opens tabs for CycleTabs
func (m *RemoteMemoryPressure) OpenCycleTabs(ctx context.Context) error {
	for i := 0; i < initialTabCount; i++ {
		t, err := m.newTab(ctx, dataTabURL)
		if err != nil {
			return errors.Wrap(err, "failed to open a tab")
		}
		if err := m.pinTab(ctx, t); err != nil {
			return errors.Wrap(err, "failed to pin a tab")
		}
		// Leaving the debug connection for a target attached results in
		// PageDiscardingHelper treating the tab as protected. So we keep
		// the connection for measuring the tab switching time later.
	}
	return nil
}

// NewRemoteMemoryPressure makes a new RemoteMemoryPressure.
// This will be used from the remote tests to add a memory pressure on
// those test. So before running the test, instead of user login, run
// this mempressure as;
//
//	// Login as a test user (you can implement your login function)
//	if err := mempressure.NewTestEnv(ctx, cl.Conn, arc, lacros); err != nil {
//		s.Fatal("Failed to initialize test environment: ", err)
//	}
//	mp, err := mempressure.NewRemoteMemoryPressure(ctx, cl.Conn, lacros)
//	if err != nil {
//		s.Fatal("Failed to make a nre RemoteMemoryPressure: ", err)
//	}
//	if err := mp.Run(ctx, max, min, cycle); err != nil {
//		s.Fatal("Failed to run RemoteMemoryPressure: ", err)
//	}
//
// This opens tabs until a tab is discarded or reaches forceTab.
func NewRemoteMemoryPressure(ctx context.Context, cc grpc.ClientConnInterface, lacros bool) (*RemoteMemoryPressure, error) {
	m := &RemoteMemoryPressure{
		lacros:    lacros,
		conn:      ui.NewConnServiceClient(cc),
		tconn:     ui.NewTconnServiceClient(cc),
		lastIndex: 0,
	}

	if err := m.OpenCycleTabs(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// NewTestEnv login with fake test account.
func NewTestEnv(ctx context.Context, cc grpc.ClientConnInterface, arc, lacros bool) error {
	crs := ui.NewChromeServiceClient(cc)
	req := &ui.NewRequest{}
	if arc {
		req.ArcMode = ui.ArcMode_ARC_MODE_ENABLED
	}
	if lacros {
		req.Lacros = &ui.Lacros{}
	}
	// Fake login on the DUT through the chrome service.
	if _, err := crs.New(ctx, req, grpc.WaitForReady(true)); err != nil {
		return errors.Wrap(err, "failed to login on the DUT ")
	}
	if lacros {
		las := ui.NewLacrosServiceClient(cc)
		if _, err := las.Launch(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrap(err, "failed to launch lacros ")
		}
	}
	return nil
}

// ConnectTestEnv connects to running test account.
func ConnectTestEnv(ctx context.Context, cc grpc.ClientConnInterface, arc, lacros bool) error {
	crs := ui.NewChromeServiceClient(cc)
	req := &ui.NewRequest{
		KeepState:       true,
		TryReuseSession: true,
	}
	if arc {
		req.ArcMode = ui.ArcMode_ARC_MODE_ENABLED
	}
	if lacros {
		req.Lacros = &ui.Lacros{}
	}
	// Fake login on the DUT through the chrome service.
	if _, err := crs.New(ctx, req, grpc.WaitForReady(true)); err != nil {
		return errors.Wrap(err, "failed to connect to login session on the DUT ")
	}
	if lacros {
		las := ui.NewLacrosServiceClient(cc)
		if _, err := las.Connect(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrap(err, "failed to connect lacros ")
		}
	}
	return nil
}
