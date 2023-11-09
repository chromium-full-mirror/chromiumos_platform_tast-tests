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
	UartGscRxAPTx   CCDCap = "UartGscRxAPTx"
	UartGscTxAPRx   CCDCap = "UartGscTxAPRx"
	UartGscRxECTx   CCDCap = "UartGscRxECTx"
	UartGscTxECRx   CCDCap = "UartGscTxECRx"
	FlashAP         CCDCap = "FlashAP"
	FlashEC         CCDCap = "FlashEC"
	OverrideWP      CCDCap = "OverrideWP"
	RebootECAP      CCDCap = "RebootECAP"
	GscFullConsole  CCDCap = "GscFullConsole"
	UnlockNoReboot  CCDCap = "UnlockNoReboot"
	UnlockNoShortPP CCDCap = "UnlockNoShortPP"
	OpenNoTPMWipe   CCDCap = "OpenNoTPMWipe"
	OpenNoLongPP    CCDCap = "OpenNoLongPP"
	BatteryBypassPP CCDCap = "BatteryBypassPP"
	Unused          CCDCap = "Unused"
	I2C             CCDCap = "I2C"
	FlashRead       CCDCap = "FlashRead"
	OpenNoDevMode   CCDCap = "OpenNoDevMode"
	OpenFromUSB     CCDCap = "OpenFromUSB"
	OverrideBatt    CCDCap = "OverrideBatt"
	APROCheckVC     CCDCap = "APROCheckVC"
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
}

// CrOSImage interacts with a board running ti50.
type CrOSImage struct {
	*CommandImage
}

// Fatal facilitates failing a test or test fixture.
type Fatal interface {
	Fatalf(format string, args ...interface{})
}

// NewCrOSImage creates a new CrOSImage.
// ctx is used as the context for opening necessary ports.
func NewCrOSImage(ctx context.Context, board DevBoard) (*CrOSImage, error) {
	i, err := NewCommandImage(ctx, board, "\n", "^(\\[[ 0-9.]+.\\] )?> ")
	if err != nil {
		return nil, err
	}
	// Allow for timestamp to be present before prompt "[ 999999.999 C] > " or just "> "
	return &CrOSImage{CommandImage: i}, nil
}

// MustOpenNewCrOSImage is shorthand for NewCrOSImage that will Fatalf the test/fixture if failed.
func MustOpenNewCrOSImage(ctx context.Context, board DevBoard, failWith Fatal) *CrOSImage {
	i, err := NewCrOSImage(ctx, board)
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
	re := regexp.MustCompile(`(?s)Known commands:\s*(.*)HELP LIST`)
	m := re.FindStringSubmatch(out)
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

// GetCCDCapabilities uses the `ccd` GSC console command to return a map of all
// CCD capability states. Capabilities that are in their default states will be
// reported as their true states.
func (i *CrOSImage) GetCCDCapabilities(ctx context.Context) (map[CCDCap]CCDCapState, error) {
	// Regex to extract CCD states and resolve `Default` states to their true states.
	re := regexp.MustCompile(`(?:\s\s([A-Za-z1-9]+)\s+[Y-]\s0=Default\s\(([A-Za-z]+)\)|\s\s([A-Za-z1-9]+)\s+[Y-]\s[01]=([A-Za-z]+))`)
	var out map[CCDCap]CCDCapState

	output, err := i.safeCommand(ctx, "ccd")
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute ccd open")
	}

	matches := re.FindAllStringSubmatch(output, -1)
	if matches == nil {
		return nil, errors.New("failed to parse ccd output")
	}

	// Map regex result to typed result
	out = make(map[CCDCap]CCDCapState)
	for i := 1; i < len(matches); i++ {
		cap := CCDCap(matches[i][1])
		state := CCDCapState(matches[i][2])
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
	if err := i.board.WriteSerial(ctx, []byte("reboot")); err != nil {
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
	commandEchoRe := regexp.MustCompile(cmd)
	accessDeniedRe := regexp.MustCompile(`(?i)access denied`)
	output, err := i.Command(ctx, cmd)
	if err != nil {
		return errors.Wrap(err, "failed to execute `"+cmd+"`")
	}
	if accessDeniedRe.MatchString(output) {
		return errors.Wrap(err, "got access denied when trying to run `"+cmd+"`")
	}
	if !commandEchoRe.MatchString(output) {
		return errors.Wrap(err, "failed to detect command echo after running `"+cmd+"` from output: "+output)
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

// GetCCDLevel uses the `ccd` GSC console command to get the current CCD level
// state.
func (i *CrOSImage) GetCCDLevel(ctx context.Context) (CCDLevel, error) {
	output, err := i.safeCommand(ctx, "ccd")
	if err != nil {
		return Lock, errors.Wrap(err, "failed get CCD command output")
	}

	consoleCcdStateRegex := regexp.MustCompile("State: ([A-Za-z]+)")
	matches := consoleCcdStateRegex.FindStringSubmatch(output)
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

func matchRoInfo(s string, slot GscSlot) (RoInfo, error) {
	slotStr := "A"
	if slot == SlotB {
		slotStr = "B"
	}

	ret := RoInfo{}
	regexp := regexp.MustCompile(`RO_` + slotStr + `:\s+([\s|*])\s([0-9.]+)\/([[:xdigit:]]+)`)
	matches := regexp.FindStringSubmatch(s)
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

var cr50RwVerStrRe = `cr50_([a-z1-9]+)\S*-([[:xdigit:]]+)`
var ti50RwVerStrRe = `ti50_common_([a-z]+)\S*:(\S+)`
var gscRwVerStrRe = cr50RwVerStrRe + `|` + ti50RwVerStrRe

func matchRwInfo(s string, slot GscSlot) (RwInfo, error) {
	slotStr := "A"
	if slot == SlotB {
		slotStr = "B"
	}

	regexp := regexp.MustCompile(`RW_` + slotStr + `:\s+([\s|*])\s(([0-9.]+)/(` + gscRwVerStrRe + `)|Empty)`)
	matches := regexp.FindStringSubmatch(s)

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
		ret.Active = false
		ret.Empty = true
		return ret, nil
	} else if numMatches == 7 {
		ret.Empty = false
		if matches[1] == "*" {
			ret.Active = true
		} else {
			ret.Active = false
		}
		ret.Version = matches[3]

		if matches[5] != "" {
			ret.Branch = getBranch(matches[5])
		} else {
			ret.Branch = getBranch(matches[7])
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
	regexp := regexp.MustCompile(`BID ` + slotStr + `:\s+` + bidRe)
	matches := regexp.FindStringSubmatch(s)
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

	regexp := regexp.MustCompile(`Build:\s+([0-9.]+/` + cr50RwVerStrRe + `|` + ti50RwVerStrRe + `)`)
	matches := regexp.FindStringSubmatch(s)
	if len(matches) != 6 {
		return ret, errors.New("regex failed to extract build info from: " + s)
	}
	if matches[2] != "" {
		ret.Branch = getBranch(matches[2])
	} else {
		ret.Branch = getBranch(matches[4])
	}

	return ret, nil
}

func getBranch(s string) GscBranch {
	switch s {
	case "tot":
		return ToT
	case "prepvt":
		return PrePvt
	case "mp":
		return MP
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
func (i *CrOSImage) WaitUntilNormalSleep(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, normalSleep)
		return err
	}, &pOpts)
}

// WaitUntilDeepSleep waits until gsc goes into deep sleep via monitoring print statement.
func (i *CrOSImage) WaitUntilDeepSleep(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, deepSleep)
		return err
	}, &pOpts)
}

// WaitUntilAnySleep waits until gsc goes into deep or normal sleep via monitoring print statement.
func (i *CrOSImage) WaitUntilAnySleep(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, anySleep)
		return err
	}, &pOpts)
}

// WaitUntilRoBoot waits until initial RO console messages are printed which happens right after
// reboot or deep sleep resume.
func (i *CrOSImage) WaitUntilRoBoot(ctx context.Context, interval time.Duration) error {
	pOpts := testing.PollOptions{Timeout: interval}
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := i.board.ReadSerialSubmatch(ctx, roBoot)
		return err
	}, &pOpts)
}
