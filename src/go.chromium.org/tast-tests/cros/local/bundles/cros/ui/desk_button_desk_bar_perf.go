// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/perf"
	uiCommon "go.chromium.org/tast-tests/cros/common/ui"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeskButtonDeskBarPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of opening and closing the desk button desk bar",
		Contacts: []string{
			"yongshun@google.com",
			"sammiequon@google.com",
			"dandersson@google.com",
			"chromeos-wm@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		VarDeps:      []string{uiCommon.GaiaPoolDefaultVarName},
		Timeout:      chrome.GAIALoginTimeout + 2*time.Minute,
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

func DeskButtonDeskBarPerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	opts := []chrome.Option{chrome.EnableFeatures("DeskButton"),
		chrome.GAIALoginPool(dma.CredsFromPool(uiCommon.GaiaPoolDefaultVarName))}
	cr, _, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, s.Param().(browser.Type), lacrosfixt.NewConfig(), opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Remove any desks we may have created during the test.
	defer ash.CleanUpDesks(cleanupCtx, tconn)

	const numWindows = 16
	if err := ash.CreateWindows(ctx, tconn, cr, ui.PerftestURL, numWindows-1); err != nil {
		s.Fatal("Failed to open browser windows: ", err)
	}

	runner := perfutil.NewRunner(cr.Browser(), perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})

	if err := runDeskButtonSubtest(ctx, s, tconn, runner, 2 /* desks */); err != nil {
		s.Fatal("Test case with 2 desks failed: ", err)
	}

	if err := runDeskButtonSubtest(ctx, s, tconn, runner, 8 /* desks */); err != nil {
		s.Fatal("Test case with 8 desks failed: ", err)
	}

	if err := runner.Values().Save(ctx, s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}

// runDeskButtonSubtest creates the number of desks needed and then opens and closes
// desk button desk bar multiple times.
func runDeskButtonSubtest(ctx context.Context, s *testing.State,
	tconn *chrome.TestConn, runner *perfutil.Runner, desks int) error {
	for {
		dc, err := ash.GetDeskCount(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to count desks")
		}

		if dc > desks {
			return errors.Wrap(err, "too many desks")
		}

		if dc == desks {
			break
		}

		if err = ash.CreateNewDesk(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create new desk")
		}
	}

	ac := uiauto.New(tconn)
	if err := ac.WithTimeout(3*time.Minute).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for desk bar stabilization")
	}

	name := fmt.Sprintf("%ddesks", desks)
	runner.RunMultiple(ctx, name, uiperf.Run(s,
		perfutil.RunAndWaitAll(tconn,
			func(ctx context.Context) error {
				if err := toggleDeskBar(ctx, ac, true /* visible */); err != nil {
					return err
				}
				if err := toggleDeskBar(ctx, ac, false /* visible */); err != nil {
					return err
				}
				return nil
			},
			"Ash.Desks.DeskButton.DeskBar.Enter.PresentationTime",
			"Ash.Desks.DeskButton.DeskBar.Exit.PresentationTime",
		)),
		perfutil.StoreAll(perf.SmallerIsBetter, "ms", name))

	return nil
}

// toggleDeskBar opens the desk bar.
// This assumes desk button is visible now.
func toggleDeskBar(ctx context.Context, ac *uiauto.Context, visible bool) error {
	deskButton := nodewith.ClassName("DeskButton").Nth(0)
	deskBarView := nodewith.ClassName("DeskBarView").Nth(0)

	var descr string
	actions := []action.Action{
		ac.DoDefault(deskButton),
	}
	if visible {
		actions = append(actions, ac.WaitUntilExists(deskBarView))
		descr = "open the desk bar"
	} else {
		actions = append(actions, ac.WithTimeout(3*time.Minute).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged))
		descr = "close the desk bar"
	}

	if err := uiauto.Combine(descr, actions...)(ctx); err != nil {
		return errors.Wrapf(err, "failed to %v", descr)
	}

	return nil
}
