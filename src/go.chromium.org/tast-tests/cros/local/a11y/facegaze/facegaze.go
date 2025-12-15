// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package facegaze provides functions to assist with interacting with the FaceGaze feature.
package facegaze

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// FakeCameraVideoFile720p specifies the video file to use for FaceGaze tests.
const FakeCameraVideoFile720p = "facegaze_camera_video_720p.y4m"

// faceGazeConfirmationDialogText specifies the text of the confirmation
// dialog that is shown when FaceGaze is first enabled.
const faceGazeConfirmationDialogText = "Face control gives you cursor control with face pointing and ability to perform actions, such as left clicking with facial gestures like smile"

// conn represents a connection to the FaceGaze background page.
type conn struct {
	*chrome.Conn
}

// newConn returns a connection to the FaceGaze extension's background page.
// If the extension is not ready, the connection will be closed before returning.
// Otherwise the calling function will close the connection.
func newConn(ctx context.Context, c *chrome.Chrome) (_ *conn, e error) {
	extConn, err := c.NewConnForTarget(ctx, chrome.MatchTargetURL(a11y.AccessibilityCommonExtensionURL))
	if err != nil {
		return nil, err
	}

	defer func() {
		if e != nil {
			extConn.Close()
		}
	}()

	if err := extConn.WaitForExpr(ctx, "Boolean(globalThis.accessibilityCommon.faceGaze_)"); err != nil {
		return nil, errors.Wrap(err, "FaceGaze is unavailable")
	}

	return &conn{extConn}, nil
}

// driver contains useful objects for driving FaceGaze tests and is
// returned by SetUp. Most notably, TearDown() should be run in a defer
// statement by the calling test to properly clean up FaceGaze.
type driver struct {
	ctx   context.Context
	conn  *conn
	Tconn *chrome.TestConn
	ui    *uiauto.Context
	tdh   *a11y.TearDownHelper
}

func newNoOpDriver(tdh *a11y.TearDownHelper) driver {
	return driver{tdh: tdh}
}

// TearDown is a convenience method that routes directly to TearDownHelper's
// implementation of TearDown.
func (d driver) TearDown() error {
	return d.tdh.TearDown()
}

func setUpFakeCamera(ctx context.Context, dataPath func(string) string, tdh *a11y.TearDownHelper) error {
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	tdh.Append(func() error {
		cancel()
		return nil
	})

	const cameraService = "cros-camera"
	tdh.Append(func() error {
		return upstart.RestartJob(cleanUpCtx, cameraService)
	})

	// Configure CrOS to use only fake HAL camera.
	if err := testutil.SetupTestConfig(ctx, testutil.UseFakeHALCamera); err != nil {
		return errors.Wrap(err, "failed to set up camera test config")
	}
	tdh.Append(func() error {
		testutil.RemoveTestConfig(cleanUpCtx)
		return nil
	})

	// Copy the fake camera video to where the camera module can access.
	dutFakeHALPath, err := testutil.CopyFakeHALFrameImage(dataPath(FakeCameraVideoFile720p))
	if err != nil {
		return errors.Wrap(err, "failed to copy fake camera input")
	}
	tdh.Append(func() error {
		return os.Remove(dutFakeHALPath)
	})

	// Write the fake HAL config to the system.
	fakeCameraConfig := testutil.FakeCameraConfig{
		ID:        1,
		Connected: true,
		Frames: &testutil.FakeCameraImageConfig{
			Path: dutFakeHALPath,
		},
		SupportedFormats: []*testutil.FakeCameraFormatsConfig{{
			Width:      320,
			Height:     180,
			FrameRates: []int{30}}, {
			Width:      640,
			Height:     360,
			FrameRates: []int{30}}, {
			Width:      1280,
			Height:     720,
			FrameRates: []int{30}},
		},
	}
	fakeHALConfig := testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{fakeCameraConfig},
	}
	if err := testutil.WriteFakeHALConfig(ctx, fakeHALConfig); err != nil {
		return errors.Wrap(err, "failed to configure HAL camera")
	}
	tdh.Append(func() error {
		return testutil.RemoveFakeHALConfig(cleanUpCtx)
	})

	if err := upstart.RestartJob(ctx, cameraService); err != nil {
		return errors.Wrapf(err, "failed to restart %s after camera setup", cameraService)
	}

	return nil
}

// maybeCloseConfirmationDialog closes the dialog that is shown when FaceGaze is first
// enabled, if it appears on the screen. The dialog informs the user about how
// to use the FaceGaze feature. This function accepts the dialog so we can use the feature.
func maybeCloseConfirmationDialog(ctx context.Context, ui *uiauto.Context) error {
	text := nodewith.NameContaining(faceGazeConfirmationDialogText).Onscreen()
	continueButton := nodewith.Name("Continue").ClassName("MdTextButton").Onscreen()

	// Check if the dialog pops up.
	if err := ui.WaitUntilExists(text)(ctx); err != nil {
		// If the dialog can't be found, then we don't need to do anything.
		return nil
	}

	if err := uiauto.Combine("Close FaceGaze confirmation dialog",
		ui.LeftClick(continueButton),
		ui.WaitUntilGone(text),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close the FaceGaze confirmation dialog")
	}

	return nil
}

// ensureFaceGazeToggleButtonOn ensures that the face control settings page is
// open and that the FaceGaze toggle button is on, since a11y.SetFaceGazeEnabled()
// may fail to enable FaceGaze.
func ensureFaceGazeToggleButtonOn(ui *uiauto.Context) uiauto.Action {
	settingsPage := nodewith.HasClass("showing-subpage").Role(role.Main)
	accessibilitySection := nodewith.Name("Accessibility").Role(role.GenericContainer).Ancestor(settingsPage).First()
	faceControlHeading := nodewith.Name("Face control").Role(role.Heading).Ancestor(accessibilitySection)
	toggleButton := nodewith.Role(role.ToggleButton).Ancestor(accessibilitySection)
	offToggleButton := toggleButton.Name("Off")
	onToggleButton := toggleButton.Name("On")
	return uiauto.NamedCombine("ensure FaceGaze toggle button is on",
		ui.WaitUntilExists(faceControlHeading),
		ui.WaitUntilAnyExists(offToggleButton, onToggleButton),
		uiauto.IfFailThen(
			ui.EnsureGoneFor(offToggleButton, 5*time.Second),
			uiauto.NamedAction(
				"toggle button on",
				ui.DoDefaultUntil(offToggleButton,
					ui.EnsureExistsFor(onToggleButton, 3*time.Second),
				),
			),
		),
	)
}

// SetUp executes common FaceGaze setup code and returns a driver that can be
// used to easily drive FaceGaze tests.
func SetUp(ctx context.Context, cr *chrome.Chrome, dataPath func(string) string) (d driver, e error) {
	// Tears down FaceGaze if SetUp encountered an error.
	defer func() {
		if e != nil {
			d.TearDown()
		}
	}()

	tdh := &a11y.TearDownHelper{}

	// Shorten deadline to leave time for cleanup.
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	tdh.Append(func() error {
		cancel()
		return nil
	})

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to create Test API connection")
	}

	if err := a11y.SetFaceGazeEnabled(ctx, tconn, true); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to enable FaceGaze")
	}
	tdh.Append(func() error {
		if err := a11y.SetFaceGazeEnabled(cleanUpCtx, tconn, false); err != nil {
			return errors.Wrap(err, "failed to disable FaceGaze")
		}

		return nil
	})

	// Create a new connection to the FaceGaze extension background page.
	conn, err := newConn(ctx, cr)
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to create a new connection to the FaceGaze extension background page")
	}
	tdh.Append(func() error {
		conn.Close()
		return nil
	})

	if err := setUpFakeCamera(ctx, dataPath, tdh); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to setup the fake camera")
	}

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	if err := maybeCloseConfirmationDialog(ctx, ui); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to close the FaceGaze confirmation dialog")
	}

	verifyFaceGazeAssetsInstalled := uiauto.NamedCombine("verify FaceGaze assets installed",
		ensureFaceGazeToggleButtonOn(ui),
		a11y.VerifyFaceGazeAssetsInstalled,
	)
	// When FaceGaze is enabled, it will automatically trigger an install of the
	// facegaze-assets DLC, so wait for it to be installed before continuing.
	if err := testing.Poll(ctx, verifyFaceGazeAssetsInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 5 * time.Second}); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to wait for the facegaze-assets dlc to be installed")
	}

	return driver{ctx, conn, tconn, ui, tdh}, nil
}
