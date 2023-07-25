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

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

type suspendResumeParams struct {
	device          string
	verifyHeadphone bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendResumeUSB4PlugUnplug,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies USB4 docking station using 40G passive cable: Insert before suspend, unplug during suspend, insert back in suspend, then resume",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:typec"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Data:         []string{"test_config.json", "testcert.p12"},
		VarDeps:      []string{"servo", "typec.dutTbtPort", "typec.cSwitchPort", "typec.domainIP"},

		Params: []testing.Param{{
			Name: "tbt_dock",
			Val: suspendResumeParams{
				device:          "TBT",
				verifyHeadphone: false,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-tbt3-bp-dock"},
		}, {
			Name: "usb4_dock",
			Val: suspendResumeParams{
				device:          "USB4",
				verifyHeadphone: true,
			},
			Timeout:   15 * time.Minute,
			ExtraAttr: []string{"group:intel-tbt4-dock"},
		}},
	})
}

func SuspendResumeUSB4PlugUnplug(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	// Config file which contains expected values of USB4 parameters.
	const testConfig = "test_config.json"

	var typeCHDMIRe = regexp.MustCompile(`.*DP branch device present.*yes\n.*Type.*HDMI`)
	typeCDisplayInfoPatterns := []*regexp.Regexp{typeCHDMIRe}

	// TBT port ID in the DUT.
	dutPort := s.RequiredVar("typec.dutTbtPort")
	// cswitch port ID.
	cSwitchON := s.RequiredVar("typec.cSwitchPort")
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("typec.domainIP")

	servoSpec := s.RequiredVar("servo")
	testOpt := s.Param().(suspendResumeParams)
	dut := s.DUT()

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

	// Read json config file.
	jsonData, err := ioutil.ReadFile(s.DataPath(testConfig))
	if err != nil {
		s.Fatalf("Failed to open %v file : %v", testConfig, err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		s.Fatal("Failed to read json: ", err)
	}

	// Checking for TBT/USB4 config data.
	deviceVal, ok := data[testOpt.device].(map[string]interface{})
	if !ok {
		s.Fatalf("Failed to find %s config data in JSON file", testOpt.device)
	}

	// Create C-Switch session that performs hot plug-unplug on TBT device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create sessionID: ", err)
	}

	cSwitchOFF := "0"
	defer func(ctx context.Context) {
		s.Log("Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Error("Failed to power on DUT in cleanup: ", err)
			}
		}

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}

		if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
			s.Error("Failed to close sessionID: ", err)
		}
	}(cleanupCtx)

	iterationValue := 10
	for i := 1; i <= iterationValue; i++ {

		s.Logf("Iteration: %d/%d", i, iterationValue)
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchON, domainIP); err != nil {
			s.Fatal("Failed to enable c-switch port: ", err)
		}

		if _, err := typecutils.IsDeviceEnumerated(ctx, dut, deviceVal["device_name"].(string), dutPort); err != nil {
			s.Fatal("Failed to enumerate the TBT device: ", err)
		}

		var portNum string
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			portNum, err = typecutils.CableConnectedPortNumber(ctx, dut, testOpt.device)
			if err != nil {
				return errors.Wrapf(err, "failed to get %s connected port number", testOpt.device)
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
			s.Fatal("Failed to get cable connected port number: ", err)
		}
		pdCableCommand := fmt.Sprintf("pdcable %s", portNum)
		ecPatterns := []string{"Cable Type: " + "Passive"}
		out, err := pxy.Servo().RunECCommandGetOutput(ctx, pdCableCommand, ecPatterns)

		if err != nil {
			s.Fatal("Failed to run EC command: ", err)
		}

		expectedOut := "Cable Type: " + "Passive"
		actualOut := out[0][0]
		if actualOut != expectedOut {
			s.Fatalf("Unexpected cable type. Want %q; got %q", expectedOut, actualOut)
		}

		_, err = typecutils.VerifyTXSpeed(ctx, dut, deviceVal["tx_speed"].(string), dutPort)
		if err != nil {
			s.Fatal("Failed to enumerate TBT device TX speed: ", err)
		}

		_, err = typecutils.VerifyRXSpeed(ctx, dut, deviceVal["rx_speed"].(string), dutPort)
		if err != nil {
			s.Fatal("Failed to enumerate TBT device RX speed: ", err)
		}

		if testOpt.verifyHeadphone {
			if err := verifyHeadphoneDetection(ctx, dut); err != nil {
				s.Fatal("Failed to verify headphone: ", err)
			}
		}

		if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, 1, typeCDisplayInfoPatterns); err != nil {
			s.Fatal("Failed to detect external HDMI display: ", err)
		}

		slpOpSetPre, pkgOpSetPre, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get SLP counter and C10 package values before suspend-resume: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			suspendCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if err := dut.Conn().CommandContext(suspendCtx, "powerd_dbus_suspend").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return errors.Wrap(err, "failed to power off DUT")
			}

			unreachCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := dut.WaitUnreachable(unreachCtx); err != nil {
				return errors.Wrap(err, "failed to wait for unreachable")
			}
			return nil
		}, &testing.PollOptions{Timeout: 25 * time.Second, Interval: 1 * time.Second}); err != nil {
			s.Fatal("Failed to verify system power state: ", err)
		}

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}

		waitCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
		defer cancel()
		if err := dut.WaitConnect(waitCtx); err != nil {
			s.Fatal("Failed to wait connect DUT: ", err)
		}

		slpOpSetPost1, pkgOpSetPost1, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get SLP counter and C10 package values after suspend-resume: ", err)
		}

		if slpOpSetPre == slpOpSetPost1 {
			s.Fatalf("Failed: SLP counter value %q should be different from the one before suspend %q", slpOpSetPost1, slpOpSetPre)
		}

		if slpOpSetPost1 == 0 {
			s.Fatal("Failed SLP counter value must be non-zero, got: ", slpOpSetPost1)
		}

		if pkgOpSetPre == pkgOpSetPost1 {
			s.Fatalf("Failed: Package C10 value %q must be different from the one before suspend %q", pkgOpSetPost1, pkgOpSetPre)
		}

		if pkgOpSetPost1 == "0x0" || pkgOpSetPost1 == "0" {
			s.Fatal("Failed: Package C10 should be non-zero")
		}

		suspendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := dut.Conn().CommandContext(suspendCtx, "powerd_dbus_suspend").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			s.Fatal("Failed to power off DUT: ", err)
		}

		unreachCtx1, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if err := dut.WaitUnreachable(unreachCtx1); err != nil {
			s.Fatal("Failed to wait for unreachable: ", err)
		}

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchON, domainIP); err != nil {
			s.Fatal("Failed to enable c-switch port: ", err)
		}

		if testOpt.device == "TBT" {
			shCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := dut.WaitUnreachable(shCtx); err != nil {
				s.Fatal("Failed to wait for unreachable: ", err)
			}

			if err := pxy.Servo().KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
				s.Fatal("Failed to press power through servo: ", err)
			}
		}

		if testOpt.device == "USB4" {
			waitCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()
			if err := dut.WaitConnect(waitCtx); err != nil {
				s.Fatal("Failed to wait connect DUT: ", err)
			}
		}

		slpOpSetPost2, pkgOpSetPost2, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get SLP counter and C10 package values after suspend-resume: ", err)
		}

		if slpOpSetPost1 == slpOpSetPost2 {
			s.Fatalf("Failed: SLP counter value %q should be different from the one before suspend %q", slpOpSetPost2, slpOpSetPost1)
		}

		if slpOpSetPost2 == 0 {
			s.Fatal("Failed SLP counter value must be non-zero, got: ", slpOpSetPost2)
		}

		if pkgOpSetPost1 == pkgOpSetPost2 {
			s.Fatalf("Failed: Package C10 value %q must be different from the one before suspend %q", pkgOpSetPost2, pkgOpSetPost1)
		}

		if pkgOpSetPost2 == "0x0" || pkgOpSetPost2 == "0" {
			s.Fatal("Failed: Package C10 should be non-zero")
		}

		_, err = typecutils.IsDeviceEnumerated(ctx, dut, deviceVal["device_name"].(string), dutPort)
		if err != nil {
			s.Fatal("Failed to enumerate the TBT device: ", err)
		}

		_, err = typecutils.VerifyTXSpeed(ctx, dut, deviceVal["tx_speed"].(string), dutPort)
		if err != nil {
			s.Fatal("Failed to enumerate TBT device TX speed: ", err)
		}

		_, err = typecutils.VerifyRXSpeed(ctx, dut, deviceVal["rx_speed"].(string), dutPort)
		if err != nil {
			s.Fatal("Failed to enumerate TBT device RX speed: ", err)
		}

		if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, 1, typeCDisplayInfoPatterns); err != nil {
			s.Fatal("Failed to detect external HDMI display: ", err)
		}
		if testOpt.verifyHeadphone {
			if err := verifyHeadphoneDetection(ctx, dut); err != nil {
				s.Fatal("Failed to verify headphone: ", err)
			}
		}

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}
	}
}

// verifyHeadphoneDetection will verify whether headphones are connect or not.
func verifyHeadphoneDetection(ctx context.Context, dut *dut.DUT) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "cras_test_client").Output()
		if err != nil {
			return errors.Wrap(err, "failed to execute cros_test_client command")
		}
		found := regexp.MustCompile(" yes.*USB").MatchString(string(out))
		if !found {
			return errors.New("failed to verify 3.5mm detection")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to detect 3.5 mm headphone")
	}
	return nil
}
