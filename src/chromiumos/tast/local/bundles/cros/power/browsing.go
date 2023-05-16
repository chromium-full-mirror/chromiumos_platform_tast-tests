// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/setup"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Browsing,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when browsing",
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome"},
		// Test will run for 1 hour plus 5 minutes buffer time
		Timeout: 65 * time.Minute,
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: "powerAsh",
		}, {
			Name:              "lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

func Browsing(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	r, err := power.NewRecorder(ctx, 20*time.Second, s.OutDir(), s.TestName())
	if err != nil {
		s.Fatal("Cannot create a new Recorder to collect power metrics: ", err)
	}
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Start of main test body.

	// TODO(b/280689994): allow custom config.
	const (
		urlPrefix = "https://storage.googleapis.com/chromiumos-test-assets-public/power_LoadTest/2023-05-02/"
		urlSuffix = ".html"
	)
	var sites = []string{
		"amazon_product", "amazon_search", "apple", "ebay", "google", "office",
		"prime_video", "stackoverflow", "wikipedia", "youtube",
	}

	const (
		loopCount     = 2
		secsPerPage   = 180
		secsPerScroll = 20
	)
	for loop := 0; loop < loopCount; loop++ {
		for _, site := range sites {
			startTime := time.Now()
			url := urlPrefix + site + urlSuffix
			if err := conn.Navigate(ctx, url); err != nil {
				s.Fatal("Failed to navigate: ", err)
			}

			scrollAmount := 600
			for sec := secsPerScroll; sec < secsPerPage; sec += secsPerScroll {
				endTime := startTime.Add(time.Duration(sec) * time.Second)
				// GoBigSleepLint: Sleep to measure power
				if err := testing.Sleep(ctx, time.Until(endTime)); err != nil {
					s.Fatal("Failed to sleep: ", err)
				}

				js := fmt.Sprintf("window.scrollBy(0, %d)", scrollAmount)
				if err := conn.Eval(ctx, js, nil); err != nil {
					s.Fatal("Failed to scroll: ", err)
				}
				scrollAmount = -scrollAmount
			}
			endTime := startTime.Add(time.Duration(secsPerPage) * time.Second)
			// GoBigSleepLint: Sleep to measure power
			if err := testing.Sleep(ctx, time.Until(endTime)); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}

		}
	}
	// End of main test body.

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
