// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/assistant/assistantutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/cpu"
	uiconsts "go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CardElementAnimationPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures animation smoothness of card elements and transition from peeking to half height",
		Contacts: []string{
			"cros-assistive@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:crosbolt",
			"crosbolt_perbuild",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "assistantPerf",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "assistantLacrosPerf",
		}},
	})
}

// CardElementAnimationPerf measures the animation performance of
// animating card elements in and out of the assistant frame. It also measures
// the performance of expanding the launcher from peeking to half height when
// a card is displayed.
func CardElementAnimationPerf(ctx context.Context, s *testing.State) {
	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)

	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome
	accel, err := assistant.ResolveAssistantHotkey(s.Features(""))
	if err != nil {
		s.Fatal("Failed to resolve assistant hotkey: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	if err := assistant.SetBetterOnboardingEnabled(ctx, tconn, false); err != nil {
		s.Fatal("Failed to disable better onboarding: ", err)
	}

	ui := uiauto.New(tconn)
	pv := perf.NewValues()
	var br *browser.Browser
	for nWindows := 0; nWindows < 3; nWindows++ {
		// Open the browser windows. Also set up the browser the first time it opens.
		if nWindows == 1 {
			var conn *chrome.Conn
			var closeBrowser uiauto.Action
			var err error
			conn, br, closeBrowser, err = browserfixt.SetUpWithURL(ctx, cr, bt, uiconsts.PerftestURL)
			if err != nil {
				s.Fatal("Failed to create new browser window: ", err)
			}
			conn.Close()
			defer closeBrowser(cleanupCtx)
		} else if nWindows > 1 {
			if err := ash.CreateWindows(ctx, tconn, br, uiconsts.PerftestURL, 1); err != nil {
				s.Fatal("Failed to create a new browser window: ", err)
			}
		}

		if err := cpu.WaitUntilIdle(ctx); err != nil {
			s.Error("Failed to wait for system cpu idle: ", err)
		}

		if err := assistant.ToggleUIWithHotkey(ctx, tconn, accel); err != nil {
			s.Fatal("Failed opening assistant: ", err)
		}

		histograms, err := metrics.RunAndWaitAll(
			ctx,
			tconn,
			time.Second,
			func(ctx context.Context) error {
				return runCardQueries(ctx, tconn, ui)
			},
			// Card element opacity fade in / out.
			"Ash.Assistant.AnimationSmoothness.CardElement",
		)
		if err != nil {
			s.Fatal("Failed to collect histograms: ", err)
		}
		if err := assistantutils.ProcessHistogram(histograms, pv, nWindows); err != nil {
			s.Fatal("Failed to process histograms: ", err)
		}
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

// runCardQueries performs two card queries in order to test the animate in
// and animate out behavior of the first card.
func runCardQueries(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	for _, q := range []string{"Mount Everest", "Weather"} {
		if err := runCardQuery(ctx, tconn, ui, q); err != nil {
			return err
		}
	}

	return nil
}

// runCardQuery is a helper function for running an Assistant query and waiting
// for a card result.
func runCardQuery(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, query string) error {
	if _, err := assistant.SendTextQuery(ctx, tconn, query); err != nil {
		return errors.Wrapf(err, "could not send query: %s", query)
	}

	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(nodewith.ClassName("AssistantCardElementView"))(ctx); err != nil {
		return errors.Wrapf(err, "query results not shown for query %s within timeout", query)
	}

	return nil
}
