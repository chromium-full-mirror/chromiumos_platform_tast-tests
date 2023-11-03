// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
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
)

// CrOSImage interacts with a board running ti50.
type CrOSImage struct {
	*CommandImage
}

// NewCrOSImage creates a new CrOSImage.
func NewCrOSImage(board DevBoard) *CrOSImage {
	// Allow for timestamp to be present before prompt "[ 999999.999 C] > " or just "> "
	return &CrOSImage{CommandImage: NewCommandImage(board, "\n", "^(\\[[ 0-9.]+.\\] )?> ")}
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

// GetGscBranch uses the `version` GSC console command to extract the source
// branch that the GSC firmware was built from.
func (i *CrOSImage) GetGscBranch(ctx context.Context) (GscBranch, error) {
	ti50BranchRegexp := regexp.MustCompile(`Build:.+ti50_common_([a-z]+)[:-]`)
	output, err := i.safeCommand(ctx, "version")
	if err != nil {
		return MP, errors.Wrap(err, "failed to run GSC version command")
	}

	match := ti50BranchRegexp.FindStringSubmatch(output)
	if len(match) != 2 {
		return MP, errors.Wrap(err, "regex failed to extract branch information from: "+output)
	}

	switch match[1] {
	case "tot":
		return ToT, nil
	case "prepvt":
		return PrePvt, nil
	case "mp":
		return MP, nil
	}

	return MP, errors.Wrap(err, "unknown GSC branch")
}
