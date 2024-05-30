// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type pointerTypeParam int

const (
	pointerMouse pointerTypeParam = iota
	pointerTouch
)

type pointerTestParam struct {
	pt pointerTypeParam
	bt browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DragTabInTabletPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the presentation time of dragging a tab in tablet mode",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      8 * time.Minute,
		Params: []testing.Param{{
			Val: pointerTestParam{pt: pointerMouse, bt: browser.TypeAsh},
		}, {
			Name: "touch",
			Val:  pointerTestParam{pt: pointerTouch, bt: browser.TypeAsh},
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               pointerTestParam{pt: pointerMouse, bt: browser.TypeLacros},
		}, {
			Name:              "touch_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               pointerTestParam{pt: pointerTouch, bt: browser.TypeLacros},
		}},
	})
}

func DragTabInTabletPerf(ctx context.Context, s *testing.State) {
	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	// Set up the browser. Open the first window now, then the second one later.
	bt := s.Param().(pointerTestParam).bt
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(),
		chrome.EnableFeatures("WebUITabStrip", "WebUITabStripTabDragIntegration"))
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer cr.Close(cleanupCtx)

	const url = ui.PerftestURL
	conn1, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, url)
	if err != nil {
		s.Fatal("Failed to create new browser window: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn1.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
	if err != nil {
		s.Fatal("Failed to ensure in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Open the second tab that will be dragged.
	conn2, err := br.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to open 2nd tab: ", err)
	}
	defer conn2.Close()

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ac := uiauto.New(tconn)

	var pc pointer.Context
	pt := s.Param().(pointerTestParam).pt
	if pt == pointerTouch {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to set up the touch context: ", err)
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}
	snapRightPoint := coords.NewPoint(info.WorkArea.Right()-1, info.WorkArea.CenterY())
	snapLeftPoint := coords.NewPoint(info.WorkArea.Left+1, info.WorkArea.CenterY())
	workAreaCenterPoint := info.WorkArea.CenterPoint()

	tabStripButton := nodewith.Role(role.Button).ClassName("WebUITabCounterButton").First()
	if err := ac.LeftClick(tabStripButton)(ctx); err != nil {
		s.Fatal("Failed to click the tab strip button")
	}

	firstTab := nodewith.Role(role.Tab).First()
	firstTabLocation, _ := ac.Location(ctx, firstTab)
	tabList := nodewith.Role(role.TabList).First()
	tabListLocation, _ := ac.Location(ctx, tabList)
	if err := perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr.Browser(), uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
		if err := uiauto.Combine("drag and move a tab",
			// Drag the first tab in the tab strip around work area, then snap back to the tab strip.
			pc.Drag(firstTabLocation.CenterPoint(),
				uiauto.Sleep(time.Second),
				pc.DragTo(snapRightPoint, 3*time.Second),
				pc.DragTo(snapLeftPoint, 3*time.Second),
				pc.DragTo(workAreaCenterPoint, 3*time.Second),
				pc.DragTo(tabListLocation.CenterPoint(), 3*time.Second)),
			// Sleep to ensure that the next run performs correctly.
			uiauto.Sleep(time.Second),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to drag the tab")
		}
		return nil
	},
		"Ash.TabDrag.PresentationTime.TabletMode",
		"Ash.TabDrag.PresentationTime.MaxLatency.TabletMode")),
		perfutil.StoreLatency,
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}
