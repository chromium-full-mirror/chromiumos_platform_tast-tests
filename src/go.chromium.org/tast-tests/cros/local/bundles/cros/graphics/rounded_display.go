// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/graphics/roundeddisplay"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type roundedDisplayTestParams struct {
	// displayRotations indicates the display rotation angles for
	// testing rounded corners.
	displayRotations []display.RotationAngle
	panelRadii       roundeddisplay.PanelRadii
}

const (
	timeoutAfterRotation = 5 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RoundedDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that rounded display mask textures are using hardware overlay as intended",
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Contacts: []string{
			"chromeos-foundations@google.com",
			"zoraiznaeem@chromium.org",
			"skau@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays(), hwdep.InternalDisplay(), hwdep.Model("bugzzy")),
		Data:         []string{"d-canvas/main.html", "d-canvas/2d.js", "d-canvas/webgl.js"},
		Fixture:      "gpuWatchHangs",
		Params: []testing.Param{{

			ExtraHardwareDeps: hwdep.D(hwdep.Model("bugzzy")),
			Val: roundedDisplayTestParams{
				panelRadii: roundeddisplay.PanelRadii{
					TopLeft:     18,
					TopRight:    18,
					BottomLeft:  18,
					BottomRight: 18,
				},
				displayRotations: []display.RotationAngle{
					display.Rotate0,
					display.Rotate90,
					display.Rotate180,
					display.Rotate270,
				}},
		}},
	})
}

func RoundedDisplay(ctx context.Context, s *testing.State) {
	// TODO(b/281763178): Test for different device scale factors.
	// Reserve ten seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	params := s.Param().(roundedDisplayTestParams)

	// Create a chrome instance and enable rounded-display feature. Also specify
	// the radii of display via the command-line flag.
	cr, err := chrome.New(
		ctx, chrome.EnableFeatures("RoundedDisplay"),
		chrome.ExtraArgs(
			roundeddisplay.FormatDisplayPropertiesAsSwitch(params.panelRadii),
			// By only allowing SingleOnTop strategy, we insure that rounded display
			// mask textures are always promoted. See b/331664214.
			"--enable-hardware-overlays=single-on-top",
		))

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Setups a maximized browser window that runs low-latency canvas animation.
	// The canvas will be behind display masks with rounded corners.
	// Since the low-latency canvas texture is promoted to the hardware overlay,
	// the rounded display mask textures will be forced to be promoted to the overlay.
	if err := roundeddisplay.SetupFullscreenLowLatencyCanvas(ctx, s, tconn, cr); err != nil {
		s.Fatal("Failed to setup fullscreen canvas: ", err)
	}

	fastInkAction := action.Sleep(time.Second)

	internalInfo, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the internal display info: ", err)
	}

	internalDisplayID := internalInfo.ID

	defer display.SetDisplayRotationSync(cleanupCtx, tconn, internalDisplayID, display.Rotate0)
	for _, displayRotation := range params.displayRotations {
		s.Run(ctx, string(displayRotation), func(ctx context.Context, s *testing.State) {
			if err := display.SetDisplayRotationSync(ctx, tconn, internalDisplayID, displayRotation); err != nil {
				s.Fatal("Failed to rotate display: ", err)
			}

			// TODO(b/281763178): Calculate instant fps and poll on the average.
			// GoBigSleepLint: After rotating the screen, wait for the fps of the
			// animation in the low-latency canvas to become stable (damage per frame
			// becomes stable as well). This will ensure consistent promotion of
			// overlays.
			if err := testing.Sleep(ctx, timeoutAfterRotation); err != nil {
				s.Fatal("Failed to wait: ", err)
			}

			if err := roundeddisplay.VerifyNumberOfPromotedOverlays(
				ctx, tconn, s, roundeddisplay.ExpectedNumberOfPromotedOverlays, fastInkAction); err != nil {
				s.Fatal("Expected number of overlays not promoted: ", err)
			}
		})
	}
}
