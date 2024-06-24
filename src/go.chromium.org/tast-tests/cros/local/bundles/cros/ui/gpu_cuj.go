// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/gpucuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GpuCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GPU performance CUJ tests",
		Contacts:     []string{"cros-sw-perf@google.com", "ramsaroop@google.com"},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      25*time.Minute + cuj.CPUStablizationTimeout,
		Data: []string{
			"video.html",
			"continuous_scroll_60fps.html",
			"gradient_color_60fps.html",
			"webgl_small_60fps.html",
			"bbb_1080p60_yuv.vp9.webm",
		},
		Params: []testing.Param{{
			Name: "maximized",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMaximized,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "maximized_rot90",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMaximized,
				Rot90:    true,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "maximized_composited",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMaximized,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUserWithForceComposition",
		}, {
			Name: "maximized_non_delegated",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMaximized,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUserWithForceNonDelegated",
		}, {
			Name: "threedot",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeThreeDot,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "threedot_rot90",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeThreeDot,
				Rot90:    true,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "threedot_composited",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeThreeDot,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUserWithForceComposition",
		}, {
			Name: "resize",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeResize,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "resize_rot90",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeResize,
				Rot90:    true,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "resize_composited",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeResize,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUserWithForceComposition",
		}, {
			Name: "moveocclusion",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMoveOcclusion,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "moveocclusion_rot90",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMoveOcclusion,
				Rot90:    true,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "moveocclusion_composited",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMoveOcclusion,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUserWithForceComposition",
		}, {
			Name: "moveocclusion_withcroswindow",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMoveOcclusionWithCrosWindow,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "moveocclusion_withcroswindow_rot90",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMoveOcclusionWithCrosWindow,
				Rot90:    true,
			},
			Fixture: "loggedInToCUJUser",
		}, {
			Name: "moveocclusion_withcroswindow_composited",
			Val: gpucuj.TestParams{
				TestType: gpucuj.TestTypeMoveOcclusionWithCrosWindow,
				Rot90:    false,
			},
			Fixture: "loggedInToCUJUserWithForceComposition",
		}},
	})
}

func GpuCUJ(ctx context.Context, s *testing.State) {
	// Setup server to serve video file.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	pv, cleanup, err := gpucuj.RunGpuCUJ(ctx, cr, s.Param().(gpucuj.TestParams),
		server.URL, s.OutDir())
	if err != nil {
		s.Fatal("Could not run GpuCUJ test: ", err)
	}
	defer func() {
		if err := cleanup(ctx); err != nil {
			s.Fatal("Failed to cleanup after creating test: ", err)
		}
	}()

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Cannot save perf data: ", err)
	}
}
