// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/setup"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendResumeWithTBTAltMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Switch between TBT mode alt mode and DP alt mode after suspend-resume power state using 40G passive cable",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel.
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:typec", "group:intel-tbt3-dock"},
		ServiceDeps:  []string{"tast.cros.typec.Service"},
		Data:         []string{"test_config.json", "testcert.p12"},
		VarDeps:      []string{"servo", "typec.dutTbtPort", "typec.cSwitchPort", "typec.domainIP"},
		HardwareDeps: hwdep.D(setup.ThunderboltSupportedDevices()),
		Timeout:      15 * time.Minute,
	})
}

// Config file which contains expected values of TBT parameters.
const testConfig = "test_config.json"

func SuspendResumeWithTBTAltMode(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	var typeCHDMIRe = regexp.MustCompile(`\[CONNECTOR:\d+:DP.*status: connected`)
	typeCDisplayInfoPatterns := []*regexp.Regexp{typeCHDMIRe}

	// TBT port ID in the DUT.
	dutPort := s.RequiredVar("typec.dutTbtPort")
	// cswitch port ID.
	cSwitchOn := s.RequiredVar("typec.cSwitchPort")
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("typec.domainIP")

	servoSpec := s.RequiredVar("servo")

	dut := s.DUT()

	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Send key file to DUT.
	keyPath := "/tmp/testcert.p12"
	defer dut.Conn().CommandContext(cleanupCtx, "rm", keyPath).Run()

	testcertKeyPath := map[string]string{s.DataPath("testcert.p12"): keyPath}
	loginChrome := func() {
		// Connect to gRPC server.
		cl, err := rpc.Dial(ctx, dut, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(cleanupCtx)

		if _, err := linuxssh.PutFiles(ctx, dut.Conn(), testcertKeyPath, linuxssh.DereferenceSymlinks); err != nil {
			s.Fatalf("Failed to send data to remote data path %v: %v", keyPath, err)
		}

		// Login to Chrome.
		client := typec.NewServiceClient(cl.Conn)
		if _, err = client.NewChromeLoginWithPeripheralDataAccess(ctx, &typec.KeyPath{Path: keyPath}); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
	}

	loginChrome()

	// Read json config file.
	jsonData, err := ioutil.ReadFile(s.DataPath(testConfig))
	if err != nil {
		s.Fatalf("Failed to open %v file : %v", testConfig, err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		s.Fatal("Failed to read json: ", err)
	}

	// Checking for USB4 config data.
	deviceVal, ok := data["TBT"].(map[string]interface{})
	if !ok {
		s.Fatal("Failed to find TBT config data in JSON file")
	}

	// Create C-Switch session that performs hot plug-unplug on TBT device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create sessionID: ", err)
	}

	const cSwitchOff = "0"
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Error("Failed to power on DUT at cleanup: ", err)
			}
		}

		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOff, domainIP); err != nil {
			s.Error("Failed to disable c-switch port: ", err)
		}

		if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
			s.Error("Failed to close sessionID: ", err)
		}
	}(cleanupCtx)

	if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOn, domainIP); err != nil {
		s.Fatal("Failed to enable c-switch port: ", err)
	}

	var portNum string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		portNum, err = typecutils.CableConnectedPortNumber(ctx, dut, "TBT")
		if err != nil {
			return errors.Wrap(err, "failed to get TBT connected port number")
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

	expectedOut := "Cable Type: Passive"
	actualOut := out[0][0]
	if actualOut != expectedOut {
		s.Fatalf("Failed: Unexpected cable type, want %q; got %q", expectedOut, actualOut)
	}

	verifyPeripheralDevices := func() {
		connected, err := typecutils.IsDeviceEnumerated(ctx, dut, deviceVal["device_name"].(string), dutPort)
		if err != nil || !connected {
			s.Fatal("Failed to enumerate the TBT device: ", err)
		}

		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "TBT=1"); err != nil {
			s.Fatal("Failed to TBT alt mode: ", err)
		}

		if err := usbutils.ExternalDisplayDetectionForRemote(ctx, dut, 1, typeCDisplayInfoPatterns); err != nil {
			s.Fatal("Failed to detect external HDMI display: ", err)
		}

		if err := verifyPendrive(ctx, dut); err != nil {
			s.Fatal("Failed to verify pendrive speed: ", err)
		}
	}

	verifyPeripheralDevices()

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(cmdCtx, "ectool", "reboot_ec").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		s.Fatal("Failed to execute ectool reboot command: ", err)
	}
	unreachCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(unreachCtx); err != nil {
		s.Fatal("Failed to wait unreach DUT: ", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := dut.WaitConnect(waitCtx); err != nil {
		s.Fatal("Failed to wait connect DUT: ", err)
	}

	loginChrome()

	verifyPeripheralDevices()

	rebootCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(rebootCtx, "ectool", "reboot_ec").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		s.Fatal("Failed to execute ectool reboot command: ", err)
	}

	unreachCtx1, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(unreachCtx1); err != nil {
		s.Fatal("Failed to wait unreach DUT: ", err)
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to wait connect DUT: ", err)
	}

	loginChrome()

	verifyPeripheralDevices()

	nIterations := 5
	for i := 1; i <= nIterations; i++ {
		s.Logf("Iteration:%d/%d", i, nIterations)

		if err := suspendDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to suspend DUT: ", err)
		}

		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "TBT=1"); err != nil {
			s.Fatal("Failed to verify TBT alt mode: ", err)
		}

		if err := dpAltMode(ctx, portNum, dut); err != nil {
			s.Fatal("Failed to enter dpAlt mode: ", err)
		}

		if err := suspendDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to suspend DUT: ", err)
		}

		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "DP=1"); err != nil {
			s.Fatal("Failed to verify TBT alt mode: ", err)
		}
		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "USB=1"); err != nil {
			s.Fatal("Failed to verify TBT alt mode: ", err)
		}

		if err := tbtAltMode(ctx, portNum, dut); err != nil {
			s.Fatal("Failed to enter TBT alt mode: ", err)
		}

	}
}

// exitAllMode function exits DUT from all mode(tbt alt or dp alt mode) and verifies it.
func exitAllMode(ctx context.Context, typecPortNum string, dut *dut.DUT) error {
	if err := dut.Conn().CommandContext(ctx, "ectool", "typeccontrol", typecPortNum, "0").Run(); err != nil {
		return errors.Wrap(err, "failed to run TBT Alt Mode command")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Verifies exit from TBT alt mode.
		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "TBT=0"); err != nil {
			return errors.Wrap(err, "failed to exit from TBT alt mode")
		}
		// Verifies exit from DP alt mode.
		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "DP=0"); err != nil {
			return errors.Wrap(err, "failed to exit from DP alt mode")
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to get cable connected port number")
	}
	return nil
}

// tbtAltMode function switches DUT to tbt alt mode and verifies it.
func tbtAltMode(ctx context.Context, typecPortNum string, dut *dut.DUT) error {
	if err := exitAllMode(ctx, typecPortNum, dut); err != nil {
		return errors.Wrap(err, "failed to exit all mode")
	}

	if err := dut.Conn().CommandContext(ctx, "ectool", "typeccontrol", typecPortNum, "2", "1").Run(); err != nil {
		return errors.Wrap(err, "failed to run TBT alt Mode command")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "TBT=1"); err != nil {
			return errors.Wrap(err, "failed to TBT alt mode")
		}

		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to verify TBT Alt mode")
	}
	return nil
}

// dpAltMode function switches DUT to dp alt mode and verifies it.
func dpAltMode(ctx context.Context, typecPortNum string, dut *dut.DUT) error {
	if err := exitAllMode(ctx, typecPortNum, dut); err != nil {
		return errors.Wrap(err, "failed to exit all mode")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {

		if err := dut.Conn().CommandContext(ctx, "ectool", "typeccontrol", typecPortNum, "2", "0").Run(); err != nil {
			return errors.Wrap(err, "failed to run DP alt Mode command")
		}

		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "DP=1"); err != nil {
			return errors.Wrap(err, "failed to DP alt mode")
		}
		if err := typecutils.CheckUSBPdMuxinfo(ctx, dut, "USB=1"); err != nil {
			return errors.Wrap(err, "failed to DP alt mode")
		}

		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to verify DP alt mode")
	}
	return nil
}

// verifyPendrive function verifies USB3.0 pendrive.
func verifyPendrive(ctx context.Context, dut *dut.DUT) error {
	const (
		usbDeviceClassName = "Mass Storage"
		usbSpeed           = "5000M"
	)
	usbDevicesList, err := usbutils.ListDevicesInfo(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices list")
	}
	got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, usbDeviceClassName, usbSpeed)
	if want := 1; got != want {
		return errors.Errorf("unexpected number of USB devices connected: got %d, want %d", got, want)
	}
	return nil
}

// suspendDUT function suspends DUT.
func suspendDUT(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT) error {
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
	}, &testing.PollOptions{Timeout: 40 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}

	if err := pxy.Servo().KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press power button through servo")
	}

	wakeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitConnect(wakeCtx); err != nil {
		return errors.Wrap(err, "failed to wait connect DUT")
	}
	return nil
}
