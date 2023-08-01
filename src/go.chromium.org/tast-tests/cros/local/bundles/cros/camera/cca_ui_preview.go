// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPreview,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies the preview functions",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org", "shik@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera", "group:intel-nda", "group:criticalstaging"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
		BugComponent: "b:978428",
	})
}

// CCAUIPreview verifies preview related functionalities of CCA.
func CCAUIPreview(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()

	if err := testResize(ctx, app); err != nil {
		s.Error("Failed in testResize(): ", err)
	}
	// TODO(shik): Add the missing preview tests in go/cca-test:
	// * Preview active after going back from gallery
	// * Preview active after taking picture
	// * Preview active after recording
	// * Preview active after suspend/resume
}

func testResize(ctx context.Context, app *cca.App) error {
	restore := func() error {
		if err := app.RestoreWindow(ctx); err != nil {
			return errors.Wrap(err, "failed to restore window")
		}
		// It is expected that the preview will only be active after the window
		// is focus.
		if err := app.Focus(ctx); err != nil {
			return errors.Wrap(err, "failed to focus window")
		}

		if err := app.WaitForVideoActive(ctx); err != nil {
			return errors.Wrap(err, "preview is inactive after restoring window")
		}
		return nil
	}
	if err := restore(); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Maximizing window")
	if err := app.MaximizeWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to maximize window")
	}
	if err := app.WaitForVideoActive(ctx); err != nil {
		return errors.Wrap(err, "preview is inactive after maximizing window")
	}
	if err := restore(); err != nil {
		return errors.Wrap(err, "failed in restore() after maximizing window")
	}

	testing.ContextLog(ctx, "Fullscreening window")
	if err := app.FullscreenWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to fullscreen window")
	}
	if err := app.WaitForVideoActive(ctx); err != nil {
		return errors.Wrap(err, "preview is inactive after fullscreening window")
	}
	if err := restore(); err != nil {
		return errors.Wrap(err, "failed in restore() after fullscreening window")
	}

	testing.ContextLog(ctx, "Minimizing window")
	if err := app.MinimizeWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to minimize window")
	}
	if err := app.CheckVideoInactive(ctx); err != nil {
		return errors.Wrap(err, "preview is active after minimizing window")
	}
	if err := restore(); err != nil {
		return errors.Wrap(err, "failed in restore() after maximizing window")
	}

	return nil
}
