// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/remote/tabletmode"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast-tests/cros/services/cros/security"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type bootupTimes struct {
	bootType   string
	tabletMode bool
}

const (
	reboot       string = "reboot"
	vt2Reboot    string = "vt2Reboot"
	lidCloseOpen string = "lidCloseOpen"
	powerButton  string = "powerButton"
	bootFromS5   string = "bootFromS5"
	refreshPower string = "refreshPower"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BootupTimes,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Boot performance test after reboot, powerbutton and lid close open",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		// Disabled due to 98%-99% failure rate and preventing other tests from running. TODO(b/242478571): fix and re-enable.
		//Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.arc.PerfBootService", "tast.cros.platform.BootPerfService", "tast.cros.security.BootLockboxService", "tast.cros.inputs.KeyboardService"},
		Vars: []string{"servo",
			"platform.BootupTimes.bootTime",
			"platform.BootupTimes.cbmemTimeout",
		},
		Params: []testing.Param{{
			Name:      "reboot",
			Val:       bootupTimes{bootType: reboot},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-sleep"},
		}, {
			Name:      "reboot_tablet_mode",
			Val:       bootupTimes{bootType: reboot, tabletMode: true},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-convertible"},
		}, {
			Name:      "vt2_reboot",
			Val:       bootupTimes{bootType: vt2Reboot},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-sleep"},
		}, {
			Name:      "lid_close_open",
			Val:       bootupTimes{bootType: lidCloseOpen},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-sleep"},
		}, {
			Name:      "power_button",
			Val:       bootupTimes{bootType: powerButton},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-sleep"},
		}, {
			Name:      "power_button_tablet_mode",
			Val:       bootupTimes{bootType: powerButton, tabletMode: true},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"group:intel-convertible"},
		}, {
			Name:              "from_s5",
			Val:               bootupTimes{bootType: bootFromS5},
			Timeout:           5 * time.Minute,
			ExtraHardwareDeps: hwdep.D(hwdep.ChromeEC()),
			ExtraAttr:         []string{"group:intel-sleep"},
		}, {
			Name:              "refresh_power",
			Val:               bootupTimes{bootType: refreshPower},
			Timeout:           5 * time.Minute,
			ExtraHardwareDeps: hwdep.D(hwdep.ChromeEC()),
			ExtraAttr:         []string{"group:intel-sleep"},
		}},
	})
}

func BootupTimes(ctx context.Context, s *testing.State) {
	var (
		bootTime     = 8.4  // default bootup time in seconds
		cbmemTimeout = 1.35 // default cbmem timeout in seconds
	)
	dut := s.DUT()
	btType := s.Param().(bootupTimes)

	bootupTime, ok := s.Var("platform.BootupTimes.bootTime")
	if !ok {
		s.Log("Default Boot Time for validation: ", bootTime)
	} else {
		btime, err := strconv.ParseFloat(bootupTime, 8)
		if err != nil {
			s.Fatal("Failed to convert boot time: ", err)
		}
		bootTime = btime
		s.Log("Boot Time for validation: ", bootTime)
	}

	cbmemtime, ok := s.Var("platform.BootupTimes.cbmemTimeout")
	if !ok {
		s.Log("Default Cbmem Timeout for validation: ", cbmemTimeout)
	} else {
		cbmtime, err := strconv.ParseFloat(cbmemtime, 8)
		if err != nil {
			s.Fatal("Failed to convert cbmemtime: ", err)
		}
		cbmemTimeout = cbmtime
		s.Log("Cbmem Timeout for validation: ", cbmemTimeout)
	}
	pxy, err := servo.NewProxy(ctx, s.RequiredVar("servo"), dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctx)

	tmc := &tabletmode.ConvertibleModeControl{}
	if err := tmc.InitControl(ctx, dut); err != nil {
		s.Fatal("Failed to init TabletModeControl: ", err)
	}

	if btType.tabletMode {
		// Force DUT into tablet mode.
		testing.ContextLog(ctx, "Put DUT into tablet mode")
		if err := tmc.ForceTabletMode(ctx); err != nil {
			s.Fatal("Failed to set DUT into tablet mode: ", err)
		}
	}

	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Connect to the gRPC server on the DUT.
	// Perform a Chrome login.
	// Chrome login excluding for lidCloseOpen.
	if btType.bootType != "lidCloseOpen" {
		client := security.NewBootLockboxServiceClient(cl.Conn)
		if _, err := client.NewChromeLogin(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to start Chrome")
		}
	}
	// Enable bootchart before running the boot perf test.
	bootPerfService := platform.NewBootPerfServiceClient(cl.Conn)
	s.Log("Enabling boot chart")
	_, err = bootPerfService.EnableBootchart(ctx, &empty.Empty{})
	if err != nil {
		// If we failed in enabling bootchart, log the failure and proceed without bootchart.
		s.Log("Warning: failed to enable bootchart: ", err)
	}
	// Stop tlsdated, that makes sure nobody will touch the RTC anymore, and also creates a sync-rtc bootstat file.
	if err := dut.Conn().CommandContext(ctx, "stop", "tlsdated").Run(); err != nil {
		s.Fatal("Failed to stop tlsdated: ", err)
	}
	defer dut.Conn().CommandContext(ctx, "start", "tlsdated").Run()

	// Undo the effect of enabling bootchart. This cleanup can also be performed (becomes a no-op) if bootchart is not enabled.
	defer func() {
		// Restore the side effect made in this test by disabling bootchart for subsequent system boots.
		s.Log("Disable bootchart")
		chl, err := rpc.Dial(ctx, dut, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer chl.Close(ctx)

		bootPerfService := platform.NewBootPerfServiceClient(chl.Conn)
		_, err = bootPerfService.DisableBootchart(ctx, &empty.Empty{})
		if err != nil {
			s.Log("Error in disabling bootchart: ", err)
		}

		testing.ContextLog(ctx, "Resetting tabletmode")
		if err := tmc.Reset(ctx); err != nil {
			s.Fatal("Failed to restore tabletmode to the original settings: ", err)
		}

	}()

	// Cleanup.
	defer func(ctx context.Context) {
		s.Log("Performing clean up")
		if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
			s.Error("Failed to press power button: ", err)
		}
	}(ctx)

	if btType.bootType == "reboot" {
		s.Log("Rebooting DUT")
		if err := dut.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot DUT: ", err)
		}
	} else if btType.bootType == "lidCloseOpen" {
		s.Log("Closing lid")
		if err := pxy.Servo().SetString(ctx, "lid_open", "no"); err != nil {
			s.Fatal("Unable to close lid : ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			pwrState, err := pxy.Servo().GetECSystemPowerState(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get power state S5 error")
			}
			if pwrState != "S5" {
				return errors.Errorf("System is not in S5, got: %s", pwrState)
			}
			return nil
		}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
			s.Fatal("Failed to enter S5 state : ", err)
		}
		if err := pxy.Servo().SetString(ctx, "lid_open", "yes"); err != nil {
			s.Fatal("Failed to open lid: ", err)
		}
		if err := dut.WaitConnect(ctx); err != nil {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to press power button: ", err)
			}
		}
	} else if btType.bootType == "powerButton" {
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", "rm -rf /var/log/metrics/*").Run(); err != nil {
			s.Fatal("Failed to remove /var/log/metrics/* files: ", err)
		}
		if err := pxy.Servo().SetString(ctx, "power_key", "long_press"); err != nil {
			s.Fatal("Unable to power state off: ", err)
		}

		if err := dut.WaitUnreachable(ctx); err != nil {
			s.Fatal("Failed to shutdown: ", err)
		}

		if err := powercontrol.ValidateG3PowerState(ctx, pxy); err != nil {
			s.Fatal("Failed to validate G3 power state: ", err)
		}

		if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to press power button: ", err)
		}
	} else if btType.bootType == bootFromS5 {
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", "rm -rf /var/log/metrics/*").Run(); err != nil {
			s.Fatal("Failed to remove /var/log/metrics/* files: ", err)
		}
		// Use the ec command here instead of power_key, because servo sleeps before the command returns
		if err := pxy.Servo().RunECCommand(ctx, "powerbtn 8500"); err != nil {
			s.Fatal("Failed to press power button: ", err)
		}

		if err := waitForS0State(ctx, pxy); err != nil {
			s.Fatal("Failed to wait for S0 state: ", err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := dut.WaitConnect(waitCtx); err != nil {
			s.Fatal("Failed to wait connect DUT: ", err)
		}
	} else if btType.bootType == refreshPower {
		waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		s.Log("Pressing power btn to shutdown DUT")
		if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurLongPress); err != nil {
			s.Fatal("Failed to power off DUT: ", err)
		}

		if err := dut.WaitUnreachable(ctx); err != nil {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to press power button: ", err)
			}
		}

		// GoBigSleepLint: Expected time sleep 5 seconds to ensure DUT switch to s5.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		s.Log("Pressing refresh + power key to boot up DUT")
		if err := pxy.Servo().KeypressWithDuration(ctx, servo.Refresh, servo.DurLongPress); err != nil {
			s.Fatal("Failed to press refresh key: ", err)
		}
		if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			s.Fatal("Failed to power normal press: ", err)
		}
		if err := dut.WaitConnect(waitCtx); err != nil {
			s.Fatal("Failed to wait connect DUT: ", err)
		}
	} else if btType.bootType == vt2Reboot {
		kb := inputs.NewKeyboardServiceClient(cl.Conn)

		if err := openVT2(ctx, kb); err != nil {
			s.Fatal("Failed to open VT2 Terminal: ", err)
		}

		if err := loginVT2(ctx, kb); err != nil {
			s.Fatal("Failed to login VT2 Terminal: ", err)
		}

		if err := rebootViaVT2(ctx, kb); err != nil {
			s.Fatal("Failed to reboot via VT2 Terminal: ", err)
		}

	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := dut.WaitConnect(waitCtx); err != nil {
		s.Fatal("Failed to wait connect DUT: ", err)
	}
	// Validating prev sleep state for power modes.
	if btType.bootType == "reboot" || btType.bootType == vt2Reboot {
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, 0); err != nil {
			s.Fatal("Failed to get previous sleep state: ", err)
		}
	} else {
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, 5); err != nil {
			s.Fatal("Failed to get previous sleep state: ", err)
		}
	}
	if err := getBootPerf(ctx, dut, s.RPCHint(), bootTime); err != nil {
		s.Fatal("Failed to get boot perf values: ", err)
	}
	if err := verifyCBMem(ctx, dut, cbmemTimeout); err != nil {
		s.Fatal("Failed to verify cbmem timeout: ", err)
	}
}

// verifyCBMem verifies cbmem timeout.
func verifyCBMem(ctx context.Context, dut *dut.DUT, cbmemTimeout float64) error {
	cbmemOutput, err := dut.Conn().CommandContext(ctx, "sh", "-c", "cbmem -t").Output()
	if err != nil {
		return errors.Wrap(err, "failed to execute cbmem command")
	}
	timeStampPattern := regexp.MustCompile(`timestamp\s+([\d,]+)`)

	timeStampMatch := timeStampPattern.FindStringSubmatch(string(cbmemOutput))
	timeStampValue := ""
	if len(timeStampMatch) > 1 {
		timeStampValue = strings.Replace(timeStampMatch[1], ",", "", -1)
	}
	timeStamp, err := strconv.ParseFloat(timeStampValue, 8)
	if err != nil {
		return errors.Wrap(err, "failed to convert string value to floating point value")
	}
	timeStamp = timeStamp / 1000000

	cbmemPattern := regexp.MustCompile(`Total Time: (.*)`)
	match := cbmemPattern.FindStringSubmatch(string(cbmemOutput))
	cbmemTotalTime := ""
	if len(match) > 1 {
		cbmemTotalTime = strings.Replace(match[1], ",", "", -1)
	}
	totalCbmemTime, err := strconv.ParseFloat(cbmemTotalTime, 8)
	if err != nil {
		return errors.Wrap(err, "failed to convert string value to floating point value")
	}
	totalCbmemTime = totalCbmemTime / 1000000

	cbmemTime := totalCbmemTime - timeStamp
	if cbmemTime > cbmemTimeout {
		return errors.Wrapf(err, "failed to validate cbmem time, actual cbmem time is more than expected cbmem time, want %v; got %v", cbmemTimeout, cbmemTime)
	}
	return nil
}

// getBootPerf validates seconds power on to login from platform bootperf values.
func getBootPerf(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, btime float64) error {
	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	bootPerfService := platform.NewBootPerfServiceClient(cl.Conn)
	metrics, err := bootPerfService.GetBootPerfMetrics(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get boot perf metrics")
	}
	if metrics.Metrics["seconds_power_on_to_login"] > btime {
		return errors.Wrapf(err, "failed seconds_power_on_to_login is greater than expected, want %v; got %v", btime, metrics.Metrics["seconds_power_on_to_login"])
	}
	return nil
}

// waitForS0State waits for S0 power state
func waitForS0State(ctx context.Context, pxy *servo.Proxy) (retErr error) {
	var leftoverLines string
	readyForPowerOn := regexp.MustCompile(`power state \d = S5`)
	tooLateToPowerOn := regexp.MustCompile(`power state \d = G3`)
	powerOnFinished := regexp.MustCompile(`power state \d = S0`)
	powerButtonPressFinished := regexp.MustCompile(`PB task \d = idle`)
	didPowerOn := false
	hitS5 := false
	donePowerOff := false
	closeUART, err := pxy.Servo().EnableUARTCapture(ctx, servo.ECUARTCapture)
	if err != nil {
		return errors.Wrap(err, "failed to enable capture EC UART")
	}
	defer func() { retErr = errors.Join(retErr, closeUART(ctx)) }()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		lines, err := pxy.Servo().GetQuotedString(ctx, servo.ECUARTStream)
		if err != nil {
			return errors.Wrap(err, "failed to read UART")
		}
		if lines == "" {
			return errors.New("Not in S0 yet")
		}
		// It is possible to read partial lines, so save the part after newline for later
		lines = leftoverLines + lines
		if crlfIdx := strings.LastIndex(lines, "\r\n"); crlfIdx < 0 {
			leftoverLines = lines
			lines = ""
		} else {
			leftoverLines = lines[crlfIdx+2:]
			lines = lines[:crlfIdx+2]
		}

		l := lines
		if readyForPowerOn.MatchString(l) && !didPowerOn {
			testing.ContextLogf(ctx, "Found S5: %q", l)
			hitS5 = true
		}

		if powerButtonPressFinished.MatchString(l) && !didPowerOn {
			testing.ContextLogf(ctx, "Found power button release: %q", l)
			donePowerOff = true
		}
		// If the long press above is done, and we've seen S5, then do a short press to power on.
		if hitS5 && donePowerOff && !didPowerOn {
			testing.ContextLog(ctx, "Pressing power button")
			if err := pxy.Servo().SetString(ctx, servo.ECUARTCmd, "powerbtn 200"); err != nil {
				return testing.PollBreak(err)
			}
			didPowerOn = true
		}
		if tooLateToPowerOn.MatchString(l) && !didPowerOn {
			testing.ContextLogf(ctx, "Found G3: %q", l)
			return errors.New("power state reached G3, power button pressed too late")
		}
		if powerOnFinished.MatchString(l) && didPowerOn {
			testing.ContextLogf(ctx, "Found S0: %q", l)
			return nil
		}
		if !hitS5 || !donePowerOff {
			return errors.New("failed to hit s5 state")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Millisecond * 200, Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "EC output parsing failed")
	}
	return nil
}

// openVT2 opens the VT2 Terminal
func openVT2(ctx context.Context, kb inputs.KeyboardServiceClient) error {
	keyboardKey := "ctrl+alt+refresh"
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: keyboardKey,
	}); err != nil {
		return errors.Wrapf(err, "failed to press key %q", keyboardKey)
	}
	waitTime := 5 * time.Second // wait time for switching to happen.
	// GoBigSleepLint: Allowing some wait time for switching to happen.
	// TODO(b:198837833): Replace with testing.Poll to query the current vts node.
	if err := testing.Sleep(ctx, waitTime); err != nil {
		return errors.Wrap(err, "failed while waiting for switching to VT2")
	}
	return nil
}

// loginVT2 logins in VT2 Terminal using default username and password
func loginVT2(ctx context.Context, kb inputs.KeyboardServiceClient) error {
	if _, err := kb.Type(ctx, &inputs.TypeRequest{
		Key: "root",
	}); err != nil {
		return errors.Wrap(err, "failed to enter username")
	}
	enterKey := "Enter"
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: enterKey,
	}); err != nil {
		return errors.Wrap(err, "failed to press enter key")
	}

	if _, err := kb.Type(ctx, &inputs.TypeRequest{
		Key: "test0000",
	}); err != nil {
		return errors.Wrap(err, "failed to enter password")
	}
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: enterKey,
	}); err != nil {
		return errors.Wrap(err, "failed to press enter key")
	}
	return nil
}

// rebootViaVT2 enters reboot command in VT2 terminal
func rebootViaVT2(ctx context.Context, kb inputs.KeyboardServiceClient) error {
	if _, err := kb.Type(ctx, &inputs.TypeRequest{
		Key: "reboot",
	}); err != nil {
		return errors.Wrap(err, "failed to enter reboot command")
	}

	// Error expected after pressing enter because DUT is turning off.
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: "Enter",
	}); err != nil {
		testing.ContextLog(ctx, "Error expected: ", err)
	}
	return nil
}
