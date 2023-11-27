// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"math"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardFloat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Validity check on floating virtual keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:input-tools-upstream", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		SearchFlags:  util.SearchFlagsWithIMEAndScreenPlay([]ime.InputMethod{ime.EnglishUS}, []string{"screenplay-dbc22988-48b4-4afb-906e-8a3a917925ae"}),
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Params: []testing.Param{
			{
				Name:    "tablet",
				Fixture: fixture.TabletVK,
			},
			{
				Name:    "clamshell",
				Fixture: fixture.ClamshellVK,
			},
		},
	})
}

func VirtualKeyboardFloat(ctx context.Context, s *testing.State) {
	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "setup")

	vkbCtx := vkb.NewContext(cr, tconn)

	if err := vkbCtx.ShowVirtualKeyboard()(ctx); err != nil {
		s.Fatal("Failed to show the virtual keyboard: ", err)
	}

	if err := vkbCtx.SetFloatingMode(uc, true)(ctx); err != nil {
		s.Fatal("Failed to set VK to floating mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := uiauto.Combine("reset VK to docked mode",
			vkbCtx.ShowVirtualKeyboard(),
			vkbCtx.SetFloatingMode(uc, false),
			vkbCtx.HideVirtualKeyboard(),
		)(ctx); err != nil {
			s.Log("Failed to reset VK to docked mode: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, s.TestName())

	validateDragVKAction := func(ctx context.Context) error {
		// Get current center point of drag button.
		dragLoc, err := uiauto.New(tconn).Location(ctx, vkb.DragPointFinder)
		if err != nil {
			return errors.Wrap(err, "failed to find drag point")
		}
		dragPoint := dragLoc.CenterPoint()

		// Drag float vk to new position.
		destinationPoint := coords.NewPoint(dragPoint.X-100, dragPoint.Y-100)
		if err := mouse.Drag(tconn, dragPoint, destinationPoint, time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to drag float window")
		}

		// Get new center point of drag button.
		newDragLoc, err := uiauto.New(tconn).Location(ctx, vkb.DragPointFinder)
		if err != nil {
			return errors.Wrap(err, "failed to find drag point")
		}
		newDragPoint := newDragLoc.CenterPoint()

		// When dragging the virtual keyboard to a given location, the actual location it lands on can be slightly different.
		// e.g. When dragging the virtual keyboard to (1016,762), it can end up at (1015, 762).
		if math.Abs(float64(newDragPoint.X-destinationPoint.X)) > 3 || math.Abs(float64(newDragPoint.Y-destinationPoint.Y)) > 3 {
			return errors.Wrapf(err, "failed to drag float VK or it did not land at desired location. got: %v, want: %v", newDragPoint, destinationPoint)
		}

		// Wait for resize handler to be shown.
		resizeHandleFinder := vkb.NodeFinder.Name("resize keyboard, double tap then drag to resize the keyboard").Role(role.Button)

		// Resizing float vk on some boards are flaky.
		// Thus only check the handler is shown.
		return uiauto.New(tconn).WaitUntilExists(resizeHandleFinder.First())(ctx)
	}

	if err := uiauto.UserAction(
		"Drag floating VK",
		validateDragVKAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureFloatVK,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate dragging floating VK: ", err)
	}
}
