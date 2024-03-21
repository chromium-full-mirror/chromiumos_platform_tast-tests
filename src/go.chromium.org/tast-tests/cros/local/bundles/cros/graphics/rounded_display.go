// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Radius of each corner of the display panel.
type panelRadii struct {
	topLeft     int
	topRight    int
	bottomLeft  int
	bottomRight int
}

type roundedDisplayTestParams struct {
	// displayRotations indicates the display rotation angles for
	// testing rounded corners.
	displayRotations []display.RotationAngle
	panelRadii       panelRadii
}

const (
	promotedOverlayHistogramName = "Compositing.Display.OverlayProcessorUsingStrategy.NumOverlaysPromoted"
	timeoutAfterRotation         = 20 * time.Second
	expectedOverlayCount         = 3
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
		HardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays(), hwdep.InternalDisplay()),
		Data:         []string{"d-canvas/main.html", "d-canvas/2d.js", "d-canvas/webgl.js"},
		Fixture:      "gpuWatchHangs",
		Params: []testing.Param{{

			ExtraHardwareDeps: hwdep.D(hwdep.Model("bugzzy")),
			Val: roundedDisplayTestParams{
				panelRadii: panelRadii{18, 18, 18, 18},
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
		chrome.ExtraArgs(formatDisplayPropertiesAsFlag(params.panelRadii)))

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	internalInfo, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the internal display info: ", err)
	}

	// Move the mouse to the center of the work area. Otherwise,
	// on some devices, when the window is full screen, the mouse
	// position will make the shelf visible, causing test failure.
	if err := mouse.Move(tconn, internalInfo.WorkArea.CenterPoint(), 0)(ctx); err != nil {
		s.Fatal("Failed to move mouse: ", err)
	}

	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()

	conn, err := cr.NewConn(ctx, srv.URL+"/d-canvas/main.html")
	if err != nil {
		s.Fatal("Failed to load d-canvas/main.html: ", err)
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for d-canvas/main.html to achieve quiescence: ", err)
	}

	if err := uiauto.New(tconn).WaitUntilGone(nodewith.HasClass("ash/message_center/MessagePopup"))(ctx); err != nil {
		s.Fatal("Failed to wait for an absence of popups (such as the one about tablet gestures): ", err)
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get windows: ", err)
	}

	// Verify that there is one window.
	if len(ws) != 1 {
		s.Fatal("Expected 1 window; found ", len(ws))
	}

	wID := ws[0].ID

	// Maximize the chrome window so that fast-ink surface is behind the rounded
	// display masks.
	if err := ash.SetWindowStateAndWait(ctx, tconn, wID, ash.WindowStateFullscreen); err != nil {
		s.Fatalf("Failed to set window state to %v: %v", string(ash.WindowStateFullscreen), err)
	}

	var fastInkAction action.Action
	fastInkAction = action.Sleep(time.Second)

	internalDisplayID := internalInfo.ID

	defer display.SetDisplayRotationSync(cleanupCtx, tconn, internalDisplayID, display.Rotate0)
	for _, displayRotation := range params.displayRotations {
		s.Run(ctx, string(displayRotation), func(ctx context.Context, s *testing.State) {
			if err := display.SetDisplayRotationSync(ctx, tconn, internalDisplayID, displayRotation); err != nil {
				s.Fatal("Failed to rotate display: ", err)
			}

			// TODO(b/281763178): Calculate instant fps and poll on the average.
			// GoBigSleepLint: After rotating the screen, wait for the fps of the
			// animation in the fast-ink surface to become stable (damage per frame
			// becomes stable as well). This will ensure consistent promotion of
			// overlays.
			if err := testing.Sleep(ctx, timeoutAfterRotation); err != nil {
				s.Fatal("Failed to wait a second: ", err)
			}

			hists, err := metrics.Run(ctx, tconn, fastInkAction, promotedOverlayHistogramName)
			if err != nil {
				s.Fatal("Error while recording histogram: ", err)
			}

			hist := hists[0]

			if len(hist.Buckets) == 0 {
				s.Fatal("Got no overlay strategy data")
			}

			for _, bucket := range hist.Buckets {
				// We expect to promote three overlays planes each frame. (one plane
				// renders fast-ink surface and two planes render rounded-display mask
				// textures). Therefore, the bucket ranges should be between 3
				// (inclusive) and 4 (exclusive).
				if bucket.Min != 3 && bucket.Max != 4 {
					s.Fatalf("%d of %d frame(s) promoted %d overlays. Expected to promote %d overlays each frame",
						bucket.Count, hist.TotalCount(), bucket.Min, expectedOverlayCount)

				}
			}
		})
	}
}

func formatDisplayPropertiesAsFlag(radii panelRadii) string {
	return fmt.Sprintf("--display-properties=[{\"rounded-corners\": {\"bottom-left\": %d, \"bottom-right\": %d, \"top-left\": %d,\"top-right\": %d}}]",
		radii.bottomLeft, radii.bottomRight, radii.topLeft, radii.topRight)
}
