// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UnlockPerf,
		Desc: "Measures animation smoothness of screen unlock",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"oshima@chromium.org",
		},
		// ChromeOS > Software > Performance > TPS
		BugComponent: "b:1045832",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
		}, {
			Name:    "passthrough",
			Fixture: "chromeLoggedInWith100FakeAppsPassthroughCmdDecoder",
		}},
		Data: []string{"animation.html", "animation.js"},
	})
}

func UnlockPerf(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	const (
		password    = "testpass"
		lockTimeout = 30 * time.Second
		authTimeout = 30 * time.Second
	)

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed creating virtual keyboard: ", err)
	}

	defer kb.Close(ctx)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	originalTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the tablet mode status: ", err)
	}
	defer ash.SetTabletModeEnabled(ctx, tconn, originalTabletMode)

	// Run an http server to serve the test contents for accessing from the chrome browsers.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	url := server.URL + "/animation.html"

	r := perfutil.NewRunner(cr, perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})
	currentWindows := 0
	// Run the unlock flow for various situations.
	// - change the number of browser windows, 2 or 8
	// - the window system status; clamshell mode or tablet mode.
	for _, windows := range []int{2, 8} {
		if err := ash.CreateWindows(ctx, tconn, cr, url, windows-currentWindows); err != nil {
			s.Fatal("Failed to create browser windows: ", err)
		}

		currentWindows = windows

		for _, inTabletMode := range []bool{false, true} {
			if err = ash.SetTabletModeEnabled(ctx, tconn, inTabletMode); err != nil {
				s.Fatalf("Failed to set tablet mode %v: %v", inTabletMode, err)
			}

			if err = ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
				return ash.WaitWindowFinishAnimating(ctx, tconn, w.ID)
			}); err != nil {
				s.Fatal("Failed to wait: ", err)
			}

			var suffix string
			if inTabletMode {
				suffix = ".TabletMode"
			} else {
				suffix = ".ClamshellMode"
			}

			r.RunMultiple(ctx, fmt.Sprintf("%dwindows%s", currentWindows, suffix), uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
				// Lock screen
				const accel = "Search+L"
				if err := kb.Accel(ctx, accel); err != nil {
					s.Fatalf("Typing %v failed: %v", accel, err)
				}
				if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, lockTimeout); err != nil {
					s.Fatalf("Waiting for screen to be locked failed: %v (last status %+v)", err, st)
				}

				if err := kb.Type(ctx, password+"\n"); err != nil {
					s.Fatal("Typing correct password failed: ", err)
				}

				if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return !st.Locked }, authTimeout); err != nil {
					s.Fatalf("Waiting for screen to be unlocked failed: %v (last status %+v)", err, st)
				}

				return nil
			},
				"Ash.UnlockAnimation.Smoothness"+suffix)),
				perfutil.StoreAll(perf.BiggerIsBetter, "percent", fmt.Sprintf("%dwindows", currentWindows)))
		}
	}

	if err := r.Values().Save(ctx, s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
