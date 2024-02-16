// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NotificationPopupPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures animation smoothness of notification popup animations",
		Contacts:     []string{"cros-status-area-eng@google.com", "leandre@chromium.org", "amehfooz@chromium.org", "chromeos-sw-engprod@google.com"},
		BugComponent: "b:1246021", // ChromeOS > Software > System UI Surfaces > Notifications
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
		Timeout: 3 * time.Minute,
	})
}

func NotificationPopupPerf(ctx context.Context, s *testing.State) {
	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Setup a browser.
	bt := s.Param().(browser.Type)
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(ctx)
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatalf("Failed to create Test API connection for %v browser: %v", bt, err)
	}

	// Pre-add some notifications to show remove animation on the first run.
	ids, err := addNotifications(ctx, tconn, bTconn)
	if err != nil {
		s.Fatal("Failed to add notifications: ", err)
	}

	// This includes adding notifications to show popup fade in and move up animation,
	// then remove notification in reverse order (newer then older) to show fade out and move down animation.
	if err := perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr.Browser(), uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
		for _, id := range ids {
			if err := browser.ClearNotification(ctx, bTconn, id); err != nil {
				return errors.Wrapf(err, "failed to clear notification (id: %s): ", id)
			}
		}
		ids = nil

		ids, err = addNotifications(ctx, tconn, bTconn)
		if err != nil {
			return errors.Wrap(err, "failed to add notifications")
		}

		return nil
	},
		"Ash.NotificationPopup.AnimationSmoothness")),
		perfutil.StoreSmoothness,
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}

// addNotifications create some test notifications and return the ids of those notifications
// in reverse order (newer then older).
// tconn is used for ash to wait notifications on the receiver side, while bTconn is for either ash-chrome or lacros-chrome browser to send notifications on the sender side.
func addNotifications(ctx context.Context, tconn *chrome.TestConn, bTconn *browser.TestConn) ([]string, error) {
	var ids []string
	const uiTimeout = 30 * time.Second
	ts := []browser.NotificationType{
		browser.NotificationTypeBasic,
		browser.NotificationTypeImage,
		browser.NotificationTypeProgress,
		browser.NotificationTypeList,
	}
	for _, t := range ts {
		id, err := browser.CreateTestNotification(ctx, bTconn, t, fmt.Sprintf("Test%sNotification", t), "test message")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create %s notification: ", t)
		}
		ids = append([]string{id}, ids...)
	}

	// Wait for the last notification to finish creating.
	if _, err := ash.WaitForNotification(ctx, tconn, uiTimeout, ash.WaitTitle(fmt.Sprintf("Test%sNotification", ts[len(ts)-1]))); err != nil {
		return nil, errors.Wrap(err, "failed to wait for notification")
	}
	return ids, nil
}
