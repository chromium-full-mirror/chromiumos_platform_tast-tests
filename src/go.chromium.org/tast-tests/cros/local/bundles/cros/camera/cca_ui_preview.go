// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPreview,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies the preview functions",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org", "shik@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera", "group:intel-nda"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
	})
}

// CCAUIPreview verifies preview related functionalities of CCA.
func CCAUIPreview(ctx context.Context, s *testing.State) {
	testBridge := s.FixtValue().(cca.FixtureData).TestBridge
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp

	subTestTimeout := 30 * time.Second
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *testutil.TestBridge, *cca.App) error
	}{{
		"testWindowResize",
		testResize,
	}, {
		"testRefresh",
		testRefresh,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				return tst.testFunc(ctx, testBridge(), app)
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}
}

func testResize(ctx context.Context, _ *testutil.TestBridge, app *cca.App) error {
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

func testRefresh(ctx context.Context, tb *testutil.TestBridge, app *cca.App) error {
	if err := app.Refresh(ctx, tb); err != nil {
		return errors.Wrap(err, "failed to complete refresh")
	}

	if err := app.WaitForVideoActive(ctx); err != nil {
		return errors.Wrap(err, "preview is not shown after refreshing")
	}
	return nil
}
