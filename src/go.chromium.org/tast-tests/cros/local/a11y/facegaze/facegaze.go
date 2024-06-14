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

func (d driver) Start() error {
	// Click the "Start FaceGaze" button.
	ctx := d.ctx
	ui := d.ui

	button := nodewith.Role(role.Button).NameContaining("Click to start FaceGaze").Onscreen()
	if err := uiauto.Combine("Click the 'Start FaceGaze' button",
		ui.WaitUntilExists(button),
		ui.DoDefault(button),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the 'Start FaceGaze' button")
	}

	if err := testing.Poll(ctx, a11y.VerifyFaceGazeAssetsInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for the facegaze-assets dlc to be installed")
	}

	return nil
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
	return driver{ctx, conn, tconn, ui, tdh}, nil
}
