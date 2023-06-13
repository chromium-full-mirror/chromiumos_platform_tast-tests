// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoPlaybackSuspendStress,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Video playback on onboard speaker: suspend-resume with operation for 10 cycles",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Attr:         []string{"group:intel-nda"},
		Data:         []string{"1080p_60fps_600frames.vp8.webm", "video.html", "playback.js"},
		VarDeps:      []string{"servo"},
		Timeout:      8 * time.Minute,
	})
}
func VideoPlaybackSuspendStress(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()
	servoSpec := s.RequiredVar("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Connect to gRPC server.
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	client := typec.NewServiceClient(cl.Conn)
	if _, err = client.NewChromeLogin(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	downloads, err := client.DownloadsPath(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal(s, "Failed to return download path: ", err)
	}

	downloadsPath := downloads.DownloadsPath

	htmlPath := filepath.Join(downloadsPath, "video.html")
	videoPath := filepath.Join(downloadsPath, "1080p_60fps_600frames.vp8.webm")
	jsPath := filepath.Join(downloadsPath, "playback.js")
	dataMap := map[string]string{s.DataPath("video.html"): htmlPath,
		s.DataPath("1080p_60fps_600frames.vp8.webm"): videoPath,
		s.DataPath("playback.js"):                    jsPath,
	}

	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), dataMap, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to send data to remote data path: ", err)
	}
	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power on DUT at cleanup: ", err)
			}
		}
		if err := dut.Conn().CommandContext(cleanupCtx, "sh", "-c", fmt.Sprintf("rm -rf %s %s %s", htmlPath, videoPath, jsPath)).Run(); err != nil {
			s.Errorf("Failed to remove  %s %s %s files : %v", htmlPath, videoPath, jsPath, err)
		}
	}(cleanupCtx)

	videoFile := "1080p_60fps_600frames.vp8.webm"
	if _, err = client.PlayLocalVideo(ctx, &typec.KeyPath{Path: videoFile}); err != nil {
		s.Fatal(s, "Failed to play local video: ", err)
	}

	expectedAudioUINode := "Speaker (internal)"
	expectedAudioNode := "INTERNAL_SPEAKER"
	isVerifyRouting := true
	if err := setAudioNodeAndCheckRouting(ctx, cl, expectedAudioNode, expectedAudioUINode, isVerifyRouting); err != nil {
		s.Fatalf("Failed to set audio node and check routing %q : %v", expectedAudioNode, err)
	}

	slpOpSetPre, pkgOpSetPre, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get SLP counter and C10 package values before suspend-resume: ", err)
	}

	if err := powercontrol.PerformSuspendStressTest(ctx, dut, 10); err != nil {
		s.Fatal("Failed to perform suspend stress test to check for zero errors: ", err)
	}

	if _, err := client.ReconnectChromeConnection(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to reconnect chrome connection: ", err)
	}

	if _, err := client.NewConnectionForTarget(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to create new connection for target: ", err)
	}

	if _, err = client.PlayVideo(ctx, &typec.KeyPath{Path: videoFile}); err != nil {
		s.Fatal(s, "Failed to play video again: ", err)
	}

	if err := setAudioNodeAndCheckRouting(ctx, cl, expectedAudioNode, expectedAudioUINode, isVerifyRouting); err != nil {
		s.Fatalf("Failed to set audio node and check routing %q : %v", expectedAudioNode, err)
	}

	slpOpSetPost, pkgOpSetPost, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get SLP counter and C10 package values after suspend-resume: ", err)
	}

	if err := powercontrol.AssertSLPAndC10(slpOpSetPre, slpOpSetPost, pkgOpSetPre, pkgOpSetPost); err != nil {
		s.Fatal("Failed to verify SLP and C10 state values: ", err)
	}
}

// setAudioNodeAndCheckRouting sets to expected audio node and verifies first runnung device.
func setAudioNodeAndCheckRouting(ctx context.Context, cl *rpc.Client, expectedAudioOuputNode, expectedAudioOuputUINode string, isVerifyRouting bool) error {
	client := typec.NewServiceClient(cl.Conn)
	expectedAudioNode := &typec.KeyPath{AudioNode: expectedAudioOuputUINode}
	testing.ContextLog(ctx, expectedAudioNode)
	if _, err := client.SetActiveNodeByUI(ctx, expectedAudioNode); err != nil {
		return errors.Wrap(err, "failed to select output audio node")
	}

	deviceName, err := client.AudioCrasSelectedOutputDevice(ctx, &empty.Empty{})
	testing.ContextLog(ctx, "deviceName:", deviceName)
	testing.ContextLog(ctx, "deviceName.DeviceType:", deviceName.DeviceType)
	if err != nil {
		return errors.Wrap(err, "failed to get output audio device info")
	}
	if deviceName.DeviceType != expectedAudioOuputNode {
		return errors.Wrapf(err, "failed to select audio device %q", expectedAudioOuputUINode)
	}

	if isVerifyRouting {
		runningDeviceName := &typec.KeyPath{AudioNode: deviceName.DeviceName}
		if _, err := client.VerifyFirstRunningDevice(ctx, runningDeviceName); err != nil {
			return errors.Wrapf(err, "failed to verify first running device %q", expectedAudioOuputUINode)
		}
	}
	return nil
}
