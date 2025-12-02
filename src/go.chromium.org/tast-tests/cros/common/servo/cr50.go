// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// These are the GSC Servo controls which can be get/set with a string value.
const (
	GSCCCDLevel     StringControl = "gsc_ccd_level"
	GSCECReset      StringControl = "gsc_ec_reset"
	GSCECResetPulse StringControl = "gsc_ecrst_pulse"
	GSCTestlab      StringControl = "gsc_testlab"
	GSCResetCount   StringControl = "gsc_reset_count"
	GSCUARTCmd      StringControl = "gsc_uart_cmd"
	GSCUARTRegexp   StringControl = "gsc_uart_regexp"
	GSCUARTStream   StringControl = "gsc_uart_stream"
	GSCVersion      StringControl = "gsc_version"
	// Add the ccd_gsc prefix to the watchdog connected control, so it works
	// on single and dual_v4 setups.
	WatchdogCCDConnected    StringControl = "ccd_gsc.watchdog_ccd_connected"
	WatchdogCCDConnectedYes string        = "yes"
)

// Regexes for GSC console commands
var (
	// Regex to check if there's a pending reboot from a WP event
	ti50APROVerifyRebootPendingRE = regexp.MustCompile(`Reboot pending at next AP power event: (Yes|No)`)
)

// These controls accept only "on" and "off" as values.
const (
	GSCUARTCapture OnOffControl = "gsc_uart_capture"
)

// CCD levels
const (
	Open   string = "open"
	Lock   string = "lock"
	Unlock string = "unlock"
)

// TestlabState contains possible ccd testlab states.
type TestlabState string

// Possible testlab states.
const (
	Enable  TestlabState = "on"
	Disable TestlabState = "off"
)

// CCDCap contains possible CCD capabilities.
type CCDCap string

// CCD capabilities.
const (
	UartGscRxAPTx     CCDCap = "UartGscRxAPTx"
	UartGscTxAPRx     CCDCap = "UartGscTxAPRx"
	UartGscRxECTx     CCDCap = "UartGscRxECTx"
	UartGscTxECRx     CCDCap = "UartGscTxECRx"
	FlashAP           CCDCap = "FlashAP"
	FlashEC           CCDCap = "FlashEC"
	OverrideWP        CCDCap = "OverrideWP"
	RebootECAP        CCDCap = "RebootECAP"
	GscFullConsole    CCDCap = "GscFullConsole"
	UnlockNoReboot    CCDCap = "UnlockNoReboot"
	UnlockNoShortPP   CCDCap = "UnlockNoShortPP"
	OpenNoTPMWipe     CCDCap = "OpenNoTPMWipe"
	OpenNoLongPP      CCDCap = "OpenNoLongPP"
	BatteryBypassPP   CCDCap = "BatteryBypassPP"
	Unused            CCDCap = "Unused"
	I2C               CCDCap = "I2C"
	FlashRead         CCDCap = "FlashRead"
	OpenNoDevMode     CCDCap = "OpenNoDevMode"
	OpenFromUSB       CCDCap = "OpenFromUSB"
	OverrideBatt      CCDCap = "OverrideBatt"
	APROCheckVC       CCDCap = "APROCheckVC"
	AllowUnverifiedRo CCDCap = "AllowUnverifiedRo"
)

// CCDCapState contains possible states for a CCD capability.
type CCDCapState string

// CCD capability states
const (
	CapDefault      CCDCapState = "Default"
	CapAlways       CCDCapState = "Always"
	CapUnlessLocked CCDCapState = "UnlessLocked"
	CapIfOpened     CCDCapState = "IfOpened"
)

// RunGSCCommand runs the given command on the GSC on the device.
func (s *Servo) RunGSCCommand(ctx context.Context, cmd string) error {
	if err := s.SetString(ctx, GSCUARTRegexp, "None"); err != nil {
		return errors.Wrap(err, "Clearing GSC UART Regexp")
	}
	return s.SetString(ctx, GSCUARTCmd, cmd)
}

// RunGSCCommandGetOutput runs the given command on the GSC on the device and returns the output matching patterns.
func (s *Servo) RunGSCCommandGetOutput(ctx context.Context, cmd string, patterns []string) ([][]string, error) {
	err := s.SetStringList(ctx, GSCUARTRegexp, patterns)
	if err != nil {
		return nil, errors.Wrapf(err, "setting GSCUARTRegexp to %s", patterns)
	}
	defer s.SetString(ctx, GSCUARTRegexp, "None")
	err = s.SetString(ctx, GSCUARTCmd, cmd)
	if err != nil {
		return nil, errors.Wrapf(err, "setting GSCUARTCmd to %s", cmd)
	}
	iList, err := s.GetStringList(ctx, GSCUARTCmd)
	if err != nil {
		return nil, errors.Wrap(err, "decoding string list")
	}
	return ConvertToStringArrayArray(ctx, iList)
}

// CheckGSCBootMode verifies that the boot mode as reported by GSC's ec_comm command is as expected.
func (s *Servo) CheckGSCBootMode(ctx context.Context, expectedModes []string) error {
	output, err := s.RunGSCCommandGetOutput(ctx, "ec_comm", []string{`boot_mode\s*:\s*(\S+)\s`})
	if err != nil {
		return errors.Wrap(err, "failed to get boot mode")
	}
	for _, expected := range expectedModes {
		if output[0][1] == expected {
			return nil
		}
	}
	return errors.Wrapf(err, "incorrect boot mode, got %q want one of %q", output[0][1], expectedModes)
}

// SetTestlab will perform the required power button presses to disable or enable CCD testlab mode.
func (s *Servo) SetTestlab(ctx context.Context, option TestlabState) error {
	// Verify CCD is open.
	regExpCcdOpen := `State:\s*Opened`
	if _, err := s.RunGSCCommandGetOutput(ctx, "ccd", []string{regExpCcdOpen}); err != nil {
		return errors.Wrap(err, "ccd is not open")
	}

	// Verify there is a servo micro or C2D2 connected.
	hasMicroOrC2D2, err := s.PreferDebugHeader(ctx)
	if err != nil {
		return errors.Wrap(err, "verifying the preferred debug header")
	}
	if !hasMicroOrC2D2 {
		return errors.New("no micro-servo or C2D2 found: manual procedure is required to modify testlab state")
	}

	testing.ContextLogf(ctx, "Setting testlab to %q", option)
	if err := s.RunGSCCommand(ctx, "ccd testlab "+string(option)); err != nil {
		return errors.Wrapf(err, "failed setting testlab to %q", option)
	}

	// GoBigSleepLint: Waiting 1 second before starting the power pressing sequence.
	if err := testing.Sleep(ctx, 1*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait 1 second before the power pressing sequence")
	}

	// Press the power button for up to 20 seconds, and space a 1 second
	// interval between these presses. The GSC console doesn't care about
	// extra presses in between. As long as all the required presses are hit,
	// testlab state would change.
	testing.ContextLog(ctx, "Starting power button presses")
	ppTimeout := 20 * time.Second
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		testlab, err := s.GetString(ctx, GSCTestlab)
		if err != nil {
			return errors.Wrap(err, "failed to get gsc_testlab")
		}
		if testlab == string(option) {
			return nil
		}

		if err := s.KeypressWithDuration(ctx, PowerKey, DurPress); err != nil {
			return errors.Wrap(err, "failed to press power button")
		}
		return errors.Errorf("testlab has not been set to %q", option)
	}, &testing.PollOptions{Timeout: ppTimeout, Interval: 1 * time.Second}); err != nil {
		return err
	}

	return nil
}

/*
GetCCDCapability will return the current state of a specific CCD capability.
Possible states are:

	0 = Default
	1 = Always
	2 = UnlessLocked
	3 = IfOpened

It will also return "Y" if the capability is accessible, and "-" otherwise.
*/
func (s *Servo) GetCCDCapability(ctx context.Context, capability CCDCap) (int, string, error) {
	re := `(` + string(capability) + `)\s*(\w|\W)\s*\d`
	var out [][]string
	var err error
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err = s.RunGSCCommandGetOutput(ctx, "ccd", []string{re})
		if err != nil {
			return errors.Wrap(err, "failed to get capability state")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 2 * time.Second}); err != nil {
		return 0, "", err
	}
	splitout := strings.Split(out[0][0], " ")
	state, err := strconv.Atoi(splitout[len(splitout)-1])
	if err != nil {
		return 0, "", errors.Wrapf(err, "unable to tell %s state", capability)
	}
	return state, splitout[len(splitout)-2], nil
}

/*
SetCCDCapability will try to set a CCD capability to a specific state.
Possible states are:

	Default
	Always
	UnlessLocked
	IfOpened
*/
func (s *Servo) SetCCDCapability(ctx context.Context, capabilities map[CCDCap]CCDCapState) error {
	// Information about CCD states is usually returned in the form of
	// '[CapabilityName|Y|1]'. Create a map to match each capability
	// state with the corresponding integer value.
	capMap := map[CCDCapState]int{
		CapDefault:      0,
		CapAlways:       1,
		CapUnlessLocked: 2,
		CapIfOpened:     3,
	}

	for capability, state := range capabilities {
		// Ensure the state we want is set.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			cmd := fmt.Sprintf("ccd set %s %s", capability, state)
			if err := s.RunGSCCommand(ctx, cmd); err != nil {
				return errors.Wrapf(err, "failed to send command %q to gsc", cmd)
			}

			currState, _, err := s.GetCCDCapability(ctx, capability)
			if err != nil {
				return errors.Wrapf(err, "failed to get current %q state", capability)
			}

			if currState != capMap[state] {
				return errors.Errorf("got state %v, but expected %v", currState, capMap[state])
			}
			return nil
		}, &testing.PollOptions{Timeout: 1 * time.Minute, Interval: 10 * time.Second}); err != nil {
			return errors.Wrapf(err, "failed to set %s to %s", capability, state)
		}
	}
	return nil
}

// CheckGSCCommandOutput is a wrapper function to that runs a GSC command and
// ensures all Python regex patterns appear at least once in the response. This
// function will throw a pretty printed error otherwise.
func (s *Servo) CheckGSCCommandOutput(ctx context.Context, cmd string, regexs []string) error {
	matches, err := s.RunGSCCommandGetOutput(ctx, cmd, regexs)
	if err != nil {
		return errors.Wrap(err, "failed to run `"+cmd+"` on GSC, expected regex patterns = {"+strings.Join(regexs, ",")+"}")
	}
	if len(matches) == 0 {
		// NOTE: I've never seen this case occur since `servod` will throw an
		// XML error if no matches are found
		return errors.New("Failed to get regex matches = {" + strings.Join(regexs, ",") + "} for `" + cmd + "`")
	}
	return nil
}

// LockCCD locks the CCD console by sending a GSC command.
func (s *Servo) LockCCD(ctx context.Context) error {
	// Cr50 and Ti50 output slightly different capitalization, so use case
	// insensitive matching
	if err := s.CheckGSCCommandOutput(ctx, "ccd lock", []string{`(?i)CCD locked`}); err != nil {
		return errors.Wrap(err, "failed to run 'ccd lock' on GSC")
	}
	return nil
}

/*
GetAPState runs the 'ccdstate' command in the gsc console,
and returns information about AP's status as follows:
1. "off", which means that the ap is off.
2. "on (K)", which means that the dut has jumped into kernel.
3. "on (F)", which means that the dut is on firmware.
*/
func (s *Servo) GetAPState(ctx context.Context) (string, string, error) {
	cmd := "ccdstate"
	regex := []string{`AP:\s*(\w*)\s*\((\w*|)\)`}
	matches, err := s.RunGSCCommandGetOutput(ctx, cmd, regex)
	if err != nil {
		return "", "", errors.Wrapf(err, "while running %s", cmd)
	}
	var apPower, screenState = "unknown", "unknown"
	if len(matches[0]) != 3 {
		return apPower, screenState, errors.Errorf("found unexpected number of matches: %v", matches)
	}
	apPower = matches[0][1]
	screenState = matches[0][2]

	return apPower, screenState, nil
}

/*
GetCCDMode runs the 'ccdstate' command in the gsc console, and returns information
about CCD_MODE's status as either "asserted" or "deasserted".
*/
func (s *Servo) GetCCDMode(ctx context.Context) (string, error) {
	cmd := "ccdstate"
	regex := []string{`CCD_MODE:\s*([a-z]+)\s`}
	matches, err := s.RunGSCCommandGetOutput(ctx, cmd, regex)
	if err != nil {
		return "", errors.Wrapf(err, "while running %s", cmd)
	}

	if len(matches[0]) != 2 {
		return "", errors.Errorf("found unexpected number of matches: %v", matches)
	}

	return matches[0][1], nil
}

// GetGscUSBSerialNumberDescriptor uses the GSC `sysinfo` command to get the
// `DEV_ID`s for the device and format them as a USB serial number descriptor
// that can be used in the `flashrom` command. This is useful when many GSCs are
// attached to a host and we want to target our device specifically.
func (s *Servo) GetGscUSBSerialNumberDescriptor(ctx context.Context) (string, error) {
	regex := `DEV_ID:\s+0x([0-9a-fA-F]+)\s+0x([0-9a-fA-F]+)`
	matches, err := s.RunGSCCommandGetOutput(ctx, "sysinfo", []string{regex})
	if err != nil {
		return "", errors.Wrap(err, "failed to match GSC `sysinfo` command output")
	}

	// TODO(b/266096476): Support Ti50 that uses lowercase instead of uppercase
	// like Cr50.
	if len(matches[0]) == 3 {
		s := strings.ToUpper(matches[0][1]) + "-" + strings.ToUpper(matches[0][2])
		return s, nil
	}

	return "", errors.New("failed to get the correct regex match from `sysinfo` command output")
}

// GSCVersionInfoStruct stores the GSC version information
type GSCVersionInfoStruct struct {
	// Version is the Epoch.Major.Minor version string
	Version string
	// Minor is the running minor version
	Minor int
	// Major is the running major version
	Major int
	// Epoch is the running epoch version
	Epoch int
	// IsTi50 is true if the device is running Ti50 firmware
	IsTi50 bool
	// IsCr50 is true if the device is running Cr50 firmware
	IsCr50 bool
	// Str is the version string containing the fw name and git sha
	Str string
}

// GSCVersionInfo returns the GSC version
func (s *Servo) GSCVersionInfo(ctx context.Context) (GSCVersionInfoStruct, error) {
	version := GSCVersionInfoStruct{}
	output, err := s.RunGSCCommandGetOutput(ctx, "version",
		[]string{`RW_[AB]: *\* *(\d+).(\d+).(\d+)/(\S+)[\r\n]`})
	if err != nil {
		return version, errors.Wrap(err, "failed to get version")
	}
	epoch, err := strconv.Atoi(output[0][1])
	if err != nil {
		return version, errors.Wrapf(err, "invalid epoch ver %s", output[0][1])
	}

	major, err := strconv.Atoi(output[0][2])
	if err != nil {
		return version, errors.Wrapf(err, "invalid major ver %s", output[0][2])
	}

	minor, err := strconv.Atoi(output[0][3])
	if err != nil {
		return version, errors.Wrapf(err, "invalid minor ver %s", output[0][3])
	}

	version.Minor = minor
	version.Major = major
	version.Epoch = epoch
	version.Str = output[0][4]
	version.Version = fmt.Sprintf("%d.%d.%d", version.Epoch, version.Major, version.Minor)
	version.IsTi50 = strings.Contains(version.Str, "ti50")
	version.IsCr50 = strings.Contains(version.Str, "cr50")
	if version.IsTi50 == version.IsCr50 {
		return GSCVersionInfoStruct{}, errors.Errorf("could not determine cr50 vs ti50: cr50 %t ti50 %t",
			version.IsCr50, version.IsTi50)
	}

	return version, nil
}

// GSCFeature stores the information about when a GSC feature was added
type GSCFeature struct {
	// Desc is a short string describing the feature
	Desc string
	// Epoch is the epoch of the version the feature was added in
	Epoch int
	// Major is the major of the version the feature was added in. Use the
	// prod major version.
	Major int
	// Minor is the minor of the version the feature was added in.
	Minor int
	// IsCr50 is true if the device is using cr50 firmware
	IsCr50 bool
	// IsTi50 is true if the device is using ti50 firmware
	IsTi50 bool
}

// GSCAPROVerificationWPReboot contains the information for when Ti50 starts
// rebooting to run verification. It'll run verification after the AP resets if
// it detects one of these WP events.
// - after WP goes from disabled to enabled or if
// - it detects WP is disabled externally when it's enabling it internally.
var GSCAPROVerificationWPReboot GSCFeature = GSCFeature{
	Desc:   "reboot after WP enable",
	Epoch:  0,
	Major:  23,
	Minor:  172,
	IsTi50: true,
	IsCr50: false,
}

// gscFeatures is used to cache GSC feature states
var gscFeatures = make(map[string]bool)

// standardizeMajorVersion converts the major version to a standard value
func standardizeMajorVersion(major int) int {
	// PrePVT and MP minor versions are equivalent. Convert a PrePVT major
	// value to the equivalent MP value.
	if major%2 == 0 {
		major = major - 1
	}
	// GSC chips are released 10 versions apart.ex 33 on NT is the same as 23 on DT.
	// Use major version mod 10 to standardize the major version across chips.
	return major % 10
}

// HasGSC returns true if servod has GSC controls
func (s *Servo) HasGSC(ctx context.Context) bool {
	hasControl, err := s.HasControl(ctx, string(GSCVersion))
	if err != nil {
		return false
	}
	return hasControl
}

// GSCHasFeature checks if the running GSC image has the given GSC feature
func (s *Servo) GSCHasFeature(ctx context.Context, feature GSCFeature) (bool, error) {
	value, ok := gscFeatures[feature.Desc]
	if ok {
		return value, nil
	}

	// Return false if the setup doesn't have GSC.
	if !s.HasGSC(ctx) {
		testing.ContextLog(ctx, "GSCHasFeature: no GSC")
		gscFeatures[feature.Desc] = false
		return false, nil
	}

	version, err := s.GSCVersionInfo(ctx)
	if err != nil {
		return false, err
	}
	hasFeature := false
	featureMajor := standardizeMajorVersion(feature.Major)
	testing.ContextLogf(ctx, "converted feature major version %d to %d", feature.Major, featureMajor)
	major := standardizeMajorVersion(version.Major)
	testing.ContextLogf(ctx, "converted running major version %d to %d", version.Major, major)
	versionOk := major > featureMajor || (major == featureMajor && version.Minor >= feature.Minor)

	if feature.IsTi50 && version.IsTi50 {
		hasFeature = versionOk
	} else if feature.IsCr50 && version.IsCr50 {
		hasFeature = versionOk
	}
	gscFeatures[feature.Desc] = hasFeature
	testing.ContextLogf(ctx, "GSC feature: %s: %t", feature.Desc, hasFeature)
	return hasFeature, nil
}

// canCheckCCDConnected returns True if the device has the watchdog_ccd_connected control
func (s *Servo) canCheckCCDConnected(ctx context.Context) bool {
	hasControl, err := s.HasControl(ctx, string(WatchdogCCDConnected))
	if err != nil {
		testing.ContextLogf(ctx, "Unable to check for %s. Returning false", WatchdogCCDConnected)
		return false
	}
	return hasControl
}

// CCDConnected returns true if CCD is connected.
func (s *Servo) CCDConnected(ctx context.Context) (bool, error) {
	connected, err := s.GetString(ctx, WatchdogCCDConnected)
	if err != nil {
		return false, err
	}
	return connected == WatchdogCCDConnectedYes, nil
}

// WaitForCCDState wait until CCD is connected or disconnected.
func (s *Servo) WaitForCCDState(ctx context.Context, connected bool, timeout time.Duration) error {
	var expectedState string
	if connected {
		expectedState = "connect"
	} else {
		expectedState = "disconnect"
	}
	testing.ContextLogf(ctx, "Wait %.2fs for CCD %s", timeout.Seconds(), expectedState)

	pOpts := testing.PollOptions{Interval: time.Second, Timeout: timeout}
	err := testing.Poll(ctx, func(ctx context.Context) error {
		isConnected, err := s.CCDConnected(ctx)
		if err != nil {
			return err
		}
		testing.ContextLogf(ctx, "CCD connected: %t", isConnected)
		if isConnected == connected {
			return nil
		}
		return errors.Errorf("CCD did not %s", expectedState)
	}, &pOpts)
	if err != nil {
		return err
	}
	return nil
}

// WaitForCCDConnect wait until CCD connected.
func (s *Servo) WaitForCCDConnect(ctx context.Context, timeout time.Duration) error {
	return s.WaitForCCDState(ctx, true, timeout)
}

// WaitForCCDDisconnect wait until CCD disconnected.
func (s *Servo) WaitForCCDDisconnect(ctx context.Context, timeout time.Duration) error {
	return s.WaitForCCDState(ctx, false, timeout)
}

// WaitForCCDDisconnectAndReconnect wait for CCD to disconnect and reconnect.
func (s *Servo) WaitForCCDDisconnectAndReconnect(ctx context.Context, timeout time.Duration) error {
	start := time.Now()
	if err := s.WaitForCCDDisconnect(ctx, timeout); err != nil {
		return err
	}
	now := time.Now()
	elapsedTime := now.Sub(start)
	secondTimeout := 3 * time.Second
	if elapsedTime+secondTimeout > timeout {
		testing.ContextLog(ctx, "took a long time to detect CCD disconnect")
		testing.ContextLog(ctx, "Increasing the overall timeout")
	} else {
		secondTimeout = timeout - elapsedTime
	}
	if err := s.WaitForCCDConnect(ctx, secondTimeout); err != nil {
		return err
	}
	return nil
}

// gscIsResponsive returns true if the GSC console is responsive
func (s *Servo) gscIsResponsive(ctx context.Context) bool {
	if !s.HasGSC(ctx) {
		return false
	}
	testing.ContextLog(ctx, "Sending an GSC command to check if GSC is responsive")
	version, err := s.GetString(ctx, GSCVersion)
	if err != nil {
		return false
	}
	// All valid GSC version strings contain 50
	if strings.Contains(version, "50") {
		testing.ContextLog(ctx, "GSC is active ")
		return true
	}
	testing.ContextLog(ctx, "GSC is not active")
	return false
}

// WaitForGSCStartup wait for the GSC startup message
func (s *Servo) WaitForGSCStartup(ctx context.Context, timeout time.Duration) error {
	pOpts := testing.PollOptions{Interval: time.Second, Timeout: timeout}
	err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := s.RunGSCCommandGetOutput(ctx, "\n\n", []string{`.*(ti50_common|Console is enabled)`})
		if err != nil {
			// If servod didn't detect the startup message, check the reset
			// count to see if GSC reset.
			resetCount, resetCountErr := s.GetString(ctx, GSCResetCount)
			if resetCountErr == nil && resetCount == "1" {
				testing.ContextLog(ctx, "GSC reset")
				return nil
			}
			return err
		}
		testing.ContextLog(ctx, "detected GSC console startup message")
		return nil
	}, &pOpts)
	if err != nil {
		return err
	}
	return nil
}

// WaitForGSCReset waits until the GSC resets. Wait for CCD to come back up if applicable.
func (s *Servo) WaitForGSCReset(ctx context.Context, timeout time.Duration) error {
	s.ClearExpectWPEventReboot()
	testing.ContextLogf(ctx, "Wait %.2fs for GSC Reset", timeout.Seconds())
	if s.canCheckCCDConnected(ctx) {
		return s.WaitForCCDDisconnectAndReconnect(ctx, timeout)
	}

	// It's possible GSC reset before this was called. That's fine. Log the error
	if err := s.WaitForGSCStartup(ctx, timeout); err != nil {
		return errors.Errorf("did not detect reset on the GSC console: %s", err)
	}
	if !s.gscIsResponsive(ctx) {
		return errors.New("GSC is unresponsive after reset")
	}
	return nil
}

// ExpectTi50WPEventReboot returns True if Ti50 is going to reset the next time
// the AP reboots
func (s *Servo) ExpectTi50WPEventReboot(ctx context.Context) bool {
	return s.ti50WPEventPendingReboot
}

// ClearExpectWPEventReboot resets the expectedTi50APROReboot value to false
func (s *Servo) ClearExpectWPEventReboot() {
	s.ti50WPEventPendingReboot = false
}

// Ti50CheckPendingWPEvent after a WP change, check if there's a penging WP event
// that will cause GSC to reset the next time the AP resets.
func (s *Servo) Ti50CheckPendingWPEvent(ctx context.Context, value FWWPStateValue) error {
	output, err := s.RunGSCCommandGetOutput(ctx, "ap_ro_verify", []string{`.*>`})
	if err != nil {
		return errors.Wrap(err, "failed to get ap_ro_verify output")
	}

	match := ti50APROVerifyRebootPendingRE.FindStringSubmatch(output[0][0])
	if len(match) != 0 {
		s.ti50WPEventPendingReboot = match[1] == "Yes"
		return nil
	}
	if hasFeature, err := s.GSCHasFeature(ctx, GSCAPROVerificationWPReboot); err != nil {
		return errors.Wrap(err, "failed to check for feature")
	} else if !hasFeature {
		s.ti50WPEventPendingReboot = false
		return nil
	}
	// If Ti50 doesn't print pending WP events in the ap_ro_verify output,
	// manually track expected reboots
	if s.ti50LastFWWPState == "" || !s.ti50WPEventPendingReboot {
		if s.ti50LastFWWPState != value {
			s.ti50WPEventPendingReboot = value == FWWPStateForceOn
			s.ti50LastFWWPState = value
		}
	}
	return nil
}
