// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package camerabox provides utilities to interact with dut and tablet in camerabox.
package camerabox

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/camera/chart"
	pb "go.chromium.org/tast-tests/cros/services/cros/camerabox"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

type fixture struct {
	// cl is the connection between host and dut. Sets to nil if connection
	// is not ready.
	cl *rpc.Client
	// chart controls chart display on chart tablet. Sets to nil if chart
	// is not ready.
	chart *chart.Chart
}

// FixtureData is the struct exposed to tests.
type FixtureData struct {

	// ConnectToDUT creates a connection between host and dut
	ConnectToDUT func(ctx context.Context, d *dut.DUT, h *testing.RPCHint) (*rpc.Client, error)

	// PrepareChart prepares chart by loading the given scene. It only works for
	// CameraBox.
	PrepareChart func(ctx context.Context, d *dut.DUT, outdir, altHostname, chartPath string) error

	// LogTestScene takes a photo of test scene as log to debug scene related problem.
	LogTestScene func(ctx context.Context, d *dut.DUT, facing pb.Facing, outdir string) error
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "cameraboxFixture",
		Desc:            "Set up camera box fixture",
		Contacts:        []string{"chromeos-camera-eng@google.com", "xinggu@google.com"},
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &fixture{},
		Vars:            []string{"chart"},
		SetUpTimeout:    5 * time.Second,
		TearDownTimeout: 5 * time.Second,
	})
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {

	return FixtureData{
		ConnectToDUT: f.connectToDUT,
		PrepareChart: f.prepareChart,
		LogTestScene: f.logTestScene,
	}
}
func (f *fixture) prepareChart(ctx context.Context, d *dut.DUT, outdir, altHostname, chartPath string) (retErr error) {
	if f.chart == nil {

		c, namePaths, err := chart.New(ctx, d, altHostname, outdir, []string{chartPath})
		if err != nil {
			return errors.Wrap(err, "failed to prepare chart tablet")
		}
		if err := c.Display(ctx, namePaths[0]); err != nil {
			if err := c.Close(ctx, outdir); err != nil {
				testing.ContextLog(ctx, "Failed to cleanup chart: ", err)
			}
			return errors.Wrap(err, "failed to display chart on chart tablet")
		}
		f.chart = c
	}
	return nil
}

func (f *fixture) logTestScene(ctx context.Context, d *dut.DUT, facing pb.Facing, outdir string) (retErr error) {
	testing.ContextLog(ctx, "Capture scene log image")

	// Release camera unique resource from cros-camera temporarily for taking a picture of test scene.
	out, err := d.Conn().CommandContext(ctx, "status", "cros-camera").Output()
	if err != nil {
		return errors.Wrap(err, "failed to get initial state of cros-camera")
	}
	if strings.Contains(string(out), "start/running") {
		if err := d.Conn().CommandContext(ctx, "stop", "cros-camera").Run(); err != nil {
			return errors.Wrap(err, "failed to stop cros-camera")
		}
		defer func() {
			if err := d.Conn().CommandContext(ctx, "start", "cros-camera").Run(); err != nil {
				if retErr != nil {
					testing.ContextLog(ctx, "Failed to start cros-camera")
				} else {
					retErr = errors.Wrap(err, "failed to start cros-camera")
				}
			}
		}()
	}

	// Timeout for capturing scene image.
	captureCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Take a picture of test scene.
	facingArg := "back"
	if facing == pb.Facing_FACING_FRONT {
		facingArg = "front"
	}
	const sceneLog = "/tmp/scene.jpg"
	captureCmd := append([]string{}, "--user=arc-camera", "cros_camera_test",
		"--gtest_filter=Camera3StillCaptureTest/Camera3DumpSimpleStillCaptureTest.DumpCaptureResult/0",
		"--camera_facing="+facingArg,
		"--dump_still_capture_path="+sceneLog,
		"--connect_to_camera_service=false")
	testing.ContextLog(ctx, "Running ", strings.Join(captureCmd, " "))
	if err := d.Conn().CommandContext(captureCtx, "sudo", captureCmd...).Run(); err != nil {
		return errors.Wrap(err, "failed to run cros_camera_test to take a scene photo")
	}

	// Copy result scene log image.
	if err := linuxssh.GetFile(ctx, d.Conn(), sceneLog, filepath.Join(outdir, filepath.Base(sceneLog)), linuxssh.PreserveSymlinks); err != nil {
		return errors.Wrap(err, "failed to pull scene log file from DUT")
	}
	if err := d.Conn().CommandContext(ctx, "rm", sceneLog).Run(); err != nil {
		return errors.Wrap(err, "failed to clean up scene log file from DUT")
	}
	return nil
}

func (f *fixture) connectToDUT(ctx context.Context, d *dut.DUT, h *testing.RPCHint) (cl *rpc.Client, retErr error) {
	if f.cl == nil {
		// Connect to the gRPC server on the DUT.
		cl, err := rpc.Dial(ctx, d, h)
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to the HAL3 service on the DUT")
		}
		f.cl = cl
	}
	return f.cl, nil
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	f.cl.Close(ctx)
	if f.chart == nil {
		return
	}
	if err := f.chart.Close(ctx, s.OutDir()); err != nil {
		s.Error("Failed to cleanup chart: ", err)
	}
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixture) Reset(ctx context.Context) error { return nil }
