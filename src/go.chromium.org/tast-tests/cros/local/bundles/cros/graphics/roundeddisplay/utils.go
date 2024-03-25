// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package roundeddisplay contains utility methods to test rounded display.
package roundeddisplay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PanelRadii has information about the radius of each corner of the display panel.
type PanelRadii struct {
	TopLeft     int
	TopRight    int
	BottomLeft  int
	BottomRight int
}

// FormatDisplayPropertiesAsSwitch returns |radii| in the switch format that is used
// to specify the radii of a display.
func FormatDisplayPropertiesAsSwitch(radii PanelRadii) string {
	return fmt.Sprintf("--display-properties=[{\"rounded-corners\": {\"bottom-left\": %d, \"bottom-right\": %d, \"top-left\": %d,\"top-right\": %d}}]",
		radii.BottomLeft, radii.BottomRight, radii.TopLeft, radii.TopRight)
}

// SetupFullscreenLowLatencyCanvas opens a webpage that use low-latency canvas to run an animation. The animation is run on a
// fullscreen chrome browser. It also ensures that textures are promoted to overlay by performing various actions.
func SetupFullscreenLowLatencyCanvas(ctx context.Context, s *testing.State, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()

	internalInfo, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the internal display info")
	}

	// Move the mouse to the center of the work area. Otherwise,
	// on some devices, when the window is full screen, the mouse
	// position will make the shelf visible, causing test failure.
	if err := mouse.Move(tconn, internalInfo.WorkArea.CenterPoint(), 0)(ctx); err != nil {
		return errors.Wrap(err, "failed to move mouse")
	}

	conn, err := cr.NewConn(ctx, srv.URL+"/d-canvas/main.html")
	if err != nil {
		return errors.Wrap(err, "failed to load d-canvas/main.html")
	}

	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for d-canvas/main.html to achieve quiescence")
	}

	if err := uiauto.New(tconn).WaitUntilGone(nodewith.HasClass("ash/message_center/MessagePopup"))(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for an absence of popups (such as the one about tablet gestures)")
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get windows")
	}

	// Verify that there is one window.
	if len(ws) != 1 {
		return errors.Wrapf(err, "expected 1 window; found %d", len(ws))
	}

	wID := ws[0].ID

	if err := ash.SetWindowStateAndWait(ctx, tconn, wID, ash.WindowStateFullscreen); err != nil {
		return errors.Wrapf(err, "failed to set window state to %v", string(ash.WindowStateFullscreen))
	}

	return nil
}

// VerifyNumberOfPromotedOverlays verifies that |count| number of overlays are promoted to overlays.
// Since this function gets the information from a histogram, we collect the metrics from histogram
// after the |action| is performed.
func VerifyNumberOfPromotedOverlays(ctx context.Context, tconn *chrome.TestConn, s *testing.State, count int, action func(ctx context.Context) error) error {
	hists, err := metrics.Run(ctx, tconn, action, PromotedOverlayHistogramName)
	if err != nil {
		return errors.Wrap(err, "error while recording histogram")
	}

	hist := hists[0]

	if len(hist.Buckets) == 0 {
		return errors.Wrap(err, "got no overlay strategy data")
	}

	for _, bucket := range hist.Buckets {
		// The bucket ranges should be between |count| (inclusive) and |count|+1 (exclusive).
		if bucket.Min != int64(count) && bucket.Max != int64(count)+1 {
			return errors.Wrapf(err, "%d of %d frame(s) promoted %d overlays. Expected to promote %d overlays each frame",
				bucket.Count, hist.TotalCount(), bucket.Min, count)
		}
	}

	return nil
}
