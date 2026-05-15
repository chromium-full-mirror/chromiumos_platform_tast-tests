// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"math"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/gio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: InputOverlayReposition,
		// Disabled by TORA. See: b/349631636
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Drag test for GIO menu entry, button group, and actions",
		Contacts:       []string{"arc-app-dev@google.com", "pjlee@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Gaming
		BugComponent: "b:1373988",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithGameDashboard",
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
	})
}

type dragType int

const (
	mouseDrag dragType = 0
	touchDrag dragType = 1
	// errorMargin is the allowable error for drag testing.
	errorMargin = 1.0
)

func InputOverlayReposition(ctx context.Context, s *testing.State) {
	gio.SetupTestApp(ctx, s, func(params gio.TestParams) error {
		// Start up UIAutomator.
		ui := uiauto.New(params.TestConn).WithTimeout(time.Minute)
		// Start up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, params.TestConn)

		gameControlsEdit := nodewith.Name("Edit game controls").HasClass("GameDashboardMainMenuView::GameControlsDetailsRow")
		tapAction := nodewith.Name("Keymapping touch point").Role(role.Group).First()
		moveAction := nodewith.Name("Keymapping D-pad").Role(role.Group)

		if err := uiauto.Combine("drag actions",
			// Open game dashboard.
			kb.AccelAction("Search+g"),
			// Open game controls.
			ui.LeftClickUntil(gameControlsEdit, ui.Gone(gameControlsEdit)),
			// Verify tap action drag works correctly.
			testDrag(tapAction, params.TestConn, mouseDrag, -4, -4),
			ui.FocusAndWait(tapAction),
			testKeyDrag(tapAction, params.TestConn),
			testOffscreenDrag(tapAction, params.TestConn),
			// Verify move action drag works correctly.
			testDrag(moveAction, params.TestConn, mouseDrag, -4, -4),
			ui.FocusAndWait(moveAction),
			testKeyDrag(moveAction, params.TestConn),
			testOffscreenDrag(moveAction, params.TestConn),
		)(ctx); err != nil {
			s.Fatal("Failed to verify action drag correctness: ", err)
		}

		return nil
	})
}

// testDrag drags the given node and verifies that the node has been dragged the
// correct distance.
func testDrag(finder *nodewith.Finder, tconn *chrome.TestConn, drag dragType, xOffset, yOffset int) action.Action {
	return func(ctx context.Context) error {
		// Start up UIAutomator.
		ui := uiauto.New(tconn).WithTimeout(time.Minute)

		// Drag the node.
		if err := performDrag(finder, tconn, drag, xOffset, yOffset)(ctx); err != nil {
			return errors.Wrap(err, "failed to do first reposition")
		}

		interimRect, err := ui.Location(ctx, finder)
		if err != nil {
			return errors.Wrap(err, "could not get interim node position")
		}
		interimLoc := interimRect.CenterPoint()
		finalLoc := coords.NewPoint(interimLoc.X-xOffset/2, interimLoc.Y-yOffset/2)

		if err := performDrag(finder, tconn, drag, -xOffset/2, -yOffset/2)(ctx); err != nil {
			return errors.Wrap(err, "failed to do second reposition")
		}

		// Verify that the node was dragged to the final location.
		currentRect, err := ui.Location(ctx, finder)
		if err != nil {
			return errors.Wrap(err, "could not get initial node position")
		}
		currentLoc := currentRect.CenterPoint()
		if !withinError(currentLoc, finalLoc) {
			return errors.Errorf("wanted final location %v within error margin %v, got %v", finalLoc, errorMargin, currentLoc)
		}

		return nil
	}
}

func performDrag(finder *nodewith.Finder, tconn *chrome.TestConn, drag dragType, xOffset, yOffset int) action.Action {
	return func(ctx context.Context) error {
		// Start up UIAutomator.
		ui := uiauto.New(tconn).WithTimeout(time.Minute)

		// Save initial location of node.
		initialRect, err := ui.Location(ctx, finder)
		if err != nil {
			return errors.Wrap(err, "could not get initial node position")
		}
		initialLoc := initialRect.CenterPoint()
		nextLoc := coords.NewPoint(initialLoc.X+xOffset, initialLoc.Y+yOffset)

		// Create the drag functions based on input type.
		var dragAction action.Action
		// var secondDragAction action.Action
		if drag == mouseDrag {
			dragAction = mouse.Drag(tconn, initialLoc, nextLoc, 5*time.Second)
		} else if drag == touchDrag {
			touch, err := touch.New(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to open new touchscreen")
			}
			dragAction = touch.Swipe(initialLoc, touch.SwipeTo(nextLoc, 5*time.Second), touch.Hold(time.Second))
		}

		if err := dragAction(ctx); err != nil {
			return errors.Wrap(err, "failed to do drag action")
		}
		return nil
	}
}

// testKeyDrag tests reposition using arrow keys.
func testKeyDrag(finder *nodewith.Finder, tconn *chrome.TestConn) action.Action {
	type Check func(initial, final coords.Point) bool
	return func(ctx context.Context) error {
		tests := []struct {
			key       string
			predicate Check
		}{
			{
				key: "Left",
				predicate: Check(func(initial, final coords.Point) bool {
					return initial.X > final.X
				}),
			},
			{
				key: "Right",
				predicate: Check(func(initial, final coords.Point) bool {
					return initial.X < final.X
				}),
			},
			{
				key: "Up",
				predicate: Check(func(initial, final coords.Point) bool {
					return initial.Y > final.Y
				}),
			},
			{
				key: "Down",
				predicate: Check(func(initial, final coords.Point) bool {
					return initial.Y < final.Y
				}),
			},
		}

		// Start up UIAutomator.
		ui := uiauto.New(tconn).WithTimeout(time.Minute)
		// Start up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)

		for _, tc := range tests {
			// Get initial location of node.
			initialRect, err := ui.Location(ctx, finder)
			if err != nil {
				return errors.Wrap(err, "could not get initial node position")
			}
			initialLoc := initialRect.CenterPoint()

			// Type key 3 times.
			if err := kb.Accel(ctx, tc.key); err != nil {
				return errors.Wrapf(err, "failed to press %s key", tc.key)
			}

			// Verify that the node was dragged to the final location.
			currentRect, err := ui.Location(ctx, finder)
			if err != nil {
				return errors.Wrap(err, "could not get initial node position")
			}
			currentLoc := currentRect.CenterPoint()
			if !tc.predicate(initialLoc, currentLoc) {
				return errors.Errorf("failed to verify key press %v", tc.key)
			}
		}

		return nil
	}
}

// testOffscreenDrag attempts to drag the node offscreen to make sure bounds
// are obeyed.
func testOffscreenDrag(finder *nodewith.Finder, tconn *chrome.TestConn) action.Action {
	return func(ctx context.Context) error {
		// Start up UIAutomator.
		ui := uiauto.New(tconn).WithTimeout(time.Minute)
		// Save initial location of node.
		initialRect, err := ui.Location(ctx, finder)
		if err != nil {
			return errors.Wrap(err, "could not get initial node position")
		}
		initialLoc := initialRect.CenterPoint()
		finalLoc := coords.NewPoint(-1, -1)

		// Drag the node.
		if err := mouse.Drag(tconn, initialLoc, finalLoc, 5*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to reposition")
		}

		// Verify that the node was not dragged offscreen.
		currentRect, err := ui.Location(ctx, finder)
		if err != nil {
			return errors.Wrap(err, "could not get initial node position")
		}
		topLeft := currentRect.TopLeft()
		if (topLeft.X < 0) || (topLeft.Y < 0) {
			return errors.New("node dragged offscreen")
		}

		return nil
	}
}

// withinError takes the difference between two points and ensures that it is
// within the allowable error bounds.
func withinError(p1, p2 coords.Point) bool {
	return (math.Abs(float64(p1.X-p2.X)) <= errorMargin) && (math.Abs(float64(p1.Y-p2.Y)) <= errorMargin)
}
