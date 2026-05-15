// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	arca11y "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     AccessibilityTextLocation,
		Desc:     "Checks AutomationNode.boundsForRange API works for Android apps",
		Contacts: []string{"arc-framework+tast@google.com", "hirokisato@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Accessibility
		BugComponent: "b:165222",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithoutUIAutomator",
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
	})
}

func AccessibilityTextLocation(ctx context.Context, s *testing.State) {
	d := s.FixtValue().(*arc.PreData)
	a := d.ARC
	cr := d.Chrome

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	_, cleanup, err := arca11y.SetUpSelectToSpeak(ctx, s, cr, a, tconn)
	if err != nil {
		s.Fatal("Failed to setup SelectToSpeak: ", err)
	}
	defer cleanup(cleanupCtx)

	arca11y.AttachUIFaillog(ctx, s, tconn, "uifaillog")
	arca11y.AttachSystemFaillog(ctx, s, a, "dumpsys")

	cleanupAct, err := arca11y.StartActivityWithSelectToSpeak(ctx, s, a, tconn, arca11y.MainActivity)
	if err != nil {
		s.Fatal("Failed to setup: ", err)
	}
	defer cleanupAct(cleanupCtx)

	ui := uiauto.New(tconn)

	appRoot := nodewith.Name(arca11y.MainActivity.Title).Role(role.Application)

	buttonFinder := nodewith.
		NameRegex(regexp.MustCompile("(CLICK TO SHOW TOAST|Click to show toast)")).
		Role(role.Button).
		Ancestor(appRoot)
	nodeBounds, err := ui.Location(ctx, buttonFinder)
	if err != nil {
		s.Fatal("Failed to get the location: ", err)
	}

	start := 0
	var lastBounds *coords.Rect
	for _, f := range strings.Fields("Click to show toast") {
		end := start + len(f)
		bounds, err := ui.BoundsForRange(ctx, buttonFinder, start, end)
		if err != nil {
			s.Fatalf("Failed to get the range(%d, %d): %v", start, end, err)
		}

		// Quick bounds validations.
		if !nodeBounds.Contains(*bounds) {
			s.Errorf("text bounds should be contained within the node. "+
				"nodeBounds=%v, boundsForRange=%v", nodeBounds, bounds)
		}
		if lastBounds != nil && lastBounds.Right() >= bounds.Left {
			s.Errorf("text bounds should be aligned from left to right without overlap. "+
				"lastBounds=%v, boundsForRange=%v", lastBounds, bounds)
		}

		start = end + 1
		lastBounds = bounds
	}
}
