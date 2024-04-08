// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/graphics/roundeddisplay"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	interval               = 5 * time.Second // Power metrics collect interval.
	testDuration           = 5 * time.Minute
	timeoutAfterFullscreen = 5 * time.Second
)

type roundedDisplayPowerParam struct {
	enableFeature bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         RoundedDisplayPower,
		LacrosStatus: testing.LacrosVariantUnneeded, // Ash is only a platform dependency.
		Desc:         "Collect power metrics of using overlays for software masks for rounded displays",
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Contacts: []string{
			"chromeos-foundations@google.com",
			"zoraiznaeem@chromium.org",
			"skau@chromium.org",
		},
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays(), hwdep.InternalDisplay(), hwdep.Model("bugzzy")),
		Data:         []string{"d-canvas/main.html", "d-canvas/2d.js", "d-canvas/webgl.js"},
		Timeout:      10*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name:    "enabled",
				Fixture: roundeddisplay.PowerRoundedDisplayWithFlagOn,
				Val:     roundedDisplayPowerParam{enableFeature: true},
			},
			{
				Name:    "disabled",
				Fixture: roundeddisplay.PowerRoundedDisplayWithFlagOff,
				Val:     roundedDisplayPowerParam{enableFeature: false},
			},
		},
	})
}

func RoundedDisplayPower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	param := s.Param().(roundedDisplayPowerParam)

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Cool down test device.
	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	// Setups a maximized browser window that runs low-latency canvas animation.
	// The canvas will be behind display masks with rounded corners.
	// Since the low-latency canvas texture is promoted to the hardware overlay,
	// the rounded display mask textures will be forced to be promoted to the overlay.
	if err := roundeddisplay.SetupFullscreenLowLatencyCanvas(ctx, s, tconn, cr); err != nil {
		s.Fatal("Failed to setup fullscreen canvas: ", err)
	}

	// GoBigSleepLint: After the browser goes into fullscreen, wait for
	// the fps of the animation in the low-latency canvas to become stable
	// (damage per frame becomes stable as well). This will ensure consistent
	// promotion of overlays.
	if err := testing.Sleep(ctx, timeoutAfterFullscreen); err != nil {
		s.Fatal("Failed to wait: ", err)
	}

	fastInkAction := action.Sleep(time.Second)

	expectedOverlays := 1
	if param.enableFeature {
		expectedOverlays = roundeddisplay.ExpectedNumberOfPromotedOverlays
	}

	if err := roundeddisplay.VerifyNumberOfPromotedOverlays(
		ctx, tconn, s, expectedOverlays, fastInkAction); err != nil {
		s.Fatal("Expected number of overlays not promoted: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// GoBigSleepLint: Keep the animation running to measure the power usage.
	if err := testing.Sleep(ctx, testDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
