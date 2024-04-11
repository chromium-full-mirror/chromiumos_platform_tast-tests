// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CCDLevel contains possible CCD levels.
type CCDLevel string

// Possible CCD levels.
const (
	Open   CCDLevel = "open"
	Lock   CCDLevel = "lock"
	Unlock CCDLevel = "unlock"
)

// Console Regular expressions.
var (
	// Regex to check if access denied shows up in the command output
	accessDeniedRE = regexp.MustCompile(`(?i)access denied`)
	// Regex to extract CCD states and resolve `Default` states to their true states.
	capDefaultRE = regexp.MustCompile(`(?:\s\s([A-Za-z1-9]+)\s+[Y\-]\s0=Default\s\(([A-Za-z]+)\)|\s\s([A-Za-z1-9]+)\s+[Y\-]\s[0-3]=([A-Za-z]+))`)
	// Regex to extract CCD State flags
	consoleCCDStateRE = regexp.MustCompile("State: ([A-Za-z]+)")
	// Regex to extract commands from help output
	knownCommandRE = regexp.MustCompile(`(?s)Known commands:\s*(.*)HELP LIST`)
	// Regex to wait for a power button prompt
	pwrbPromptRE = regexp.MustCompile("Press the physical button now")
	// Regex to wait until ccd testlab mode is enabled
	testlabDisabledRE = regexp.MustCompile("Updating testlab to false|CCD test lab mode disabled")
	// Regex to wait until ccd testlab mode is disabled
	testlabEnabledRE = regexp.MustCompile("Updating testlab to true|CCD test lab mode enabled")
	// GSC version strings
	verRWCr50StrRE = `cr50_([0-9_vpmefi\.]*)\.[0-9]*-([[:xdigit:]]+)`
	verRWTi50StrRE = `ti50_common_([a-z]+)\S*:(\S+)`
	verRWGSCStrRE  = verRWCr50StrRE + `|` + verRWTi50StrRE
	// GSC board properties
	brdPropRE          = regexp.MustCompile(`properties = 0x([0-9a-fA-F]+)`)
	gettimeDeepSleepRE = regexp.MustCompile(`deep sleep:.*= ([0-9\.]*) `)
	gettimeColdResetRE = regexp.MustCompile(`reset:.*= ([0-9\.]*) `)
	// USB ADC info regex
	usbAdcStateRE = regexp.MustCompile(`(PHY [AB])|ADC: (disconnected)|connected: ([\S]+)`)
	usbAdcCc1RE   = regexp.MustCompile(`ADC: CC1 = ([0-9]+) mV`)
	usbAdcCc2RE   = regexp.MustCompile(`ADC: CC2 = ([0-9]+) mV`)
	// RMA regex
	rmaAuthChallengeRE = regexp.MustCompile(`([A-Z0-9]{80})|(RMA Auth error)|(Must wait)`)
)

// TestlabState contains possible CCD testlab states.
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
	UartGscRxFpmcuTx  CCDCap = "UartGscRxFpmcuTx"
	UartGscTxFpmcuRx  CCDCap = "UartGscTxFpmcuRx"
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
	UpdateNoTPMWipe   CCDCap = "UpdateNoTPMWipe" // Cr50 only
	Unused            CCDCap = "Unused"
	I2C               CCDCap = "I2C"
	FlashRead         CCDCap = "FlashRead"
	OpenNoDevMode     CCDCap = "OpenNoDevMode"
	OpenFromUSB       CCDCap = "OpenFromUSB"
	OverrideBatt      CCDCap = "OverrideBatt"
	APROCheckVC       CCDCap = "APROCheckVC"
	AllowUnverifiedRO CCDCap = "AllowUnverifiedRo"
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

// GscBranch contains possible GSC firmware branch origins.
type GscBranch uint

// Possible GSC branches
const (
	ToT GscBranch = iota
	PrePvt
	MP
	EFI
	Unknown
)

// GscSlot contains possible GSC firmware slots.
type GscSlot uint

// Possible GSC firmware slots
const (
	SlotA GscSlot = iota
	SlotB
)

// VersionCommandInfo contains structured information returned from the GSC
// `version` command.
type VersionCommandInfo struct {
	RoA   RoInfo
	RoB   RoInfo
	RwA   RwInfo
	RwB   RwInfo
	Bid   BidInfo
	Build BuildInfo
}

// RoInfo contains information about a loaded ro image slot.
type RoInfo struct {
	Active     bool
	Version    string
	ImageCheck string
}

// RwInfo contains information about a loaded rw image slot.
type RwInfo struct {
	Empty   bool
	Active  bool
	Debug   bool
	Version string
	Branch  GscBranch
}

// BidInfo contains board id information for a slot.
type BidInfo struct {
	Empty   bool
	BidType uint32
	Mask    uint32
	Flags   uint32
}

// BuildInfo contains information about the firmware currently running.
type BuildInfo struct {
	Branch GscBranch
	Debug  bool
}

// CrOSImage interacts with a board running ti50.
type CrOSImage struct {
	*CommandImage
}

// Fatal facilitates failing a test or test fixture.
type Fatal interface {
	Fatalf(format string, args ...interface{})
}

// OpenCrOSImage creates a new CrOSImage. This must be closed to free connection.
func OpenCrOSImage(ctx context.Context, console SerialChannel) (*CrOSImage, error) {
	i, err := OpenCommandImage(ctx, console, "\n", "^(\\[[ 0-9.]+.\\] )?> ")
	if err != nil {
		return nil, err
	}
	// Allow for timestamp to be present before prompt "[ 999999.999 C] > " or just "> "
	return &CrOSImage{CommandImage: i}, nil
}

// MustOpenCrOSImage is shorthand for OpenCrOSImage that will Fatalf the test/fixture if failed.
func MustOpenCrOSImage(ctx context.Context, console SerialChannel, failWith Fatal) *CrOSImage {
	i, err := OpenCrOSImage(ctx, console)
	if err != nil {
		failWith.Fatalf("New CrOS Image: %v", err)
	}
	return i
}

// WaitUntilBooted waits until the image is fully booted.
func (i *CrOSImage) WaitUntilBooted(ctx context.Context) error {
	return i.CommandImage.WaitUntilBooted(ctx, 2*time.Second)
}

// CrOSImageHelpOutput holds relevant data from the 'help' command.
type CrOSImageHelpOutput struct {
	Raw      string
	Commands []string
}

// Help issues 'help' and parses its output.
func (i *CrOSImage) Help(ctx context.Context) (CrOSImageHelpOutput, error) {
	out, err := i.Command(ctx, "help")
	if err != nil {
		return CrOSImageHelpOutput{}, errors.Wrap(err, "failed to execute help command")
	}
	m := knownCommandRE.FindStringSubmatch(out)
	if m == nil {
		return CrOSImageHelpOutput{}, errors.New("failed to parse help output")
	}
	cmds := make([]string, 0)
	for _, c := range strings.Split(m[1], " ") {
		c = strings.TrimSpace(c)
		if len(c) > 0 {
			cmds = append(cmds, c)
		}
	}
	return CrOSImageHelpOutput{out, cmds}, nil
}

// CCDLock uses the `ccd` GSC console command to set the CCD level to locked.
func (i *CrOSImage) CCDLock(ctx context.Context) error {
	if _, err := i.Command(ctx, "ccd lock"); err != nil {
		return errors.Wrap(err, "failed to execute ccd lock")
	}
	// TODO(b/307544573): Check output string for Ti50 + Cr50

	return nil
}

// CCDOpen uses the `ccd` GSC console command to set the CCD level to open.
func (i *CrOSImage) CCDOpen(ctx context.Context) error {
	if _, err := i.Command(ctx, "ccd open"); err != nil {
		return errors.Wrap(err, "failed to execute ccd open")
	}
	// TODO(b/307544573): Check output string for Ti50 + Cr50

	return nil
}

// CCDReset uses the `ccd` GSC console command set the CCD states to defaults.
func (i *CrOSImage) CCDReset(ctx context.Context) error {
	return i.runCommand(ctx, "ccd reset")
}

// CCDResetFactory uses the `ccd` GSC console command set the CCD states to factory.
func (i *CrOSImage) CCDResetFactory(ctx context.Context) error {
	return i.runCommand(ctx, "ccd reset factory")
}

// GetCCDCapabilities uses the `ccd` GSC console command to return a map of all
// CCD capability states. Capabilities that are in their default states will be
// reported as their true states.
func (i *CrOSImage) GetCCDCapabilities(ctx context.Context) (map[CCDCap]CCDCapState, error) {
	output, err := i.safeCommand(ctx, "ccd")
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute ccd open")
	}

	return matchCCDCapabilities(output)
}

func matchCCDCapabilities(s string) (map[CCDCap]CCDCapState, error) {
	var out map[CCDCap]CCDCapState

	matches := capDefaultRE.FindAllStringSubmatch(s, -1)
	if matches == nil {
		return nil, errors.New("failed to parse ccd output")
	}

	// Map regex result to typed result
	out = make(map[CCDCap]CCDCapState)
	for i := 0; i < len(matches); i++ {
		var cap CCDCap
		var state CCDCapState
		if matches[i][1] != "" {
			cap = CCDCap(matches[i][1])
			state = CCDCapState(matches[i][2])
		} else {
			cap = CCDCap(matches[i][3])
			state = CCDCapState(matches[i][4])
		}
		out[cap] = state
	}

	return out, nil
}

// GetCCDCapability uses the `ccd` GSC console command to return the state of
// the requested CCD capability.
func (i *CrOSImage) GetCCDCapability(ctx context.Context, capability CCDCap) (CCDCapState, error) {
	states, err := i.GetCCDCapabilities(ctx)
	if err != nil {
		return CapDefault, err
	}
	return states[capability], nil
}

// SetCCDCapability uses the `ccd` GSC console command to set a specific
// capability to a given state.
func (i *CrOSImage) SetCCDCapability(ctx context.Context, capability CCDCap, state CCDCapState) error {
	if _, err := i.Command(ctx, "ccd set "+string(capability)+" "+string(state)); err != nil {
		return errors.Wrap(err, "failed to set CCD cap "+string(capability)+" to "+string(state))
	}
	return nil
}

// SendConsoleRebootCmd issues the reboot command but does not listen for a response since the GSC
// is expected to reboot. Note that this does not detect if reboot was not performed because
// CCD wasn't open.
func (i *CrOSImage) SendConsoleRebootCmd(ctx context.Context) error {
	if err := i.WriteSerial(ctx, []byte("reboot\r")); err != nil {
		return err
	}
	return nil
}

// SetCCDCapabilities uses the `ccd` GSC console command to set the device
// capabilities to the given map.
func (i *CrOSImage) SetCCDCapabilities(ctx context.Context, capabilities map[CCDCap]CCDCapState) error {
	currentStates, err := i.GetCCDCapabilities(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get initial CCD states")
	}

	// Update capabilities if needed
	for c := range capabilities {
		if capabilities[c] != currentStates[c] {
			if err = i.SetCCDCapability(ctx, c, capabilities[c]); err != nil {
				return errors.Wrap(err, "failed to set CCD capability")
			}
		}
	}

	// TODO(b/307544573): Recheck states here?

	return nil
}

// runCommand executes the `cmd` string as a GSC console command and checks for
// basic error output.
func (i *CrOSImage) runCommand(ctx context.Context, cmd string) error {
	output, err := i.Command(ctx, cmd)
	if err != nil {
		return errors.Wrap(err, "failed to execute `"+cmd+"`")
	}
	if accessDeniedRE.MatchString(output) {
		return errors.Wrap(err, "got access denied when trying to run `"+cmd+"`")
	}
	return nil
}

// safeCommand restricts the console channel before running the requested
// command to prevent the output from being broken up.
func (i *CrOSImage) safeCommand(ctx context.Context, cmd string) (string, error) {
	if err := i.runCommand(ctx, "chan save"); err != nil {
		return "", errors.Wrap(err, "failed to save the channel")
	}

	// Restore the channel when finished
	defer func() {
		_ = i.runCommand(ctx, "chan restore")
	}()

	if err := i.runCommand(ctx, "chan 1"); err != nil {
		return "", errors.Wrap(err, "failed to set the channel to 1")
	}
	return i.Command(ctx, cmd)
}

// EcrstOff uses the `ecrst` GSC console command to deassert the EC reset line,
// `EC_RST_L`.
func (i *CrOSImage) EcrstOff(ctx context.Context) error {
	return i.runCommand(ctx, "ecrst off")
}

// EcrstOn uses the `ecrst` GSC console command to assert the EC reset line,
// `EC_RST_L`.
func (i *CrOSImage) EcrstOn(ctx context.Context) error {
	return i.runCommand(ctx, "ecrst on")
}

// EcrstPulse uses the `ecrst` GSC console command to pulse the EC reset line,
// `EC_RST_L`.
func (i *CrOSImage) EcrstPulse(ctx context.Context) error {
	return i.runCommand(ctx, "ecrst pulse")
}

// SysrstOff uses the `sysrst` GSC console command to deassert the AP reset
// line, `SYS_RST_L`.
func (i *CrOSImage) SysrstOff(ctx context.Context) error {
	return i.runCommand(ctx, "sysrst off")
}

// SysrstOn uses the `sysrst` GSC console command to assert the AP reset line,
// `SYS_RST_L`.
func (i *CrOSImage) SysrstOn(ctx context.Context) error {
	return i.runCommand(ctx, "sysrst on")
}

// SysrstPulse uses the `sysrst` GSC console command to pulse the AP reset line,
// `SYS_RST_L`.
func (i *CrOSImage) SysrstPulse(ctx context.Context) error {
	return i.runCommand(ctx, "sysrst pulse")
}

// TestlabOpen uses the `ccd testlab open` GSC console command to open CCD. Testlab mode
// must have been previously enabled
func (i *CrOSImage) TestlabOpen(ctx context.Context) error {
	return i.runCommand(ctx, "ccd testlab open")
}

// SetWp uses `wp` to set the current write protect value
func (i *CrOSImage) SetWp(ctx context.Context, enabled bool) error {
	return i.runCommand(ctx, "wp "+strconv.FormatBool(enabled))
}

// SetWpAtBoot uses `wp` to set the current and atboot write protect value
func (i *CrOSImage) SetWpAtBoot(ctx context.Context, enabled bool) error {
	return i.runCommand(ctx, "wp "+strconv.FormatBool(enabled)+" atboot")
}

// GetCCDLevel uses the `ccd` GSC console command to get the current CCD level
// state.
func (i *CrOSImage) GetCCDLevel(ctx context.Context) (CCDLevel, error) {
	output, err := i.safeCommand(ctx, "ccd")
	if err != nil {
		return Lock, errors.Wrap(err, "failed get CCD command output")
	}

	matches := consoleCCDStateRE.FindStringSubmatch(output)
	if len(matches) != 2 {
		return Lock, errors.Wrap(err, "regex failed to extract CCD state from: "+output)
	}

	// TODO(b/307544573): Make sure the string matching below works with Cr50 too
	level := matches[1]
	if level == "Opened" {
		return Open, nil
	} else if level == "Locked" {
		return Lock, nil
	} else if level == "Unlocked" {
		return Unlock, nil
	}

	return Lock, errors.Wrap(err, "failed parse CCD level")
}

// IsCCDOpen uses the `ccd` GSC console command to check if CCD is in the open
// state or not and returns true if so.
func (i *CrOSImage) IsCCDOpen(ctx context.Context) (bool, error) {
	level, err := i.GetCCDLevel(ctx)
	if level == Open {
		return true, err
	}
	return false, err
}

// GetVersionInfo uses the `version` GSC console command to returned information
// about the running firmware.
func (i *CrOSImage) GetVersionInfo(ctx context.Context) (VersionCommandInfo, error) {
	output, err := i.safeCommand(ctx, "version")
	if err != nil {
		return VersionCommandInfo{}, errors.Wrap(err, "failed to run GSC version command")
	}
	return matchVersionInfo(output)
}

// CheckRW returns an error if the rw information doesn't match the expected value.
func CheckRW(rw RwInfo, expectedVersion string, expectedDebug bool) bool {
	return expectedVersion == rw.Version && expectedDebug == rw.Debug
}

// ValidateVersionInfo validates the expected version is running.
func ValidateVersionInfo(version VersionCommandInfo, expectedVersion string, expectedDebug, checkBothSlots bool) bool {
	if version.RwA.Active || checkBothSlots {
		matches := CheckRW(version.RwA, expectedVersion, expectedDebug)
		if !matches || !checkBothSlots {
			return matches
		}
	}
	return CheckRW(version.RwB, expectedVersion, expectedDebug)
}

// CheckRunningVersion validates the expected version is running.
func (i *CrOSImage) CheckRunningVersion(ctx context.Context, expectedVersion string, expectedDebug, checkBothSlots bool) (bool, error) {
	versionInfo, err := i.GetVersionInfo(ctx)
	if err != nil {
		return false, err
	}
	matches := ValidateVersionInfo(versionInfo, expectedVersion, expectedDebug, checkBothSlots)
	desc := "Running"
	if !matches {
		desc = "Not running"
	}
	testing.ContextLogf(ctx, "RW_A: %+v", versionInfo.RwA)
	testing.ContextLogf(ctx, "RW_B: %+v", versionInfo.RwB)
	testing.ContextLogf(ctx, "%s %s", desc, expectedVersion)
	return matches, nil
}

func matchRoInfo(s string, slot GscSlot) (RoInfo, error) {
	slotStr := "A"
	if slot == SlotB {
		slotStr = "B"
	}

	ret := RoInfo{}
	verRE := regexp.MustCompile(`RO_` + slotStr + `:\s+([\s|*])\s([0-9.]+)\/([[:xdigit:]]+)`)
	matches := verRE.FindStringSubmatch(s)
	if len(matches) != 4 {
		return ret, errors.New("regex failed to extract ro info from: " + s)
	}
	ret.Version = matches[2]
	ret.ImageCheck = matches[3]

	if matches[1] == "*" {
		ret.Active = true
	}

	return ret, nil
}

func matchRwInfo(s string, slot GscSlot) (RwInfo, error) {
	slotStr := "A"
	if slot == SlotB {
		slotStr = "B"
	}

	verRE := regexp.MustCompile(`RW_` + slotStr + `:\s+([\s|*])\s(([0-9.]+)(/DBG)?/(` + verRWGSCStrRE + `)|Empty|Error)`)
	matches := verRE.FindStringSubmatch(s)

	// Manually figure out how many matches we got since `regexp` only returns
	// the maximum number of matches that can be returned.
	numMatches := 0
	for _, value := range matches {
		if value != "" {
			numMatches++
		}
	}

	ret := RwInfo{}
	if numMatches == 3 {
		ret.Empty = true
		return ret, nil
	} else if numMatches >= 7 {
		if matches[1] == "*" {
			ret.Active = true
		}
		ret.Version = matches[3]

		if matches[6] != "" {
			ret.Branch = getBranch(matches[6])
		} else {
			ret.Branch = getBranch(matches[8])
		}

		if numMatches == 8 {
			ret.Debug = true
		}

		return ret, nil
	}

	return RwInfo{}, errors.New("regex failed to extract rw info from: " + s)
}

var bidRe = `([[:xdigit:]]+):([[:xdigit:]]+):([[:xdigit:]]+)`

func matchBidInfo(s string, slot GscSlot) (BidInfo, error) {
	slotStr := "A"
	if slot == SlotB {
		slotStr = "B"
	}

	ret := BidInfo{}
	bidRE := regexp.MustCompile(`BID ` + slotStr + `:\s+` + bidRe)
	matches := bidRE.FindStringSubmatch(s)
	if len(matches) == 0 {
		ret.Empty = true
		return ret, nil
	} else if len(matches) != 4 {
		return BidInfo{}, errors.New("regex failed to extract bid info from: " + s)
	}

	hexToUint32 := func(str string) (uint32, error) {
		res, err := strconv.ParseInt(str, 16, 32)
		if err != nil {
			return 0, errors.Wrap(err, "could not parse hex string")
		}
		return uint32(res), nil
	}
	bidType, err := hexToUint32(matches[1])
	if err != nil {
		return ret, err
	}
	mask, err := hexToUint32(matches[2])
	if err != nil {
		return ret, err
	}
	flags, err := hexToUint32(matches[3])
	if err != nil {
		return ret, err
	}

	ret.Empty = false
	ret.BidType = bidType
	ret.Mask = mask
	ret.Flags = flags

	return ret, nil
}

func matchBuildInfo(s string) (BuildInfo, error) {
	ret := BuildInfo{}

	buildRE := regexp.MustCompile(`Build:\s+([0-9.]+(/DBG)?/` + verRWCr50StrRE + `|` + verRWTi50StrRE + `)`)
	matches := buildRE.FindStringSubmatch(s)
	if len(matches) != 7 {
		return ret, errors.New("regex failed to extract build info from: " + s)
	}
	if matches[3] != "" {
		ret.Branch = getBranch(matches[3])
	} else {
		ret.Branch = getBranch(matches[5])
	}
	if matches[2] != "" {
		ret.Debug = true
	}

	return ret, nil
}

func getBranch(s string) GscBranch {
	switch s {
	// DT/OT branch strings.
	case "tot":
		return ToT
	case "prepvt":
		return PrePvt
	case "mp":
		return MP
	// Cr50 branch strings.
	case "v2.0":
		return ToT
	case "v3.94_pp":
		return PrePvt
	case "v4.08_pp":
		return PrePvt
	case "v2.94_mp":
		return MP
	case "v4.11_mp":
		return MP
	case "v4.11_28_efi":
		return EFI
	default:
		return Unknown
	}
}

func matchVersionInfo(s string) (VersionCommandInfo, error) {
	ret := VersionCommandInfo{}
	roInfoA, err := matchRoInfo(s, SlotA)
	if err != nil {
		return ret, err
	}
	roInfoB, err := matchRoInfo(s, SlotB)
	if err != nil {
		return ret, err
	}
	rwInfoA, err := matchRwInfo(s, SlotA)
	if err != nil {
		return ret, err
	}
	rwInfoB, err := matchRwInfo(s, SlotB)
	if err != nil {
		return ret, err
	}
	bidInfoA, err := matchBidInfo(s, SlotA)
	if err != nil {
		return ret, err
	}
	bidInfoB, err := matchBidInfo(s, SlotB)
	if err != nil {
		return ret, err
	}
	buildInfo, err := matchBuildInfo(s)
	if err != nil {
		return ret, err
	}

	bidInfo := bidInfoA
	if rwInfoB.Active {
		bidInfo = bidInfoB
	}

	ret = VersionCommandInfo{
		RoA:   roInfoA,
		RoB:   roInfoB,
		RwA:   rwInfoA,
		RwB:   rwInfoB,
		Bid:   bidInfo,
		Build: buildInfo,
	}

	return ret, nil
}

// WaitUntilNormalSleep waits until gsc goes into deep sleep via monitoring print statement.
func (i *CrOSImage) WaitUntilNormalSleep(ctx context.Context, timeout time.Duration) error {
	_, err := i.WaitUntilMatch(ctx, normalSleep, timeout)
	return err
}

// WaitUntilDeepSleep waits until gsc goes into deep sleep via monitoring print statement.
func (i *CrOSImage) WaitUntilDeepSleep(ctx context.Context, timeout time.Duration) error {
	_, err := i.WaitUntilMatch(ctx, deepSleep, timeout)
	return err
}

// WaitUntilAnySleep waits until gsc goes into deep or normal sleep via monitoring print statement.
func (i *CrOSImage) WaitUntilAnySleep(ctx context.Context, timeout time.Duration) error {
	_, err := i.WaitUntilMatch(ctx, anySleep, timeout)
	return err
}

// WaitUntilRoBoot waits until initial RO console messages are printed which happens right after
// reboot or deep sleep resume.
func (i *CrOSImage) WaitUntilRoBoot(ctx context.Context, timeout time.Duration) error {
	_, err := i.WaitUntilMatch(ctx, roBoot, timeout)
	return err
}

// StartTestlabEnable starts the testlab enable process that will require power button pushes.
func (i *CrOSImage) StartTestlabEnable(ctx context.Context) error {
	// Use WriteSerial instead of Command so we do not consume the the power button prompt message
	// that happens before the next command prompt ( >). This allows WaitForPowerButtonPrompt to
	// work right after StartTestlabEnable
	return i.WriteSerial(ctx, []byte("ccd testlab enable\r"))
}

// WaitForPowerButtonPrompt waits for a power button prompt.
func (i *CrOSImage) WaitForPowerButtonPrompt(ctx context.Context, timeout time.Duration) error {
	_, err := i.WaitUntilMatch(ctx, pwrbPromptRE, timeout)
	return err
}

// WaitForTestlabEnable waits until a testlab enable message is printed.
func (i *CrOSImage) WaitForTestlabEnable(ctx context.Context, timeout time.Duration) error {
	_, err := i.WaitUntilMatch(ctx, testlabEnabledRE, timeout)
	return err
}

// WaitForTestlabDisable waits until a testlab disable message is printed.
func (i *CrOSImage) WaitForTestlabDisable(ctx context.Context, timeout time.Duration) error {
	_, err := i.WaitUntilMatch(ctx, testlabDisabledRE, timeout)
	return err
}

// GetBoardProperties gets the numerical value from the "brdprop" GSC command.
func (i *CrOSImage) GetBoardProperties(ctx context.Context) (uint64, error) {
	output, err := i.safeCommand(ctx, "brdprop")
	if err != nil {
		return 0, errors.Wrap(err, "failed to run GSC brdprop command")
	}
	matches := brdPropRE.FindStringSubmatch(output)
	brdprop, _ := strconv.ParseUint(matches[1], 16, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse brdprop value")
	}
	return brdprop, nil
}

// GetBoardPropertiesTPMBus uses the "brdprop" GSC command to discover
// transport of the TPM bus (SPI/I2C).
func (i *CrOSImage) GetBoardPropertiesTPMBus(ctx context.Context) (TpmBus, error) {
	brdprop, err := i.GetBoardProperties(ctx)
	if err != nil {
		return TpmBusInvalid, err
	}
	switch brdprop & 0x03 {
	case 0x01:
		return TpmBusSpi, nil
	case 0x02:
		return TpmBusI2c, nil
	default:
		return TpmBusInvalid, errors.Errorf("unrecognized brdprop value: 0x%08x", brdprop)
	}
}

// GSCTime contains the time since cold reset and the time since deep sleep reset
type GSCTime struct {
	// coldReset is the time since a cold reset (ex power-on, hard, security)
	coldResetTime time.Duration
	// dsTime is the time since deep sleep or any other reset.
	dsTime time.Duration
}

// extractGSCTime extracts the time since deep sleep and cold reset from the gettime output
func extractGSCTime(out string) (GSCTime, error) {
	ret := GSCTime{}

	// Find the deep sleep time
	m := gettimeDeepSleepRE.FindStringSubmatch(out)
	if m == nil {
		return ret, errors.New("failed to find deep sleep time in gettime output")
	}
	t, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return ret, err
	}
	ret.dsTime = time.Duration(t * float64(time.Second))

	// Find the cold reset time
	m = gettimeColdResetRE.FindStringSubmatch(out)
	if m == nil {
		return ret, errors.New("failed to find cold reset time in gettime output")
	}
	t, err = strconv.ParseFloat(m[1], 64)
	if err != nil {
		return ret, err
	}
	ret.coldResetTime = time.Duration(t * float64(time.Second))
	return ret, nil
}

// Gettime runs the gettime command and extracts the system time information
func (i *CrOSImage) Gettime(ctx context.Context) (GSCTime, error) {
	output, err := i.Command(ctx, "gettime")
	if err != nil {
		return GSCTime{}, errors.Wrap(err, "failed to run GSC gettime command")
	}
	return extractGSCTime(output)
}

// UsbDeviceLinkState contains all possible USB link states the GSC can detect.
type UsbDeviceLinkState uint

// USB device link states
const (
	UsbDisconnected UsbDeviceLinkState = iota
	SuzyQConnected
	SuzyQFlippedConnected
	ServoConnected
	ServoFlippedConnected
	ServoSink1Connected
	ServoSink2Connected
	ServoSink3Connected
)

// UsbAdcInfo contains information about the connected USB device and raw voltages
// read from the ADC.
type UsbAdcInfo struct {
	// UsbDeviceLinkState is the current state of the USB device link.
	State UsbDeviceLinkState
	// cc1Mv is the USB-C configuration channel 1 voltage.
	Cc1Mv uint
	// cc2Mv is the USB-C configuration channel 2 voltage.
	Cc2Mv uint
}

// GetUsbAdcInfo extracts the USB ADC information from `usb` command.
func (i *CrOSImage) GetUsbAdcInfo(ctx context.Context) (UsbAdcInfo, error) {
	output, err := i.Command(ctx, "usb")
	if err != nil {
		return UsbAdcInfo{}, errors.Wrap(err, "failed to run GSC `usb` command")
	}
	return matchUsbAdcInfo(output)
}

func matchUsbAdcInfo(s string) (UsbAdcInfo, error) {
	ret := UsbAdcInfo{}
	stateMatches := usbAdcStateRE.FindStringSubmatch(s)
	if len(stateMatches) != 4 {
		return ret, errors.New("regex failed to get correct matches from usb adc state from: " + s)
	}

	if stateMatches[1] != "" {
		return ret, errors.New("H1 does not support ADC readings")
	} else if stateMatches[2] == "disconnected" {
		ret.State = UsbDisconnected
	} else {
		switch stateMatches[3] {
		case "SuzyQ":
			ret.State = SuzyQConnected
		case "SuzyQFlipped":
			ret.State = SuzyQFlippedConnected
		case "Servo-src(Rp1A5/Rp3A0)":
			ret.State = ServoConnected
		case "Servo-src(Rp3A0/Rp1A5)":
			ret.State = ServoFlippedConnected
		case "Servo-snk(dut:RpUSB)":
			ret.State = ServoSink1Connected
		case "Servo-snk(dut:Rp1A5)":
			ret.State = ServoSink2Connected
		case "Servo-snk(dut:Rp3A0)":
			ret.State = ServoSink3Connected
		default:
			return ret, errors.New("regex failed to extract usb adc connected state from: " + s)
		}
	}

	cc1Match := usbAdcCc1RE.FindStringSubmatch(s)
	if len(cc1Match) != 2 {
		return ret, errors.New("regex failed to extract usb adc cc1 from: " + s)
	}
	cc1, err := strconv.ParseUint(cc1Match[1], 10, 32)
	if err != nil {
		return ret, errors.New("failed to parse cc1 as a uint: " + cc1Match[1])
	}
	ret.Cc1Mv = uint(cc1)

	cc2Match := usbAdcCc2RE.FindStringSubmatch(s)
	if len(cc2Match) != 2 {
		return ret, errors.New("regex failed to extract usb adc cc2 from: " + s)
	}
	cc2, err := strconv.ParseUint(cc2Match[1], 10, 32)
	if err != nil {
		return ret, errors.New("failed to parse cc2 as a uint: " + cc2Match[1])
	}
	ret.Cc2Mv = uint(cc2)

	return ret, nil
}

// GetRmaAuth runs the `rma_auth` command and returns the generate RMA challenge
// string. The method returns an empty string if a rma_auth rate limiting
// timeout has been triggered.
func (i *CrOSImage) GetRmaAuth(ctx context.Context) (string, error) {
	output, err := i.Command(ctx, "rma_auth")
	if err != nil {
		return "", errors.Wrap(err, "failed to run GSC `rma_auth` command")
	}

	return matchRmaChallenge(output)
}

func matchRmaChallenge(s string) (string, error) {
	matches := rmaAuthChallengeRE.FindStringSubmatch(s)
	if len(matches) != 4 {
		return "", errors.New("regex failed to get correct rma auth matches from: " + s)
	}

	if matches[1] != "" {
		return matches[1], nil
	} else if matches[2] != "" || matches[3] != "" {
		return "", nil
	}

	return "", errors.New("regex failed to process rma auth matches from: " + s)
}
