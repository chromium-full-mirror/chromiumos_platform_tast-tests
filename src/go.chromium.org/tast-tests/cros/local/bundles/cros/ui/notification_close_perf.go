// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/notification"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type notificationCloseTestType int

const (
	clearOneAtATime        notificationCloseTestType = iota // In test, clear notifications one at a time.
	clearOneAtATimeWithARC                                  // In test, clear notifications one at a time, with ARC notification.
	clearAll                                                // In test, clear all notifications at once.
	clearAllWithARC                                         // In test, clear all notifications at once, with ARC notification.
)

type notificationClearTestVal struct {
	testType notificationCloseTestType
	bt       browser.Type
}

func init() {
	// TODO(b/294542935): Re-enable the `clear_all*` variants.
	// TODO(b/294542488): Re-enable the `one_at_a_time*` variants.
	testing.AddTest(&testing.Test{
		Func:         NotificationClosePerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures animation performance of the clear all animation or individual notification deletion in the message center",
		Contacts:     []string{"cros-status-area-eng@google.com", "newcomer@chromium.org", "chromeos-sw-engprod@google.com"},
		BugComponent: "b:1246021", // ChromeOS > Software > System UI Surfaces > Notifications
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			Name: "one_at_a_time",
			Val:  notificationClearTestVal{clearOneAtATime, browser.TypeAsh},
		}, {
			Name:              "one_at_a_time_arc",
			ExtraSoftwareDeps: []string{"arc"},
			Val:               notificationClearTestVal{clearOneAtATimeWithARC, browser.TypeAsh},
		}, {
			Name: "clear_all",
			Val:  notificationClearTestVal{clearAll, browser.TypeAsh},
		}, {
			Name:              "clear_all_arc",
			ExtraSoftwareDeps: []string{"arc"},
			Val:               notificationClearTestVal{clearAllWithARC, browser.TypeAsh},
		}, {
			Name:              "one_at_a_time_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               notificationClearTestVal{clearOneAtATime, browser.TypeLacros},
		}, {
			Name:              "one_at_a_time_arc_lacros",
			ExtraSoftwareDeps: []string{"arc", "lacros"},
			Val:               notificationClearTestVal{clearOneAtATimeWithARC, browser.TypeLacros},
		}, {
			Name:              "clear_all_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               notificationClearTestVal{clearAll, browser.TypeLacros},
		}, {
			Name:              "clear_all_arc_lacros",
			ExtraSoftwareDeps: []string{"arc", "lacros"},
			Val:               notificationClearTestVal{clearAllWithARC, browser.TypeLacros},
		}},
	})
}

func NotificationClosePerf(ctx context.Context, s *testing.State) {
	// Reserve a few seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testType := s.Param().(notificationClearTestVal).testType
	shouldClearOneAtATime := testType == clearOneAtATime || testType == clearOneAtATimeWithARC
	isArc := testType == clearOneAtATimeWithARC || testType == clearAllWithARC

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	var initArcOpt []chrome.Option
	if isArc {
		initArcOpt = []chrome.Option{chrome.ARCEnabled()}
	}

	bt := s.Param().(notificationClearTestVal).bt
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(), initArcOpt...)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	atconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API from ash: ", err)
	}
	btconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API from browser: ", err)
	}

	// Minimize opened windows (if exists) to reduce background noise during the measurement.
	if err := ash.ForEachWindow(ctx, atconn, func(w *ash.Window) error {
		return ash.SetWindowStateAndWait(ctx, atconn, w.ID, ash.WindowStateMinimized)
	}); err != nil {
		s.Fatal("Failed to set window states: ", err)
	}

	var arcclient *notification.ARCClient
	if isArc {
		// Note that ARC uses the test API from ash-chrome to manage notifications.
		arcclient, err = notification.NewARCClient(ctx, atconn, cr, s.OutDir())
		if err != nil {
			s.Fatal("Failed to start ARCClient: ", err)
		}
		defer arcclient.Close(cleanupCtx, atconn)
	}

	automationController := uiauto.New(atconn)
	statusAreaRegex := regexp.MustCompile(ash.StatusAreaClassNameRegex)
	statusArea := nodewith.ClassNameRegex(statusAreaRegex)
	collapseButton := nodewith.ClassName("CollapseButton")

	// Ensure no notifications currently exist.
	if err := ash.CloseNotifications(ctx, atconn); err != nil {
		s.Fatal("Failed to clear all notifications prior to adding notifications")
	}

	var histogramName string
	if shouldClearOneAtATime {
		histogramName = "Ash.Notification.MoveDown.AnimationSmoothness"
	} else {
		histogramName = "Ash.Notification.ClearAllVisible.AnimationSmoothness"
	}

	const (
		uiTimeout = 30 * time.Second
		// Create 12 notifications, 3 groups of 4 types of notifications. This is
		// enough to force notification overflow to happen.
		n = 3
	)

	// Add some notifications so that notification centre shows up when opening
	// Quick Settings.
	notificationTypes := []browser.NotificationType{
		browser.NotificationTypeBasic,
		browser.NotificationTypeImage,
		browser.NotificationTypeProgress,
		browser.NotificationTypeList,
	}

	// Create 12 notifications (3 groups of 4 different notifications) with 3 ARC notifications if applicable,
	// close them all via either the ClearAll button or one at a time, and record performance metrics.
	// Note that ash-chrome (cr and atconn) is passed in to take traces and metrics from ash-chrome.
	if err := perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr.Browser(), uiperf.Run(s, perfutil.RunAndWaitAll(atconn, func(ctx context.Context) error {
		ids := make([]string, n*len(notificationTypes))
		for i := 0; i <= n-1; i++ {
			for idx, t := range notificationTypes {
				if id, err := browser.CreateTestNotification(ctx, btconn, t, fmt.Sprintf("Test%sNotification%d", t, i), "test message"); err != nil {
					s.Fatalf("Failed to create %d-th %s notification: %v", i, t, err)
				} else {
					var index = i*len(notificationTypes) + idx
					ids[index] = id
					// Wait for each notification to post. This is faster than waiting for
					// the final notification at the end, because sometimes posting 12
					// notifications at once can result in a very long wait.
					if _, err := ash.WaitForNotification(ctx, atconn, uiTimeout, ash.WaitTitle(fmt.Sprintf("Test%sNotification%d", t, i))); err != nil {
						s.Fatal("Failed waiting for notification: ", err)
					}
				}
			}

			// Create an ARC notification.
			if isArc {
				if err := arcclient.CreateOrUpdateTestNotification(ctx, atconn, fmt.Sprintf("TestARCNotification%d", i), "test message", fmt.Sprintf("%d", i)); err != nil {
					s.Fatalf("Failed to create %d-th ARC notification: %v", i, err)
				}
			}
		}

		// Open the uber tray, then collapse quick settings which results in an expanded MessageCenter.
		// TODO(b/294542935): We should click on the notification tray specifically, not just the whole
		// status area in general as that's not guaranteed to open the notification bubble.
		if err := uiauto.Combine(
			"open the uber tray, then collapse quick settings",
			automationController.LeftClick(statusArea),
			automationController.WaitUntilExists(collapseButton),
			automationController.LeftClick(collapseButton),
			automationController.WaitForLocation(collapseButton),
		)(ctx); err != nil {
			s.Fatal("Failed to open the uber tray and expand quick settings: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			// GoBigSleepLint: Wait a few seconds, otherwise all notifications will be added and
			// removed very quickly.
			// TODO(crbug/1236150): Replace Sleeps with WaitUntilIdle when implemented.
			if err := testing.Sleep(ctx, 2*time.Second); err != nil {
				return errors.Wrap(err, "failed to wait")
			}

			if shouldClearOneAtATime {
				// Clear the notifications one at a time.
				for i := len(ids) - 1; i >= 0; i-- {
					if err := testing.Poll(ctx, func(ctx context.Context) error {
						if err := browser.ClearNotification(ctx, btconn, ids[i]); err != nil {
							return errors.Wrap(err, "failed to clear notification")
						}
						// GoBigSleepLint: Wait for stabilization / animation completion, otherwise all
						// notification removals will happen unrealistically fast.
						// TODO(crbug/1236150): Replace Sleeps with WaitUntilIdle when implemented.
						if err := testing.Sleep(ctx, time.Second); err != nil {
							return errors.Wrap(err, "failed to wait")
						}

						return nil
					}, &testing.PollOptions{Timeout: uiTimeout}); err != nil {
						return errors.Wrap(err, "failed to wait for clearing the notification")
					}
				}

				if isArc {
					// Clear ARC notifications.
					for i := n - 1; i >= 0; i-- {
						if err := testing.Poll(ctx, func(ctx context.Context) error {
							if err := arcclient.RemoveNotification(ctx, atconn, fmt.Sprintf("%d", i)); err != nil {
								return errors.Wrap(err, "failed to remove notification")
							}
							// GoBigSleepLint: Wait for stabilization / animation completion.
							if err := testing.Sleep(ctx, time.Second); err != nil {
								return errors.Wrap(err, "failed to wait")
							}
							return nil
						}, &testing.PollOptions{Timeout: uiTimeout}); err != nil {
							return errors.Wrap(err, "failed to wait for clearing the notification")
						}
					}

					// While clearing ARC notification, we interact with the testing app and there's a chance that
					// quick settings gets closed because it looses focus. If that's the case, reopen quick settings.
					if err := automationController.Exists(collapseButton)(ctx); err != nil {
						if err := uiauto.Combine(
							"open the uber tray",
							automationController.LeftClick(statusArea),
							automationController.WaitUntilExists(collapseButton),
						)(ctx); err != nil {
							s.Fatal("Failed to open the uber tray: ", err)
						}
					}
				}
			} else {
				// Clear all notifications at once via the ClearAll button.
				if err := uiauto.Combine(
					"click the ClearAll button to close all notifications",
					automationController.LeftClick(nodewith.ClassName("StackingBarLabelButton")),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to collapse the uber tray")
				}

				// GoBigSleepLint: Wait a few seconds for notifications to stabilize.
				// TODO(crbug/1236150): Replace Sleeps with WaitUntilIdle when implemented.
				if err := testing.Sleep(ctx, 3*time.Second); err != nil {
					return errors.Wrap(err, "failed to wait")
				}
			}

			// Expand quick settings back to original state, then close uber tray.
			if err := uiauto.Combine(
				"expand quick settings, then close the uber tray",
				automationController.LeftClick(collapseButton),
				automationController.WaitForLocation(collapseButton),
				automationController.LeftClick(statusArea),
				automationController.WaitUntilGone(collapseButton),
			)(ctx); err != nil {
				s.Fatal("Failed to expand quick settings and close the uber tray: ", err)
			}

			return nil
		}, &testing.PollOptions{Timeout: uiTimeout}); err != nil {
			return errors.Wrap(err, "failed to wait for notification")
		}
		return nil
	},
		histogramName)),
		perfutil.StoreAllWithHeuristics(""),
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}
