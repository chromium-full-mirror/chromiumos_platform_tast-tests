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
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GpuCUJ,
		Desc:         "GPU performance CUJ tests",
		Contacts:     []string{"cros-sw-perf@google.com", "yichenz@chromium.org"},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      25*time.Minute + cuj.CPUStabilizationTimeout,
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
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Setup server to serve video file.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	params := s.Param().(gpucuj.TestParams)
	if err := gpucuj.Run(ctx, cr, params, server.URL, s.OutDir()); err != nil {
		s.Fatal("Failed to run GpuCUJ test: ", err)
	}
}
