// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	initTimePNPDirectOffUSBCameraPower               = 1 * time.Minute
	traceTimePNPDirectOffUSBCameraPower              = 20 * time.Minute
	traceTimePerResolutionPNPDirectOffUSBCameraPower = 20 * time.Second
)

type captureMetadata struct {
	cameraName string
	format     string
	width      int32
	height     int32
	fps        int32
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPDirectOffUSBCameraPower,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics for capturing frame from a usb camera using yavta",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{caps.BuiltinUSBCamera},
		Fixture:      pnp.StablePowerLacros,
		Timeout:      initTimePNPDirectOffUSBCameraPower + traceTimePNPDirectOffUSBCameraPower + power.RecorderTimeout,
	})
}

func parseYavtaEnumFormats(ctx context.Context, videoNode string) ([]captureMetadata, error) {
	var ret []captureMetadata

	cmd := testexec.CommandContext(
		ctx, "yavta", "--enum-formats", videoNode)
	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return ret, errors.Wrap(err, "failed to run yavta --enum-formats")
	}

	lines := strings.Split(string(output), "\n")
	cameraNameSplitter := func(r rune) bool {
		return strings.Contains("`'", string(r))
	}
	formatSplitter := func(r rune) bool {
		return strings.Contains(":(", string(r))
	}
	frameSizeSplitter := func(r rune) bool {
		return strings.Contains(":x(/,)", string(r))
	}
	format := ""

	// Camera name string is of the following style:
	// Device `<camera_name>` on ...
	cameraName := strings.FieldsFunc(lines[1], cameraNameSplitter)[1]

	for _, line := range lines {
		if strings.Contains(line, "Format") {
			// Format string is of the following style:
			// Format <index>: <format> (<format id>)
			format = strings.TrimSpace(strings.FieldsFunc(line, formatSplitter)[1])
		} else if strings.Contains(line, "Frame size") {
			// Frame size string is of the following style:
			// Frame size: <width>x<height>(1/<fps1>, 1/<fps2>,...).
			token := strings.FieldsFunc(line, frameSizeSplitter)
			width, err := strconv.ParseInt(strings.TrimSpace(token[1]), 10, 32)
			if err != nil {
				return ret, errors.Wrap(err, "failed to parse width to int")
			}
			height, err := strconv.ParseInt(strings.TrimSpace(token[2]), 10, 32)
			if err != nil {
				return ret, errors.Wrap(err, "failed to parse height to int")
			}

			for i := 4; i < len(token); i += 2 {
				fps, err := strconv.ParseInt(strings.TrimSpace(token[i]), 10, 32)
				if err != nil {
					return ret, errors.Wrap(err, "failed to parse fps to int")
				}
				ret = append(ret, captureMetadata{
					cameraName: cameraName,
					format:     format,
					width:      int32(width),
					height:     int32(height),
					fps:        int32(fps),
				})
			}
		}
	}
	return ret, nil
}

func PNPDirectOffUSBCameraPower(ctx context.Context, s *testing.State) {
	// Reserve some time for the cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.StopJob(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to stop cros-camera service: ", err)
	}
	defer func() {
		if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
			s.Fatal("Failed to start cros-camera service: ", err)
		}
	}()

	usbCameraList, err := testutil.USBCamerasFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to get usb camera list: ", err)
	}

	perfTimeline, err := perf.NewTimeline(ctx, metrics.TestMetrics(), perf.Interval(2*time.Second))
	if err != nil {
		s.Fatal("Failed to build metrics timeline: ", err)
	}

	if err := perfTimeline.Start(ctx); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	pnpRoutine := pnp.Routine{}
	defer pnpRoutine.Close(cleanupCtx)
	perfValue := perf.NewValues()
	if err := pnp.Cooldown(ctx); err != nil {
		s.Fatal("Failed to run pnp cooldown routine: ", err)
	}

	testing.ContextLog(ctx, "[Start Work Phase]")
	for _, videoNode := range usbCameraList {
		usbCameraVersion, err := testutil.GetUsbCameraVersion(ctx, videoNode)
		vidPid := usbCameraVersion.IDVendor + ":" + usbCameraVersion.IDProduct

		captureMetadatas, err := parseYavtaEnumFormats(ctx, videoNode)
		if err != nil {
			s.Fatal("Failed to parse yavta enum formats from node ", videoNode, ": ", err)
		}
		if len(captureMetadatas) == 0 {
			s.Fatal("Find no captureMetadata in videoNode ", videoNode, ": ", err)
		}

		setCtrlCmd := testexec.CommandContext(
			ctx, "v4l2-ctl", "--device", videoNode, "--set-ctrl",
			"exposure_dynamic_framerate=0")
		_, err = setCtrlCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			s.Log("Failed to set exposure dynamic framerate to zero: ", err)
		}

		for _, captureMetadata := range captureMetadatas {
			// In ChromeOS Camera USB HAL, we use YUYV if it can support 30 fps
			// or MJPEG otherwise.
			if captureMetadata.fps < 30 {
				continue
			}

			// Nbufs is set to 4 to align with ChromeOS Camera USB HAL params.
			cmd := testexec.CommandContext(
				ctx, "yavta", "-Bcapture", "-c",
				fmt.Sprintf("--nbufs=4"),
				fmt.Sprintf("-f%s", captureMetadata.format),
				fmt.Sprintf("-s%dx%d", captureMetadata.width, captureMetadata.height),
				videoNode)
			s.Log("Run command: ", cmd)

			err := cmd.Start()
			if err != nil {
				s.Fatal("Failed to run yavta to capture frames: ", err)
			}

			if err := pnp.WarmUp(ctx); err != nil {
				s.Fatal("Failed to run pnp warm up routine: ", err)
			}

			testing.ContextLog(ctx, "[Record Phase] Start recording trace for ", traceTimePerResolutionPNPDirectOffUSBCameraPower)
			if err := perfTimeline.StartRecording(ctx); err != nil {
				s.Fatal("Failed to start recording: ", err)
			}

			// GoBigSleepLint: Collecting power metrics.
			if err := testing.Sleep(ctx, traceTimePerResolutionPNPDirectOffUSBCameraPower); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
			subPerfValue, err := perfTimeline.StopRecording(ctx)
			if err != nil {
				s.Fatal("Error while recording power metrics: ", err)
			}
			if err = cmd.Signal(unix.SIGINT); err != nil {
				s.Fatal("Failed to signal to yavta command: ", err)
			}
			if err = cmd.Wait(); err != nil {
				// We're expecting the exit status to be non-zero if the process
				// was killed by SIGINT. Anything else indicates a problem.
				if ws, ok := testexec.GetWaitStatus(err); !ok || !ws.Signaled() || ws.Signal() != unix.SIGINT {
					s.Fatal("Failed to wait for yavta termination: ", err)
				}
			}

			metricSuffix := fmt.Sprintf(
				".%s_%s_%s_%d_%d_%d", vidPid, captureMetadata.cameraName,
				captureMetadata.format, captureMetadata.width,
				captureMetadata.height, captureMetadata.fps)
			re := regexp.MustCompile("[^a-zA-Z0-9._-]")
			metricSuffix = re.ReplaceAllString(metricSuffix, "-")
			perfValue.MergeWithSuffix(metricSuffix, subPerfValue)
		}
	}
	if err = perfValue.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf value: ", err)
	}
}
