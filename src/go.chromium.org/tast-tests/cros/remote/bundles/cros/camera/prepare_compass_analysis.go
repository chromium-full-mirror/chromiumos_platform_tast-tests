// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/services/cros/camera"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"

	"golang.org/x/sync/errgroup"
)

const (
	artifactGSBucketURL       = "gs://cros-engprod-intertech-test-artifect-alpha/"
	defaultCCAOutputDirectory = "/home/chronos/user/MyFiles/Camera/"
	lightControlDevice        = "FT232 Serial (UART) IC"
)

type cameraParams struct {
	mode   camera.CameraMode
	facing camera.Facing
}

func init() {
	testing.AddTest(&testing.Test{
		Func: PrepareCompassAnalysis,
		Desc: "Generate Compass Report",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"yunjunlee@google.com",
		},

		// BugComponent of CrOS Platform EngProd Interactive Technology
		BugComponent: "b:1280385",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.camera.CCAService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "photo_front",
			Val: cameraParams{
				mode:   camera.CameraMode_PHOTO,
				facing: camera.Facing_FACING_FRONT,
			},
		}, {
			Name: "photo_back",
			Val: cameraParams{
				mode:   camera.CameraMode_PHOTO,
				facing: camera.Facing_FACING_BACK,
			},
		}, {
			Name: "video_front",
			Val: cameraParams{
				mode:   camera.CameraMode_VIDEO,
				facing: camera.Facing_FACING_FRONT,
			},
		}, {
			Name: "video_back",
			Val: cameraParams{
				mode:   camera.CameraMode_VIDEO,
				facing: camera.Facing_FACING_BACK,
			},
		}},
	})
}

// checkLightControlRequirements checks the light control requirements:
// the execution PAR file and the light control USB connection
func checkLightControlRequirements(ctx context.Context, dutConn *ssh.Conn) error {
	connectedUsbDevices, err := dutConn.CommandContext(ctx, "lsusb").Output()
	if err != nil {
		return errors.Wrap(err, "failed to check USB devices connected to DUT")
	}
	if !strings.Contains(string(connectedUsbDevices), lightControlDevice) {
		return errors.New("unable to find light control device. (Is it connected to DUT?)")
	}
	return nil
}

// takePhotoWithCCA takes photo with CCA service on DUT
func takePhotoWithCCA(ctx context.Context, dutConn *ssh.Conn, cp cameraParams, cr camera.CCAServiceClient) (string, error) {
	testOpenCameraRequest := &camera.CameraTestRequest{
		Mode:   cp.mode,
		Facing: cp.facing,
	}
	_, err := cr.OpenCamera(ctx, testOpenCameraRequest)
	if err != nil {
		return "", err
	}
	defer cr.CloseCamera(ctx, &empty.Empty{})

	_, err = cr.TakePicture(ctx, &empty.Empty{})
	if err != nil {
		return "", err
	}

	file, err := dutConn.CommandContext(ctx, "ls", defaultCCAOutputDirectory).Output()
	if err != nil {
		return "", err
	}

	path := defaultCCAOutputDirectory + strings.TrimSpace(string(file))
	testing.ContextLogf(ctx, "Source file Path on DUT: %s", path)

	return path, nil
}

// activateLightControl activates light control unit with specified intensities, light-temperature, and intervals
func activateLightControl(ctx context.Context, dutConn *ssh.Conn) error {
	script := "/usr/local/autotest/bin/light_control_ccvp.py"
	// the intervals are hard-coded for now since we only have fixed scenario.
	intervals := "0,0, 1000,30000, 25000,10000, 65535,50000, 3000,500, 0,0"
	err := dutConn.CommandContext(ctx, "python3", script, "jump", "1", intervals, "3").Run()
	if err != nil {
		testing.ContextLogf(ctx, "Unable to activate light control: %s", err)
		return err
	}
	return nil
}

// recordVideoWithCCA record video with CCA service on DUT
func recordVideoWithCCA(ctx context.Context, dutConn *ssh.Conn, cp cameraParams, cr camera.CCAServiceClient) (string, error) {
	defaultVideoOutputPath := "/tmp/test_recording_10s.mp4"
	testOpenCameraRequest := &camera.CameraTestRequest{
		Mode:   cp.mode,
		Facing: cp.facing,
	}
	_, err := cr.OpenCamera(ctx, testOpenCameraRequest)
	if err != nil {
		return "", err
	}
	defer cr.CloseCamera(ctx, &empty.Empty{})

	eg := errgroup.Group{}
	eg.Go(func() error {
		return activateLightControl(ctx, dutConn)
	})
	eg.Go(func() error {
		return dutConn.CommandContext(ctx, "cca", "record-video", "--duration=10", "--output="+defaultVideoOutputPath).Run()
	})
	if err := eg.Wait(); err != nil {
		testing.ContextLogf(ctx, "Unable to take video with light control: %s", err)
		return "", err
	}
	testing.ContextLogf(ctx, "Source file Path on DUT: %s", defaultVideoOutputPath)

	return defaultVideoOutputPath, nil
}

// getCurrentTimeInUTC returns a filename based on UTC
func getCurrentTimeInUTC(ctx context.Context) string {
	currentTime := time.Now().UTC().Format(time.RFC3339)
	testing.ContextLogf(ctx, "Current time in UTC: %s", currentTime)
	currentTimeInUTC := strings.Replace(strings.Replace(currentTime, ":", "", -1), "-", "", -1)
	return currentTimeInUTC
}

// uploadArtifactToGS uploads the IQ artifacts to GS bucket
func uploadArtifactToGS(ctx context.Context, artifactPath, artifactName string) (uuid.UUID, error) {
	analysisUUID := uuid.New()
	artifactGSURL := fmt.Sprintf("%s%s/", artifactGSBucketURL, analysisUUID)
	testing.ContextLogf(ctx, "Using URL (GS bucket with folder): %s", artifactGSURL)
	cmd := testexec.CommandContext(ctx, "gsutil", "cp", artifactPath, artifactGSURL+artifactName)
	testing.ContextLog(ctx, "Running command: ", shutil.EscapeSlice(cmd.Args))
	return analysisUUID, cmd.Run(testexec.DumpLogOnError)
}

// PrepareCompassAnalysis prepares the required materials for Compass analysis
// To generate the Compass PDF report in human readable format, we will need:
// A photo (or video) taken under specific light environment and selected chart.
//
// TODO: Add Tast argument of JSON file that provides Compass parameters (e.g. mode, chart...etc.)
func PrepareCompassAnalysis(ctx context.Context, s *testing.State) {
	dutConn := s.DUT().Conn()
	params := s.Param().(cameraParams)

	// Bootstrap Light Control
	if err := checkLightControlRequirements(ctx, dutConn); err != nil {
		s.Fatal("Failed to verify the Light Control requirements. Please check the DUT connection: ", err)
	}

	// Connect to RPC.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	cr := camera.NewCCAServiceClient(cl.Conn)
	if _, err := cr.NewChrome(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.CloseChrome(ctx, &empty.Empty{})

	var artifactPathInDUT, fileExtension string
	if params.mode == camera.CameraMode_PHOTO {
		fileExtension = ".jpg"
		artifactPathInDUT, err = takePhotoWithCCA(ctx, dutConn, s.Param().(cameraParams), cr)
		if err != nil {
			s.Fatal("Failed to take photo with CCA: ", err)
		}
	} else {
		fileExtension = ".mp4"
		artifactPathInDUT, err = recordVideoWithCCA(ctx, dutConn, s.Param().(cameraParams), cr)
		if err != nil {
			s.Fatal("Failed to record video with CCA: ", err)
		}
	}
	defer dutConn.CommandContext(ctx, "rm", artifactPathInDUT).Output()

	// Move the file from DUT to Drone
	currentTimeInUTC := getCurrentTimeInUTC(ctx)
	artifactName := "iq-" + currentTimeInUTC + fileExtension
	artifactPathInDrone := filepath.Join(s.OutDir(), artifactName)
	if err = linuxssh.GetFile(ctx, dutConn, artifactPathInDUT, artifactPathInDrone, linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy the file from DUT to Drone: ", err)
	}
	defer os.Remove(artifactPathInDrone)

	uuid, err := uploadArtifactToGS(ctx, artifactPathInDrone, artifactName)
	if err != nil {
		s.Fatal("Failed to upload artifact: ", err)
	}
	testing.ContextLogf(ctx, "Analysis UUID (one-time generated): %s", uuid)
}
