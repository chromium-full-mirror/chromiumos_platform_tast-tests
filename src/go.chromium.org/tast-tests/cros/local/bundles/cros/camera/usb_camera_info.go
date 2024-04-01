// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBCameraInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect camera modules info",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera_dependent", "group:camera-stability"},
		SoftwareDeps: []string{caps.BuiltinUSBCamera},
	})
}

func USBCameraInfo(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.StopJob(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to stop cros-camera service: ", err)
	}
	defer func() {
		if err := upstart.EnsureJobRunning(cleanupCtx, "cros-camera"); err != nil {
			s.Fatal("Failed to start cros-camera service: ", err)
		}
	}()

	usbCameraList, err := testutil.USBCamerasFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to get usb camera list: ", err)
	}

	for _, videoNode := range usbCameraList {
		usbCameraVersion, err := testutil.GetUsbCameraVersion(ctx, videoNode)
		vidPid := usbCameraVersion.IDVendor + ":" + usbCameraVersion.IDProduct
		vidPidBcd := vidPid + ":" + usbCameraVersion.BcdDevice
		videoNodeNum := path.Base(videoNode)
		lsusbCmd := testexec.CommandContext(
			ctx, "lsusb", "-d", vidPid, "-vvvv")
		output, err := lsusbCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to run lsusb with vid:pid = ", vidPid)
		}
		os.WriteFile(
			fmt.Sprintf("%s/%s-%s-lsusb.txt", s.OutDir(), vidPidBcd, videoNodeNum),
			output, 0644)

		yavtaControlCmd := testexec.CommandContext(
			ctx, "yavta", "--list-controls", videoNode)
		output, err = yavtaControlCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to run yavta --list-controls with videoNode = ", videoNode)
		}
		os.WriteFile(
			fmt.Sprintf("%s/%s-%s-yavta-control.txt", s.OutDir(), vidPidBcd, videoNodeNum),
			output, 0644)

		yavtaFormatCmd := testexec.CommandContext(
			ctx, "yavta", "--enum-formats", videoNode)
		output, err = yavtaFormatCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to run yavta --enum-formats with videoNode = ", videoNode)
		}
		os.WriteFile(
			fmt.Sprintf("%s/%s-%s-yavta-format.txt", s.OutDir(), vidPidBcd, videoNodeNum),
			output, 0644)

		v4l2ControlFormatCmd := testexec.CommandContext(
			ctx, "v4l2-ctl", "--all", "-d", videoNode)
		output, err = v4l2ControlFormatCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to run v4l2-ctl --all with videoNode = ", videoNode)
		}
		os.WriteFile(
			fmt.Sprintf("%s/%s-%s-v4l2-ctl.txt", s.OutDir(), vidPidBcd, videoNodeNum),
			output, 0644)
	}
}
