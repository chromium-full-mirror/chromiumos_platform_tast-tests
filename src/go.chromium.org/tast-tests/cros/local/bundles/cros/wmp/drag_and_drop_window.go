// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DragAndDropWindow,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that window drag and drop works correctly and smoothly",
		Contacts: []string{
			"chromeos-wm-corexp@google.com",
			"chromeos-sw-engprod@google.com",
			"sophiewen@chromium.org",
		},
		BugComponent: "b:1253115",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func DragAndDropWindow(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(ctx)

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if _, err := filesapp.Launch(ctx, tconn); err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	ac := uiauto.New(tconn)

	oldInfo, err := ac.Info(ctx, nodewith.ClassName("WebAppFrameToolbarView"))
	oldBounds := oldInfo.Location
	start := oldBounds.CenterPoint()
	end := start.Add(coords.NewPoint(100, 100))

	if err := mouse.Drag(tconn, start, end, time.Second)(ctx); err != nil {
		s.Fatal("Failed to drag window: ", err)
	}

	newInfo, err := ac.Info(ctx, nodewith.ClassName("WebAppFrameToolbarView"))
	newBounds := newInfo.Location
	// Window bounds should change after the drap and drop.
	if oldBounds == newBounds {
		s.Fatal("Drag failed: window bounds didn't change")
	}
}
