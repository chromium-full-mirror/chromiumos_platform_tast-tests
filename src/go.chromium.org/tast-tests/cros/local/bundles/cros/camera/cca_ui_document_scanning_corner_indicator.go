// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIDocumentScanningCornerIndicator,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the corner indicator for document scanning",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org", "chuhsuan@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", "ondevice_document_scanner_rootfs_or_dlc", "camera_doc_corner_indicator"},
		Data:         []string{"document_3264x2448.mjpeg"},
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
	})
}

func CCAUIDocumentScanningCornerIndicator(ctx context.Context, s *testing.State) {
	startApp := s.FixtValue().(cca.FixtureData).StartApp
	stopApp := s.FixtValue().(cca.FixtureData).StopApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := switchScene(ctx, cca.SceneData{Path: s.DataPath("document_3264x2448.mjpeg")}); err != nil {
		s.Fatal("Failed to prepare document scene: ", err)
	}

	app, err := startApp(ctx)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := stopApp(cleanupCtx, s.HasError()); err != nil {
			s.Fatal("Failed to close CCA: ", err)
		}
	}(cleanupCtx)

	if err := app.EnterDocumentMode(ctx); err != nil {
		s.Fatal("Failed to enter document mode: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		result, err := app.Visible(ctx, cca.DocumentCorner)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to check visibility of corner indicator"))
		} else if !result {
			return errors.Wrap(err, "no document is found")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for corner indicator to show up: ", err)
	}
}
