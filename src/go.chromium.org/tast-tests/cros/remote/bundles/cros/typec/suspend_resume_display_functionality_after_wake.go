// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
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

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendResumeDisplayFunctionalityAfterWake,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies display connect via TBT dongle and then connect TBT dongle at suspend mode, and check functionality after the wake",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:typec"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Data:         []string{"testcert.p12", "1080p_60fps_600frames.vp8.webm", "video.html", "test_config.json", "playback.js"},
		VarDeps:      []string{"servo", "typec.dutTbtPort", "typec.cSwitchPort", "typec.domainIP"},
		HardwareDeps: hwdep.D(setup.ThunderboltSupportedDevices()),
		Timeout:      8 * time.Minute,
		Params: []testing.Param{{
			Name:      "typec_hdmi",
			Val:       `.*DP branch device present.*yes\n.*Type.*HDMI`,
			ExtraAttr: []string{"group:intel-tbt3-hdmi-dongle"},
		}, {
			Name:      "typec_dp",
			Val:       `\[CONNECTOR:\d+:DP.*status: connected((.|\n)*)DP branch device present: no`,
			ExtraAttr: []string{"group:intel-tbt3-dp-dongle"},
		}},
	})
}

// SuspendResumeDisplayFunctionalityAfterWake requires the following H/W topology to run.
// DUT ---> C-Switch(device that performs hot plug-unplug) --
// ---> TBT Dongle ---> Typec HDMI/DP Display.
func SuspendResumeDisplayFunctionalityAfterWake(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	// Config file which contains expected values of TBT parameters.
	const testConfig = "test_config.json"

	// TBT port ID in the DUT.
	dutPort := s.RequiredVar("typec.dutTbtPort")
	// cswitch port ID.
	cSwitchON := s.RequiredVar("typec.cSwitchPort")
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("typec.domainIP")

	servoSpec := s.RequiredVar("servo")
	testOpt := s.Param().(string)
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
	_, err = client.NewChromeLoginWithPeripheralDataAccess(ctx, &typec.KeyPath{Path: keyPath})
	if err != nil {
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

	connected, err = typecutils.VerifyTXSpeed(ctx, dut, deviceVal["tx_speed"].(string), dutPort)
	if err != nil && !connected {
		s.Fatal("Failed to enumerate the TBT device TX speed: ", err)
	}

	connected, err = typecutils.VerifyRXSpeed(ctx, dut, deviceVal["rx_speed"].(string), dutPort)
	if err != nil && !connected {
		s.Fatal("Failed to enumerate the TBT device RX speed: ", err)
	}

	if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "TBT=1"); err != nil {
		s.Fatal("Failed to verify dmesg log: ", err)
	}

	displayDetectionRe := regexp.MustCompile(testOpt)
	hdmi4kRe := regexp.MustCompile("3840x2160|4096x2160")
	typeCDisplayInfoPatterns := []*regexp.Regexp{displayDetectionRe, hdmi4kRe}
	if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, 1, typeCDisplayInfoPatterns); err != nil {
		s.Fatal("Failed to detect external 4K HDMI display: ", err)
	}

	if _, err := client.SetMirrorModeDisplay(ctx, &typec.KeyPath{SetMode: true}); err != nil {
		s.Fatal("Failed to set mirror mode: ", err)
	}

	if _, err := client.VerifyMirrorMode(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to verify mirror mode: ", err)
	}

	videoFile := "1080p_60fps_600frames.vp8.webm"
	if _, err = client.PlayLocalVideo(ctx, &typec.KeyPath{Path: videoFile}); err != nil {
		s.Fatal(s, "Failed to play local video: ", err)
	}

	slpOpSetPre, pkgOpSetPre, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get SLP counter and C10 package values before suspend-resume: ", err)
	}
	numIterations := 10
	for i := 1; i <= numIterations; i++ {
		s.Logf("Iteration:%d/%d", i, numIterations)
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			suspendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := dut.Conn().CommandContext(suspendCtx, "powerd_dbus_suspend").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return errors.Wrap(err, "failed to power off DUT")
			}

			sCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := dut.WaitUnreachable(sCtx); err != nil {
				return errors.Wrap(err, "failed to wait for unreachable")
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
			s.Fatal("Failed to suspend DUT: ", err)
		}

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchON, domainIP); err != nil {
			s.Fatal("Failed to enable c-switch port: ", err)
		}

		if err := pxy.Servo().KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
			s.Fatal("Failed to press power button through servo: ", err)
		}

		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := dut.WaitConnect(waitCtx); err != nil {
			s.Fatal("Failed to wait connect DUT: ", err)
		}

		connected, err = typecutils.IsDeviceEnumerated(ctx, dut, deviceVal["device_name"].(string), dutPort)
		if err != nil && !connected {
			s.Fatal("Failed to enumerate the TBT device: ", err)
		}

		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "TBT=1"); err != nil {
			s.Fatal("Failed to verify dmesg log: ", err)
		}

		if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, 1, typeCDisplayInfoPatterns); err != nil {
			s.Fatal("Failed to detect external HDMI display: ", err)
		}
	}

	slpOpSetPost, pkgOpSetPost, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get SLP counter and C10 package values before suspend-resume: ", err)
	}

	if err := powercontrol.AssertSLPAndC10(slpOpSetPre, slpOpSetPost, pkgOpSetPre, pkgOpSetPost); err != nil {
		s.Fatal("Failed to assert SLP and package C10 values after suspend-resume: ", err)
	}

}
