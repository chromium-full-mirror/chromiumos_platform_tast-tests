// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/setup"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type displayParams struct {
	displayType        string
	displayDetectionRe string
	iterationValue     int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HDMIDisplayFunctionalityAfterLidCloseOpen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies connect HDMI Display to TBT to HDMI Dongle and check functionality after Lid close/open",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:typec"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Data:         []string{"testcert.p12", "1080p_60fps_600frames.vp8.webm", "video.html", "test_config.json", "playback.js"},
		VarDeps:      []string{"servo", "typec.dutTbtPort", "typec.cSwitchPort", "typec.domainIP", "typec.tbtDisplayPort"},
		HardwareDeps: hwdep.D(setup.ThunderboltSupportedDevices()),
		Params: []testing.Param{{
			Name: "tbt_dock_with_tbt_display",
			Val: displayParams{
				displayType:    usbutils.TBTDisplay,
				iterationValue: 1,
			},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-tbt3-dock-tbt-display"},
		}, {
			Name: "hdmi_lid_open_close",
			Val: displayParams{
				displayType:    usbutils.TypeCHDMI,
				iterationValue: 5,
			},
			Timeout:   8 * time.Minute,
			ExtraAttr: []string{"group:intel-tbt3-hdmi-dongle"},
		}, {
			Name: "dp_with_tbt_dongle",
			Val: displayParams{
				displayType:    usbutils.TypeCDP,
				iterationValue: 1,
			},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-tbt3-dp-dongle"},
		}},
	})
}

func HDMIDisplayFunctionalityAfterLidCloseOpen(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	// Config file which contains expected values of USB4 parameters.
	const testConfig = "test_config.json"

	// TBT port ID in the DUT.
	dutPort := s.RequiredVar("typec.dutTbtPort")
	// cswitch port ID.
	cSwitchON := s.RequiredVar("typec.cSwitchPort")
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("typec.domainIP")
	tbtPort := s.RequiredVar("typec.tbtDisplayPort")

	servoSpec := s.RequiredVar("servo")
	testOpt := s.Param().(displayParams)
	dut := s.DUT()

	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Connect to gRPC server
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	// Send key file to DUT.
	keyPath := filepath.Join("/tmp", "testcert.p12")
	defer dut.Conn().CommandContext(cleanupCtx, "rm", keyPath).Run()

	testcertKeyPath := map[string]string{s.DataPath("testcert.p12"): keyPath}
	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), testcertKeyPath, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote data path %v: %v", keyPath, err)
	}

	// Login to Chrome.
	client := typec.NewServiceClient(cl.Conn)
	if _, err = client.NewChromeLoginWithPeripheralDataAccess(ctx, &typec.KeyPath{Path: keyPath}); err != nil {
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
	defer dut.Conn().CommandContext(cleanupCtx, "sh", "-c", fmt.Sprintf("rm -rf %s %s %s", htmlPath, videoPath, jsPath)).Run()

	// Read json config file.
	jsonData, err := ioutil.ReadFile(s.DataPath(testConfig))
	if err != nil {
		s.Fatalf("Failed to open %v file : %v", testConfig, err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		s.Fatal("Failed to read json: ", err)
	}

	// Checking for TBT config data.
	deviceVal, ok := data["TBT"].(map[string]interface{})
	if !ok {
		s.Fatal("Failed to find TBT config data in JSON file")
	}

	// Create C-Switch session that performs hot plug-unplug on TBT device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create sessionID: ", err)
	}

	cSwitchOFF := "0"
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Error("Failed to power on DUT at cleanup: ", err)
			}
		}

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}

		if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
			s.Error("Failed to close sessionID: ", err)
		}
	}(cleanupCtx)

	if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchON, domainIP); err != nil {
		s.Fatal("Failed to enable c-switch port: ", err)
	}

	connected, err := typecutils.IsDeviceEnumerated(ctx, dut, deviceVal["device_name"].(string), dutPort)
	if err != nil && !connected {
		s.Fatal("Failed to enumerate the TBT device: ", err)
	}

	if testOpt.displayType == "TBT_display" {
		connected, err := typecutils.IsDeviceEnumerated(ctx, dut, deviceVal["device_detection"].(string), tbtPort)
		if err != nil && !connected {
			s.Fatal("Failed to enumerate the TBT device: ", err)
		}
	}
	connected, err = typecutils.VerifyTXSpeed(ctx, dut, deviceVal["tx_speed"].(string), dutPort)
	if err != nil && !connected {
		s.Fatal("Failed to enumerate tx speed: ", err)
	}

	connected, err = typecutils.VerifyRXSpeed(ctx, dut, deviceVal["rx_speed"].(string), dutPort)
	if err != nil && !connected {
		s.Fatal("Failed to enumerate rx speed: ", err)
	}

	if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "TBT=1"); err != nil {
		s.Fatal("Failed to verify dmesg log: ", err)
	}

	numberOfDisplay := 1
	spec := usbutils.DisplaySpec{
		NumberOfDisplays: &numberOfDisplay,
		DisplayType:      testOpt.displayType,
		DisplayRes:       "4K",
	}
	if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, spec); err != nil {
		s.Fatalf("Failed to detect external 4K %s display: %v", testOpt.displayType, err)
	}

	display, err := client.DisplayName(ctx, &typec.KeyPath{DisplayInfo: 1})
	if err != nil {
		s.Fatal("Failed to get display name: ", err)
	}
	displayName := display.DisplayName

	setMirrorMode := true
	if _, err := client.SetMirrorModeDisplay(ctx, &typec.KeyPath{SetMode: setMirrorMode}); err != nil {
		s.Fatal("Failed to set DUT to mirror mode: ", err)
	}
	if _, err := client.VerifyMirrorMode(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to verify mirror mode: ", err)
	}

	expectedAudioUINode := "Speaker (internal)"
	expectedAudioNode := "INTERNAL_SPEAKER"
	isVerifyRouting := true
	if err := setAudioNodeAndCheckRouting(ctx, cl, expectedAudioNode, expectedAudioUINode, !isVerifyRouting); err != nil {
		s.Fatal("Failed to set audio node and check routing: ", err)
	}

	if _, err := client.VerifyMirrorMode(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to verify mirror mode: ", err)
	}

	videoFile := "1080p_60fps_600frames.vp8.webm"
	if _, err = client.PlayLocalVideo(ctx, &typec.KeyPath{Path: videoFile}); err != nil {
		s.Fatal(s, "Failed to play local video: ", err)
	}

	for i := 1; i <= testOpt.iterationValue; i++ {
		s.Logf("Iteration: %d/%d", i, testOpt.iterationValue)
		if err := pxy.Servo().CloseLid(ctx); err != nil {
			s.Fatal("Failed to close lid: ", err)
		}

		// GoBigSleepLint: After closing lid and before playing video, expected 2s of sleep
		if err := testing.Sleep(ctx, 2*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		if _, err = client.PlayVideo(ctx, &typec.KeyPath{Path: videoFile}); err != nil {
			s.Fatal(s, "Failed to play video again: ", err)
		}

		expectedAudioUINode = displayName + " (HDMI/DP)"
		expectedAudioNode = "HDMI"
		if err := setAudioNodeAndCheckRouting(ctx, cl, expectedAudioNode, expectedAudioUINode, isVerifyRouting); err != nil {
			s.Fatal("Failed to set audio node and check routing: ", err)
		}

		if _, err := client.VerifyMirrorMode(ctx, &empty.Empty{}); err == nil {
			s.Fatal("Failed to verify mirror mode: ", err)
		}

		if err := pxy.Servo().OpenLid(ctx); err != nil {
			s.Error("Failed to lid open: ", err)
		}

		if _, err := client.VerifyMirrorMode(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to verify mirror mode: ", err)
		}

		expectedAudioUINode = "Speaker (internal)"
		expectedAudioNode = "INTERNAL_SPEAKER"
		if err := setAudioNodeAndCheckRouting(ctx, cl, expectedAudioNode, expectedAudioUINode, isVerifyRouting); err != nil {
			s.Fatal("Failed to set audio node and check routing: ", err)
		}
	}

	if testOpt.displayType != "HDMI" {
		if _, err := client.SetMirrorModeDisplay(ctx, &typec.KeyPath{SetMode: !setMirrorMode}); err != nil {
			s.Fatal("Failed to unset mirror mode: ", err)
		}

		if _, err = client.PlayVideo(ctx, &typec.KeyPath{Path: videoFile}); err != nil {
			s.Fatal(s, "Failed to play video again: ", err)
		}

		if _, err = client.VerifyEdpPrimary(ctx, &empty.Empty{}); err != nil {
			s.Fatal(s, "Failed to verify extended mode: ", err)
		}

		if _, err = client.VerifyWindowOnDisplay(ctx, &typec.KeyPath{DisplayInfo: 0}); err != nil {
			s.Fatal(s, "Failed to verify window on primary display: ", err)
		}
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
			return errors.Wrapf(err, "failed to route audio through %q", expectedAudioOuputUINode)
		}
	}
	return nil
}
