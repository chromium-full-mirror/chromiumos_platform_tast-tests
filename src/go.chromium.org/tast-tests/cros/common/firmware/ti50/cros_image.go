// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

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

// WaitForSleepTimeout is the maximum amount of time we should wait in a test
// for either normal/deep sleep. This is 10 seconds longer than max sleep delay.
const WaitForSleepTimeout = 70 * time.Second

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
	// AccessDeniedRE is the regex to check if access denied shows up in the command output
	AccessDeniedRE = regexp.MustCompile(`(?i)access denied`)
	// Regex to extract CCD states and resolve `Default` states to their true states.
	capDefaultRE = regexp.MustCompile(`(?:\s\s([A-Za-z1-9]+)\s+[Y\-]\s0=Default\s\(([A-Za-z]+)\)|\s\s([A-Za-z1-9]+)\s+[Y\-]\s[0-3]=([A-Za-z]+))`)
	// Regex to extract CCD level from the ccd ouutput
	consoleCCDLevelRE = regexp.MustCompile("State: ([A-Za-z]+)")
	// Regex to extract commands from help output
	knownCommandRE = regexp.MustCompile(`(?s)Known commands:\s*(.*)HELP LIST`)
	// Regex to wait for a power button prompt
	pwrbPromptRE = regexp.MustCompile("Press the physical button now")
	// Regex to wait until ccd testlab mode is enabled
	testlabDisabledRE = regexp.MustCompile("Updating testlab to false|CCD test lab mode disabled")
	// Regex to wait until ccd testlab mode is disabled
	testlabEnabledRE = regexp.MustCompile("Updating testlab to true|CCD test lab mode enabled")
	// GSC version strings.
	// Group 1 is the branch name.
	// Group 2 is the version sha
	verRWCr50StrRE       = `cr50_([0-9_vpmefi\.]*)\.[0-9]*[\-\+]([[:xdigit:]]+)`
	verRWTi50StrRE       = `ti50_common_([a-z]+)\S*:\S+[\-\+](\S+)`
	verRWLegacyTi50StrRE = `(ti50_common):\S+[\-\+](\S+)`
	verRWGSCStrRE        = verRWCr50StrRE + `|` + verRWTi50StrRE + `|` + verRWLegacyTi50StrRE
	hexRE                = `[0-9a-fA-F]+`
	// GSC board properties
	// ex properties = 0xa1234578
	brdPropRE = regexp.MustCompile(`properties = 0x(` + hexRE + `)`)
	// regex to parse ti50 gettime output
	// example ti50 output:
	//    gettime
	//    Time: 0x000000000000542d = 21.549 s
	//    Since reset: 0x000000000000542d = 21.549 s
	//    Since deep sleep: 0x0000000000001620 = 5.664 s
	gettimeTi50RE = regexp.MustCompile(`Since reset:.*\s([0-9.]+) s\s+Since deep sleep:.*\s([0-9.]+) s`)
	// regex to parse cr50 gettime output
	// example cr50 output:
	//    gettime
	//    Time: 0x00000000001ea5ff = 2.008575 s
	//    since cold_reset: 1683 s
	gettimeCr50RE = regexp.MustCompile(`Time:.*\s([0-9.]+) s\s+since cold_reset:.*\s([0-9.]+) s`)

	// USB ADC info regex
	usbAdcStateRE = regexp.MustCompile(`(PHY [AB])|ADC: (disconnected)|connected: ([\S]+)`)
	usbAdcCc1RE   = regexp.MustCompile(`ADC: CC1 = ([0-9]+) mV`)
	usbAdcCc2RE   = regexp.MustCompile(`ADC: CC2 = ([0-9]+) mV`)
	// regex to parse rma_auth output
	rmaAuthChallengeRE = regexp.MustCompile(`([A-Z0-9]{80})|(RMA Auth error)|(Must wait)`)
	// regex to find the chip type in H1 sysinfo output
	h1SysinfoChipRE = regexp.MustCompile(`B2-(D|C)`)
	// Rollback Bits are printed Flash/SlotA/SlotB
	// ? is used when the image has been invalidated and the number of
	// rollback bits is unknown
	// ex: 4/2/3
	// 4 bits are blown in flash
	// 2 bits are blown in the slot A image
	// 3 bits are blown in the slot B image
	SysinfoRollbackRE = regexp.MustCompile(`(?P<flash>[\d\?]*)/(?P<slotA>[\d\?]*)/(?P<slotB>[\d\?]*)`)
	// Old Ti50 image print the rollback bits SlotA/SlotB/Flash
	// each field is numBits.0 in RW and 0.numBits in RO.
	// If 128 bits are erased in a region, it shows up as 128.128
	// Ti50 prints "?.?" when the slot contents are invalid
	// ex: 128.128/?.?/4.0
	// 128 bits are blown in slot A
	// an unknown number of bits are blown in slot B
	// 4 bits are blown in flash
	legacyTi50SysinfoRollbackRE = regexp.MustCompile(`(?P<slotA>[\d\?]*).\S+/(?P<slotB>[\d\?]*).\S+/(?P<flash>[\d\?]*).\S+`)

	// regex expectation for output of `ccd lock`
	CCDLockedRE = regexp.MustCompile(`CCD [Ll]ocked`)
	// regex expectation for output of `ccd open`
	CCDOpenedRE = regexp.MustCompile(`CCD [Oo]pened`)

	// regexes to parse the sysinfo output
	// ex Cr50 output
	//    Reset flags: 0x00000140 (hibernate wake-pin)
	//    Reset count: 1
	//    Chip:        g cr50 B2-C
	//    RO keyid:    0xaa66150f
	//    RW keyid:    0x87b73b67
	//    DEV_ID:      0x00000000 0x00000000
	//    Rollback:    1/1/2 4/4/4
	//    TPM MODE:    enabled (0)
	//    Key Ladder:  prod
	// ex Ti50 output
	//    Reset flags: 0x00000001 (Cold)
	//    Reset count: 1
	//    Breadcrumbs: 0x0000000000001234
	//    Chip:        g Ti50 D3C1
	//    RO keyid:    0xabcdef01
	//    RW keyid:    0x23456789
	//    DEV_ID:      0x00000000 0x00000000
	//    Rollback:    0.3/0.3/0.3 4.0/?.?/4.0
	//    TPM MODE:    disabled (3)
	//    Key Ladder:  prod
	//    EK Cert:     Compliant
	sysinfoFactoryMode        = `Chip factory mode.`
	sysinfoResetFlagRE        = `Reset flags:\s+0x(?P<resetFlags>` + hexRE + `)\s+\([^)]*\)\s*`
	sysinfoRollbackDetectedRE = `(?P<rollbackDetected>Rollback detected)?\s*`
	sysinfoResetCountRE       = `Reset count:\s+(?P<resetCount>\d*)\s*`
	sysinfoBreadcrumbRE       = `(Breadcrumbs:\s+0x(?P<breadcrumbs>` + hexRE + `))?\s*`
	sysinfoChipRE             = `Chip:\s+g\s+(?P<chipName>Ti50|cr50) (?P<chipSKU>\S+)\s*`
	// Old Cr50 sysinfo output may print "(prod|dev)" after the keyids. Ignore it.
	// ex: RO keyid:    0xaa66150f(prod)
	sysinfoROKeyidRE = `RO keyid:\s+(?P<roKeyid>0x` + hexRE + `)(?:\(\S+\))?\s*`
	sysinfoRWKeyidRE = `RW keyid:\s+(?P<rwKeyid>0x` + hexRE + `)(?:\(\S+\))?\s*`
	sysinfoDevidRE   = `DEV_ID:\s+(?P<devid>0x` + hexRE + ` 0x` + hexRE + `)\s*`
	// Old Cr50 sysinfo output does not print the RO rollback info. Accept an empty string on cr50.
	sysinfoRollbackRE    = `Rollback:\s+(?P<roRollback>\S+)? (?P<rwRollback>\S+)\s*`
	sysinfoTPMModeRE     = `TPM [ModeODE]+:\s+(?P<tpmMode>enabled|disabled) \((?P<tpmModeStatus>[0-9])\)\s*`
	sysinfoKeyladderRE   = `Key Ladder:\s+(?P<keyladder>\S*)\s*`
	sysinfoEKCertRE      = `(EK Cert:\s+(?P<ekCert>\S+))?\s*`
	sysinfoFactoryModeRE = `(?P<factoryMode>` + sysinfoFactoryMode + `)?`

	sysinfoRE = regexp.MustCompile(sysinfoResetFlagRE + sysinfoRollbackDetectedRE + sysinfoResetCountRE + sysinfoBreadcrumbRE + sysinfoChipRE + sysinfoROKeyidRE + sysinfoRWKeyidRE + sysinfoDevidRE + sysinfoRollbackRE + sysinfoTPMModeRE + sysinfoKeyladderRE + sysinfoEKCertRE + sysinfoFactoryModeRE)
	// regex to parse the chip bid output
	// ex Cr50 output: Board ID: ffffffff:00000000, flags 00000010
	// ex Ti50 output: Board ID: ffffffff:00000000, flags: 00000010
	chipBIDRE = regexp.MustCompile(`Board ID:\s*(` + hexRE + `):(` + hexRE + `),\s*flags:?\s*(` + hexRE + `)`)
	// regex to parse the factory config output
	// ex Cr50 output: fc = 0x0000000000001234
	// ex Ti50 output: Factory config: 0x0000000000001234
	factoryConfigRE = regexp.MustCompile(`(fc =|Factory config:)\s*0x(` + hexRE + `)`)
	boardConfigRE   = regexp.MustCompile(`board_cfg =\s*0x(` + hexRE + `)`)
	// Example ap_ro_info output.
	// hash saved
	//      result    : 0
	//      supported : no (14)
	//      gbbd      : na (10)
	//      sha256 hash b0863a864df1a0f2d4df2393ae4390fed6e1f01ccbdd562969da9b530e8ece2e
	//      Covered ranges:
	//      00c10000...00c107ff
	//      00c11000...00cfffff
	// no hash
	//      result    : 0
	//      supported : no (10)
	// match 1 is the integer result
	cr50APROResultRE = regexp.MustCompile(`result\s*:\s*(\d*)`)
	// match 1 is yes or no
	// match 2 is the integer reason for why AP RO verification is supported/unsupported
	cr50APROSupportedRE = regexp.MustCompile(`supported\s*:\s*(yes|no)\s*\((\d*)\)`)
	// match 1 is the value of the saved digest
	cr50APROHashRE = regexp.MustCompile(`sha256 hash ([a-f0-9]*)`)
	// Regex to extract all of the ccdstate key value pairs
	ccdStateRE = regexp.MustCompile(`([\S ]*)\s*: *([\S ]*)\s*`)
	// Regex to match all of the "on" states
	ccdStateOnRE = regexp.MustCompile(`\b(on|asserted|enabled|connected)\b`)
	// Regex to match all of the "off" states
	ccdStateOffRE = regexp.MustCompile(`\b(off|deasserted|disabled|disconnected)\b`)
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
	BID   ImageBID
	Build BuildInfo
}

// ActiveRw returns a reference to the active RW slot.
func (v VersionCommandInfo) ActiveRw() RwInfo {
	if v.RwA.Active {
		return v.RwA
	}
	return v.RwB
}

// InactiveRw returns a reference to the inactive RW slot.
func (v VersionCommandInfo) InactiveRw() RwInfo {
	if !v.RwA.Active {
		return v.RwA
	}
	return v.RwB
}

// RoInfo contains information about a loaded ro image slot.
type RoInfo struct {
	Active     bool
	Version    string
	ImageCheck string
}

// RwInfo contains information about a loaded rw image slot.
type RwInfo struct {
	Empty      bool
	Active     bool
	Debug      bool
	Version    string
	VersionStr string
	Branch     GscBranch
}

// ImageBID contains board id information for a slot.
type ImageBID struct {
	Empty bool
	Type  BIDField
	Mask  BIDField
	Flags BIDField
}

// BuildInfo contains information about the firmware currently running.
type BuildInfo struct {
	Branch     GscBranch
	Debug      bool
	VersionStr string
}

// CrOSImage interacts with a board running ti50.
type CrOSImage struct {
	*CommandImage
	TestbedType TestbedType
}

// Fatal facilitates failing a test or test fixture.
type Fatal interface {
	Fatalf(format string, args ...interface{})
}

// OpenCrOSImage creates a new CrOSImage. This must be closed to free connection.
func OpenCrOSImage(ctx context.Context, console SerialChannel, testbed TestbedType) (*CrOSImage, error) {
	i, err := OpenCommandImage(ctx, console, "\n", "^(\\[[ 0-9.]+.\\] )?> ")
	if err != nil {
		return nil, err
	}
	// Allow for timestamp to be present before prompt "[ 999999.999 C] > " or just "> "
	return &CrOSImage{CommandImage: i, TestbedType: testbed}, nil
}

// MustOpenCrOSImage is shorthand for OpenCrOSImage that will Fatalf the test/fixture if failed.
func MustOpenCrOSImage(ctx context.Context, console SerialChannel, failWith Fatal, testbed TestbedType) *CrOSImage {
	i, err := OpenCrOSImage(ctx, console, testbed)
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
	cmds := strings.Fields(string(m[1]))
	return CrOSImageHelpOutput{out, cmds}, nil
}

// CCDLock uses the `ccd` GSC console command to set the CCD level to locked.
func (i *CrOSImage) CCDLock(ctx context.Context) error {
	output, err := i.Command(ctx, "ccd lock")
	if err != nil {
		return errors.Wrap(err, "failed to execute ccd lock")
	}
	if CCDLockedRE.FindStringSubmatch(output) == nil {
		return errors.Errorf("unexpected output to ccd lock: %s", output)
	}
	return nil
}

// CCDOpen uses the `ccd` GSC console command to set the CCD level to open.
func (i *CrOSImage) CCDOpen(ctx context.Context) error {
	output, err := i.Command(ctx, "ccd open")
	if err != nil {
		return errors.Wrap(err, "failed to execute ccd open")
	}
	if CCDOpenedRE.FindStringSubmatch(output) == nil {
		return errors.Errorf("unexpected output to ccd open: %s", output)
	}
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

// CCDCapabilities uses the `ccd` GSC console command to return a map of all
// CCD capability states. Capabilities that are in their default states will be
// reported as their true states.
func (i *CrOSImage) CCDCapabilities(ctx context.Context) (map[CCDCap]CCDCapState, error) {
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

// CCDCapability uses the `ccd` GSC console command to return the state of
// the requested CCD capability.
func (i *CrOSImage) CCDCapability(ctx context.Context, capability CCDCap) (CCDCapState, error) {
	states, err := i.CCDCapabilities(ctx)
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

// SendDBGConsoleCrashCmd issues the crashme command but does not listen for a
// response since the GSC is expected to reboot/crash. Note that this does not
// detect if crash didn't happen due not using a DBG image.
func (i *CrOSImage) SendDBGConsoleCrashCmd(ctx context.Context) error {
	if i.TestbedType == GscH1Shield {
		return i.WriteSerial(ctx, []byte("crash divzero\r"))
	}
	return i.WriteSerial(ctx, []byte("crash watchdog\r"))
}

// Reboot issues the reboot command and waits until GSC stops printing output.
func (i *CrOSImage) Reboot(ctx context.Context) error {
	if err := i.SendConsoleRebootCmd(ctx); err != nil {
		return err
	}
	return i.WaitUntilBooted(ctx)
}

// Rollback issues the rollback command but does not listen for a response since the GSC
// is expected to reboot. Note that this does not detect if rollback was not performed because
// it's missing from the image.
func (i *CrOSImage) Rollback(ctx context.Context) error {
	if err := i.WriteSerial(ctx, []byte("rollback\r")); err != nil {
		return err
	}
	return i.WaitUntilBooted(ctx)
}

// SetCCDCapabilities uses the `ccd` GSC console command to set the device
// capabilities to the given map.
func (i *CrOSImage) SetCCDCapabilities(ctx context.Context, capabilities map[CCDCap]CCDCapState) error {
	currentStates, err := i.CCDCapabilities(ctx)
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
	if AccessDeniedRE.MatchString(output) {
		return errors.Wrap(err, "got access denied when trying to run `"+cmd+"`")
	}
	return nil
}

// safeCommand restricts the console channel before running the requested
// command to prevent the output from being broken up.
func (i *CrOSImage) safeCommand(ctx context.Context, cmd string) (string, error) {
	if i.TestbedType == GscH1Shield {
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

// CCDLevel uses the `ccd` GSC console command to get the current CCD level
// state.
func (i *CrOSImage) CCDLevel(ctx context.Context) (CCDLevel, error) {
	output, err := i.safeCommand(ctx, "ccd")
	if err != nil {
		return Lock, errors.Wrap(err, "failed get CCD command output")
	}

	matches := consoleCCDLevelRE.FindStringSubmatch(output)
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
	level, err := i.CCDLevel(ctx)
	if level == Open {
		return true, err
	}
	return false, err
}

// VersionInfo uses the `version` GSC console command to returned information
// about the running firmware.
func (i *CrOSImage) VersionInfo(ctx context.Context) (VersionCommandInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := i.safeCommand(ctx, "version")
	if err != nil {
		return VersionCommandInfo{}, errors.Wrap(err, "failed to run GSC version command")
	}
	return matchVersionInfo(output)
}

// CheckRW returns true if the rw information matches the expected value.
func CheckRW(rw RwInfo, expectedVersion string) bool {
	return expectedVersion == rw.Version
}

// ValidateVersionInfo validates the expected version is running.
func ValidateVersionInfo(version VersionCommandInfo, expectedVersion string, checkBothSlots bool) bool {
	if version.RwA.Active || checkBothSlots {
		matches := CheckRW(version.RwA, expectedVersion)
		if !matches || !checkBothSlots {
			return matches
		}
	}
	matches := CheckRW(version.RwB, expectedVersion)
	// Check the version string and DBG status match each other to verify
	// the exact same image is running in both slots.
	if checkBothSlots && matches {
		return (version.RwA.Debug == version.RwB.Debug &&
			version.RwA.VersionStr == version.RwB.VersionStr)
	}
	return matches
}

// CheckRunningVersion validates the expected version is running.
func (i *CrOSImage) CheckRunningVersion(ctx context.Context, expectedVersion string, checkBothSlots bool) (bool, error) {
	versionInfo, err := i.VersionInfo(ctx)
	if err != nil {
		return false, err
	}
	matches := ValidateVersionInfo(versionInfo, expectedVersion, checkBothSlots)
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
	verRE := regexp.MustCompile(`RO_` + slotStr + `:\s+([\s|*])\s([0-9.]+)\/([[:xdigit:]]+)|Empty`)
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
		ret.VersionStr = matches[5]

		if matches[6] != "" {
			ret.Branch = getBranch(matches[6])
		} else {
			ret.Branch = getBranch(matches[8])
		}

		if numMatches == 8 {
			ret.Debug = true
			ret.VersionStr = "DBG/" + ret.VersionStr
		}

		return ret, nil
	}

	return RwInfo{}, errors.New("regex failed to extract rw info from: " + s)
}

var bidRe = `([[:xdigit:]]+):([[:xdigit:]]+):([[:xdigit:]]+)`

func matchImageBID(s string, slot GscSlot) (ImageBID, error) {
	slotStr := "A"
	if slot == SlotB {
		slotStr = "B"
	}

	ret := ImageBID{}
	bidRE := regexp.MustCompile(`BID ` + slotStr + `:\s+` + bidRe)
	matches := bidRE.FindStringSubmatch(s)
	if len(matches) == 0 {
		ret.Empty = true
		return ret, nil
	} else if len(matches) != 4 {
		return ImageBID{}, errors.New("regex failed to extract bid info from: " + s)
	}

	hexToBIDField := func(str string) (BIDField, error) {
		res, err := strconv.ParseUint(str, 16, 32)
		if err != nil {
			return 0, errors.Wrap(err, "could not parse hex string")
		}
		return BIDField(res), nil
	}
	bidType, err := hexToBIDField(matches[1])
	if err != nil {
		return ret, err
	}
	mask, err := hexToBIDField(matches[2])
	if err != nil {
		return ret, err
	}
	flags, err := hexToBIDField(matches[3])
	if err != nil {
		return ret, err
	}

	ret.Empty = false
	ret.Type = bidType
	ret.Mask = mask
	ret.Flags = flags

	return ret, nil
}

func matchBuildInfo(s string) (BuildInfo, error) {
	ret := BuildInfo{}

	buildRE := regexp.MustCompile(`Build:\s+([0-9./]*(DBG)?/?(` + verRWGSCStrRE + `))`)
	matches := buildRE.FindStringSubmatch(s)
	if len(matches) != 10 {
		return ret, errors.Errorf("regex failed to extract build info from found %d matches in %s: %s", len(matches), s, matches)
	}
	ret.VersionStr = matches[3]
	if matches[4] != "" {
		ret.Branch = getBranch(matches[4])
	} else {
		ret.Branch = getBranch(matches[6])
	}
	if matches[2] != "" {
		ret.Debug = true
		ret.VersionStr = "DBG/" + ret.VersionStr
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
	case "v5.55_pp":
		return PrePvt
	case "v6.55_pp":
		return PrePvt
	case "v2.94_mp":
		return MP
	case "v4.11_mp":
		return MP
	case "v5.60_mp":
		return MP
	case "v6.60_mp":
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
	imageBIDA, err := matchImageBID(s, SlotA)
	if err != nil {
		return ret, err
	}
	imageBIDB, err := matchImageBID(s, SlotB)
	if err != nil {
		return ret, err
	}
	buildInfo, err := matchBuildInfo(s)
	if err != nil {
		return ret, err
	}

	imageBID := imageBIDA
	if rwInfoB.Active {
		imageBID = imageBIDB
	}

	ret = VersionCommandInfo{
		RoA:   roInfoA,
		RoB:   roInfoB,
		RwA:   rwInfoA,
		RwB:   rwInfoB,
		BID:   imageBID,
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

// WaitUntilDeepSleepWake waits until gsc prints one of the Ti50/Cr50 startup messages
func (i *CrOSImage) WaitUntilDeepSleepWake(ctx context.Context, timeout time.Duration) (string, error) {
	out, err := i.WaitUntilMatch(ctx, rwBoot, timeout)
	if err != nil {
		return "", err
	}
	return string(out[0]), nil
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

const (
	// BoardPropSPI is the GSC board property mask for SPI TPMs
	BoardPropSPI = (1 << 0)
	// BoardPropI2C is the GSC board property mask for I2C TPMs
	BoardPropI2C = (1 << 1)
	// BoardPropSysRst is the GSC board property mask devices that use SYS_RST_L for TPM_RST_L
	BoardPropSysRst = (1 << 5)
	// BoardPropPltRst is the GSC board property mask devices that use PLT_RST_L for TPM_RST_L
	BoardPropPltRst = (1 << 6)
)

// BoardProperties gets the numerical value from the "brdprop" GSC command.
func (i *CrOSImage) BoardProperties(ctx context.Context) (uint64, error) {
	output, err := i.safeCommand(ctx, "brdprop")
	if err != nil {
		return 0, errors.Wrap(err, "failed to run GSC brdprop command")
	}
	matches := brdPropRE.FindStringSubmatch(output)
	if matches == nil || len(matches) != 2 {
		return 0, errors.Errorf("failed to find %s in brdprop output %s", brdPropRE, output)
	}
	brdprop, err := strconv.ParseUint(matches[1], 16, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse brdprop value")
	}
	return brdprop, nil
}

// BoardPropertiesTPMBus uses the "brdprop" GSC command to discover
// transport of the TPM bus (SPI/I2C).
func (i *CrOSImage) BoardPropertiesTPMBus(ctx context.Context) (TpmBus, error) {
	brdprop, err := i.BoardProperties(ctx)
	if err != nil {
		return TpmBusInvalid, err
	}
	switch brdprop & (BoardPropSPI | BoardPropI2C) {
	case BoardPropSPI:
		return TpmBusSpi, nil
	case BoardPropI2C:
		return TpmBusI2c, nil
	default:
		return TpmBusInvalid, errors.Errorf("unrecognized brdprop value: 0x%08x", brdprop)
	}
}

// GSCTime contains the time since cold reset and deep sleep reset.
type GSCTime struct {
	// ColdResetTime is the time since a cold reset.
	ColdResetTime time.Duration
	// DeepSleepTime is the time since deep sleep.
	DeepSleepTime time.Duration
}

// extractGSCTime extracts the times from the gettime output
func extractGSCTime(out string) (GSCTime, error) {
	var ret GSCTime
	var coldResetTime string
	var dsTime string

	// Find the cold reset and deep sleep time.
	if m := gettimeTi50RE.FindStringSubmatch(out); m != nil {
		coldResetTime = m[1]
		dsTime = m[2]
	} else if m := gettimeCr50RE.FindStringSubmatch(out); m != nil {
		dsTime = m[1]
		coldResetTime = m[2]
	} else {
		return ret, errors.New("failed to match gettime output")
	}

	t, err := strconv.ParseFloat(coldResetTime, 64)
	if err != nil {
		return ret, err
	}
	ret.ColdResetTime = time.Duration(t * float64(time.Second))

	t, err = strconv.ParseFloat(dsTime, 64)
	if err != nil {
		return ret, err
	}
	ret.DeepSleepTime = time.Duration(t * float64(time.Second))

	return ret, nil
}

// Time runs the gettime command and extracts the system time information
func (i *CrOSImage) Time(ctx context.Context) (GSCTime, error) {
	output, err := i.Command(ctx, "gettime")
	if err != nil {
		return GSCTime{}, errors.Wrap(err, "failed to run GSC gettime command")
	}
	return extractGSCTime(output)
}

// UsbDeviceLinkState contains all possible USB link states the GSC can detect.
type UsbDeviceLinkState string

// USB device link states
const (
	UsbDisconnected          UsbDeviceLinkState = "UsbDisconnected"
	SuzyQConnected           UsbDeviceLinkState = "SuzyQConnected"
	Servo1A5FlippedConnected UsbDeviceLinkState = "Servo1A5FlippedConnected"
	ServoConnected           UsbDeviceLinkState = "ServoConnected"
	ServoFlippedConnected    UsbDeviceLinkState = "ServoFlippedConnected"
	ServoSink1Connected      UsbDeviceLinkState = "ServoSink1Connected"
	ServoSink2Connected      UsbDeviceLinkState = "ServoSink2Connected"
	ServoSink3Connected      UsbDeviceLinkState = "ServoSink3Connected"
)

// USBADCInfo contains information about the connected USB device and raw voltages
// read from the ADC.
type USBADCInfo struct {
	// UsbDeviceLinkState is the current state of the USB device link.
	State UsbDeviceLinkState
	// cc1Mv is the USB-C configuration channel 1 voltage.
	Cc1Mv uint
	// cc2Mv is the USB-C configuration channel 2 voltage.
	Cc2Mv uint
}

// USBADCInfo extracts the USB ADC information from `usb` command.
func (i *CrOSImage) USBADCInfo(ctx context.Context) (USBADCInfo, error) {
	output, err := i.Command(ctx, "usb")
	if err != nil {
		return USBADCInfo{}, errors.Wrap(err, "failed to run GSC `usb` command")
	}
	return matchUSBADCInfo(output)
}

func matchUSBADCInfo(s string) (USBADCInfo, error) {
	ret := USBADCInfo{}
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
		case "Servo-src(RpUSB/Rp1A5)":
			ret.State = Servo1A5FlippedConnected
		case "Servo-src(Rp3A0/Rp1A5)":
			ret.State = ServoConnected
		case "Servo-src(Rp1A5/Rp3A0)":
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

// RMAAuth runs the `rma_auth` command and returns the generate RMA challenge
// string. The method returns an empty string if a rma_auth rate limiting
// timeout has been triggered.
func (i *CrOSImage) RMAAuth(ctx context.Context) (string, error) {
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

// FindChipSKU finds the chip sku in the sysinfo output
func FindChipSKU(output string) ChipSKU {
	matches := h1SysinfoChipRE.FindStringSubmatch(output)
	if matches == nil {
		return SKUDT
	}
	if matches[1] == "D" {
		return SKUH1Detachable
	}
	return SKUH1Clamshell
}

// ChipSKU contains possible CCD levels.
type ChipSKU string

// Possible CCD levels.
const (
	// SKUH1Detachable is the sysinfo string used for H1 Detachable chips
	SKUH1Detachable ChipSKU = "H1-D"
	// SKUH1Clamshell is the sysinfo string used for H1 Clamshell chips
	SKUH1Clamshell ChipSKU = "H1-C"
	// SKUDT is the sysinfo string used for the DT chip
	SKUDT ChipSKU = "D3C1"
)

// Sysinfo returns current sysinfo state
func (i *CrOSImage) Sysinfo(ctx context.Context) (Sysinfo, error) {
	out, err := i.safeCommand(ctx, "sysinfo")
	if err != nil {
		return Sysinfo{}, errors.Wrap(err, "unable to run sysinfo")
	}
	sysinfoMap, err := parseSysinfo(out)
	if err != nil {
		return Sysinfo{}, errors.Wrap(err, "unable to parse sysinfo output")
	}
	return getSysinfoStruct(sysinfoMap)
}

// parseSysinfo converts the sysinfo output into a map
func parseSysinfo(output string) (map[string]string, error) {
	result := make(map[string]string)
	match := sysinfoRE.FindStringSubmatch(output)
	if match == nil {
		return result, errors.Errorf("could not find %s in %s", sysinfoRE, output)
	}
	for i, name := range sysinfoRE.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	return result, nil
}

const (
	// InvalidBits is used when the bit value is set to "?"
	InvalidBits uint8 = 255
)

// SysinfoRollbackSlot contains structured information about the rollback bits
type SysinfoRollbackSlot struct {
	// Bits is the number of bits blown
	Bits uint8
	// Some boards wipe inactive slots. The rollback bits may not be valid
	// in one of the image regions
	// Valid is True if the rollback bits are valid
	Valid bool
}

// SysinfoRollbackBits contains structured information about the rollback bits
type SysinfoRollbackBits struct {
	// SlotA is the rollback information for slot A
	SlotA SysinfoRollbackSlot
	// SlotB is the rollback information for slot B
	SlotB SysinfoRollbackSlot
	// Flash is the rollback information for the bits currently blown in flash
	Flash SysinfoRollbackSlot
}

// Sysinfo contains structured information returned from the GSC
// `sysinfo` command.
type Sysinfo struct {
	// OriginalResetFlags is the original reset flag value from the sysinfo output
	OriginalResetFlags uint32
	// ResetFlags is the sysinfo reset flag with unified values between cr50 and ti50
	ResetFlags uint32
	// ResetCount is the number of times GSC has reset without clearing the counter
	ResetCount uint32
	// Breadcrumbs is a Ti50 field that track boot events
	Breadcrumbs string
	// ChipName is the chip specific name Ti50 or Cr50
	ChipName string
	// ChipSKU is the chip sku from the sysinfo output
	ChipSKU ChipSKU
	// ROKeyid is the key id used to sign the active RO
	ROKeyid string
	// RWKeyid is the key id used to sign the active RW
	RWKeyid string
	// Devid is the chip devid
	Devid string
	// RORollback is a string describing the rollback bits blown in the chip and the RO A/B image
	RORollback string
	// RWRollback is a string describing the rollback bits blown in the chip and the RW A/B image
	RWRollback string
	// RWRollbackBits is a SysinfoRollbackBits with number of bits blown in each RW section
	RWRollbackBits SysinfoRollbackBits
	// TpmMode is the tpm mode state string
	TpmMode string
	// TpmEnabled is True if the tpm is enabled
	TpmEnabled bool
	// TpmModeStatus is the integer value of the tpm mode
	TpmModeStatus uint32
	// Keyladder is "prod" if GSC is using the prod keyladder "dev" if it isn't
	Keyladder string
	// ProdKeyladder is true if the prod keyladder is enabled
	ProdKeyladder bool
	// FactoryModeValid is true when the InFactoryMode field is valid
	FactoryModeValid bool
	// InFactoryMode is true if sysinfo prints the chip is in factory mode (ti50 only)
	InFactoryMode bool
	// EKCert is the EK Cert value
	EKCert string
	// RollbackDetected is true if sysinfo prints a rollback is detected
	RollbackDetected bool
}

const (
	// Cr50ResetFlagPowerOn is set when Cr50 did a Power-on reset
	Cr50ResetFlagPowerOn = (1 << 3)
	// Cr50ResetFlagHibernate is set when Cr50 woke from deep sleep
	Cr50ResetFlagHibernate = (1 << 6)
	// Cr50ResetFlagRTCAlarm is set when Cr50 woke from rtc-alarm
	Cr50ResetFlagRTCAlarm = (1 << 7)
	// Cr50ResetFlagWakePin is set when Cr50 woke from a wake-pin
	Cr50ResetFlagWakePin = (1 << 8)
	// Cr50ResetFlagHard is set when Cr50 requests a hard reset
	Cr50ResetFlagHard = (1 << 11)
	// Cr50ResetFlagRdd is set when Rdd woke Cr50
	Cr50ResetFlagRdd = (1 << 15)
	// Cr50ResetFlagRbox is set when Cr50 resumes because of an rbox wake source
	Cr50ResetFlagRbox = (1 << 16)

	// GscResetFlagPowerOn is set when GSC did a Power-on reset
	GscResetFlagPowerOn = 1
	// GscResetFlagHibernate is set when Cr50 woke from deep sleep
	GscResetFlagHibernate = (1 << 1)
	// GscResetFlagHard is set when GSC requests a hard reset
	GscResetFlagHard = (1 << 5)
	// GscResetBrownOut is set when GSC reset due to power loss
	GscResetBrownOut = (1 << 6)
	// GscResetFlagRbox is set when GSC resumes because of an rbox wake source
	GscResetFlagRbox = (1 << 8)
	// GscResetFlagRdd is when when Rdd woke GSC
	GscResetFlagRdd = (1 << (31 - 0))

	// OTResetFlagPowerOn is set when OpenTitan did a Power-on reset
	OTResetFlagPowerOn = 1
	// OTResetFlagHibernate is set when OpenTitan woke from deep sleep
	OTResetFlagHibernate = (1 << 1)
	// OTResetFlagHard is set when OpenTitan requests a hard reset
	OTResetFlagHard = (1 << 2)
	// OTResetFlagRbox is set when OpenTitan resumes because of an rbox wake source
	OTResetFlagRbox = (1 << 3)
	// OTResetBrownOut is set when OpenTitan reset due to power loss
	OTResetBrownOut = (1 << 5)
)

func convertCr50ResetFlags(flags uint64) uint32 {
	res := uint32(0)
	if flags&Cr50ResetFlagPowerOn != 0 {
		res |= GscResetFlagPowerOn
	}
	if flags&Cr50ResetFlagHibernate != 0 {
		res |= GscResetFlagHibernate
	}
	if flags&Cr50ResetFlagHard != 0 {
		res |= GscResetFlagHard
	}
	if flags&Cr50ResetFlagRdd != 0 {
		res |= GscResetFlagRdd
	}
	if flags&Cr50ResetFlagRbox != 0 {
		res |= GscResetFlagRbox
	}
	return res
}

func convertOTResetFlags(flags uint64) uint32 {
	res := uint32(0)
	if flags&OTResetFlagPowerOn != 0 {
		res |= GscResetFlagPowerOn
	}
	if flags&OTResetFlagHibernate != 0 {
		res |= GscResetFlagHibernate
	}
	if flags&OTResetFlagHard != 0 {
		res |= GscResetFlagHard
	}
	if flags&OTResetFlagRbox != 0 {
		res |= GscResetFlagRbox
	}
	if flags&OTResetBrownOut != 0 {
		res |= GscResetBrownOut
	}
	return res
}

// parseSysinfoRollbackBits converts the sysinfo output into a map
func parseSysinfoRollbackBits(output string) (map[string]uint8, error) {
	result := make(map[string]uint8)
	rollbackRE := SysinfoRollbackRE
	match := rollbackRE.FindStringSubmatch(output)
	if match == nil {
		rollbackRE = legacyTi50SysinfoRollbackRE
		match = rollbackRE.FindStringSubmatch(output)
		if match == nil {
			return result, errors.Errorf("could not extract rollback bits %s", output)
		}
	}
	for i, name := range rollbackRE.SubexpNames() {
		if i != 0 && name != "" {

			// If rollback bits are set to "?", the image is
			// invalid, but this is not a error. It's part of
			// normal operation.
			if match[i] == "?" {
				// GSCs have a max of 128 bits. Tests should
				// check the Valid state. Make the Bits state
				// obviously wrong.
				result[name] = InvalidBits
				continue
			}
			bits, err := strconv.ParseUint(match[i], 10, 8)
			if err != nil {
				return result, errors.Wrapf(err, "could not parse %s rollback bits %s", name, match[i])
			}
			result[name] = uint8(bits)
		}
	}
	return result, nil
}

// findSysinfoRollbackBits returns the full sysinfoRollback structure
func findSysinfoRollbackBits(input string, isOT bool) (SysinfoRollbackBits, error) {
	result := SysinfoRollbackBits{}
	rollbackMap, err := parseSysinfoRollbackBits(input)
	if err != nil {
		return SysinfoRollbackBits{}, errors.Wrap(err, "unable to parse rollback output")
	}
	result.SlotA.Bits = rollbackMap["slotA"]
	result.SlotA.Valid = result.SlotA.Bits != InvalidBits
	result.SlotB.Bits = rollbackMap["slotB"]
	result.SlotB.Valid = result.SlotB.Bits != InvalidBits
	result.Flash.Bits = rollbackMap["flash"]
	result.Flash.Valid = result.Flash.Bits != InvalidBits
	if isOT {
		// On OT the number of bits blown in flash is unreadable: ?.?/?.?/?.?.
		// Ignore the error, otherwise it will cause most tests to fail.
		result.Flash.Bits = 0
	} else if !result.Flash.Valid {
		return SysinfoRollbackBits{}, errors.New("The number of bits blown in flash was unreadable " + input)
	}
	return result, nil
}

// getSysinfoStruct returns the full sysinfo structure
func getSysinfoStruct(input map[string]string) (Sysinfo, error) {
	result := Sysinfo{}

	result.ChipName = input["chipName"]
	result.ChipSKU = FindChipSKU(input["chipSKU"])
	result.ROKeyid = input["roKeyid"]
	result.RWKeyid = input["rwKeyid"]
	result.Devid = input["devid"]

	result.RORollback = input["roRollback"]
	result.RWRollback = input["rwRollback"]
	result.TpmMode = input["tpmMode"]
	result.Keyladder = input["keyladder"]
	result.Breadcrumbs = input["breadcrumbs"]
	result.EKCert = input["ekCert"]
	result.InFactoryMode = false
	result.FactoryModeValid = false
	result.RollbackDetected = input["rollbackDetected"] != ""

	result.TpmEnabled = result.TpmMode == "enabled"
	result.ProdKeyladder = result.Keyladder == "prod" || result.Keyladder == "enabled"

	isCr50 := result.ChipName == "cr50"
	isOT := strings.HasPrefix(input["chipSKU"], "NT")

	if result.RORollback == "" && !isCr50 {
		return Sysinfo{}, errors.New("Did not find RO Rollback info on Ti50")
	}

	rollbackBits, err := findSysinfoRollbackBits(result.RWRollback, isOT)
	if err != nil {
		return Sysinfo{}, err
	}
	result.RWRollbackBits = rollbackBits

	res, err := strconv.ParseUint(input["resetCount"], 10, 32)
	if err != nil {
		return result, errors.Wrap(err, "invalid resetCount")
	}
	result.ResetCount = uint32(res)

	res, err = strconv.ParseUint(input["tpmModeStatus"], 10, 32)
	if err != nil {
		return result, errors.Wrap(err, "invalid tpmModeStatus")
	}
	result.TpmModeStatus = uint32(res)

	res, err = strconv.ParseUint(input["resetFlags"], 16, 32)
	if err != nil {
		return result, errors.Wrap(err, "invalid resetFlags")
	}
	result.OriginalResetFlags = uint32(res)
	if isCr50 {
		result.ResetFlags = uint32(convertCr50ResetFlags(res))
	} else {
		if isOT {
			result.ResetFlags = uint32(convertOTResetFlags(res))
		} else {
			result.ResetFlags = result.OriginalResetFlags
		}
		result.FactoryModeValid = true
		result.InFactoryMode = input["factoryMode"] == sysinfoFactoryMode
	}

	return result, nil
}

// ChipSKU returns the chip sku from sysinfo
func (i *CrOSImage) ChipSKU(ctx context.Context) (ChipSKU, error) {
	sysinfo, err := i.Sysinfo(ctx)
	if err != nil {
		return "", err
	}
	return sysinfo.ChipSKU, nil
}

// EraseFlashInfo runs eraseflashinfo.
func (i *CrOSImage) EraseFlashInfo(ctx context.Context) (string, error) {
	return i.Command(ctx, "eraseflashinfo both")
}

// BIDField is the bid field type
type BIDField uint32

const (
	// UnsetBID is the unset BID value
	UnsetBID BIDField = 0xffffffff
)

// ChipBID contains information about the chip board ID.
type ChipBID struct {
	Type           BIDField
	TypeInv        BIDField
	Flags          BIDField
	IsErased       bool
	FlagsAreErased bool
	TypeIsErased   bool
}

// parseChipBID returns the chip sku from sysinfo
func parseChipBID(output string) (ChipBID, error) {
	result := ChipBID{}
	matches := chipBIDRE.FindStringSubmatch(output)
	if matches == nil {
		return ChipBID{}, errors.Errorf("Unable to find chip board ID %s in bid output %s", chipBIDRE, output)
	}
	res, err := strconv.ParseUint(matches[1], 16, 32)
	if err != nil {
		return ChipBID{}, errors.Wrap(err, "could not parse board ID Type")
	}
	result.Type = BIDField(res)

	res, err = strconv.ParseUint(matches[2], 16, 32)
	if err != nil {
		return ChipBID{}, errors.Wrap(err, "could not parse board ID TypeInv")
	}
	result.TypeInv = BIDField(res)

	res, err = strconv.ParseUint(matches[3], 16, 32)
	if err != nil {
		return ChipBID{}, errors.Wrap(err, "could not parse board ID Flags")
	}
	result.Flags = BIDField(res)

	result.TypeIsErased = (result.Type == UnsetBID) && (result.TypeInv == UnsetBID)
	if !result.TypeIsErased && result.Type != (result.TypeInv^UnsetBID) {
		return ChipBID{}, errors.Errorf("BID Type %x is not the inverted BID Type Inv %x", result.Type, result.TypeInv)
	}
	result.FlagsAreErased = (result.Flags == UnsetBID) && result.TypeIsErased
	result.IsErased = result.TypeIsErased && result.FlagsAreErased

	return result, nil
}

// ChipBID gets the current chip board id.
func (i *CrOSImage) ChipBID(ctx context.Context) (ChipBID, error) {
	out, err := i.Command(ctx, "bid")
	if err != nil {
		return ChipBID{}, err
	}
	return parseChipBID(out)
}

// writeOnceInfoPagesAreErased returns True if all fields in protected info
// pages are erased. This checks the board id and sysinfo in factory mode bit.
// If any other information is added to a write once info page, this check
// should be updated to include it.
func writeOnceInfoPagesAreErased(bid ChipBID, sysinfo Sysinfo, factoryConfig uint64) bool {
	return bid.IsErased && (!sysinfo.FactoryModeValid || sysinfo.InFactoryMode) && (factoryConfig == 0)
}

// WriteOnceInfoPagesAreErased returns True if all fields in the info pages that
// RO protects are erased.
func (i *CrOSImage) WriteOnceInfoPagesAreErased(ctx context.Context) (bool, error) {
	bid, err := i.ChipBID(ctx)
	if err != nil {
		return false, err
	}
	sysinfo, err := i.Sysinfo(ctx)
	if err != nil {
		return false, err
	}
	factoryConfig, err := i.FactoryConfig(ctx)
	if err != nil {
		return false, err
	}
	return writeOnceInfoPagesAreErased(bid, sysinfo, factoryConfig), nil
}

// parseFactoryConfig converts brdprop factory config output into the uint64 value
func parseFactoryConfig(output string) (uint64, error) {
	match := factoryConfigRE.FindStringSubmatch(output)
	if match == nil {
		return 0, errors.Errorf("could not find %s in %s", factoryConfigRE, output)
	}
	config, err := strconv.ParseUint(match[2], 16, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse brdprop factory config value from %s", match[2])
	}
	return config, nil
}

// FactoryConfig gets the numerical value from the "brdprop" GSC command.
func (i *CrOSImage) FactoryConfig(ctx context.Context) (uint64, error) {
	output, err := i.safeCommand(ctx, "brdprop")
	if err != nil {
		return 0, errors.Wrap(err, "failed to run GSC brdprop command")
	}
	return parseFactoryConfig(output)
}

// Cr50APROInfo contains the AP RO verification information
type Cr50APROInfo struct {
	Result          uint64
	Supported       bool
	SupportedReason uint64
	Hash            string
}

// parseCr50APROInfo converts brdprop factory config output into the uint64 value
func parseCr50APROInfo(output string) (Cr50APROInfo, error) {
	match := cr50APROResultRE.FindStringSubmatch(output)
	if match == nil {
		return Cr50APROInfo{}, errors.Errorf("could not find %s in %s", cr50APROResultRE, output)
	}
	result, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil {
		return Cr50APROInfo{}, errors.Wrapf(err, "failed to parse ap_ro_info result from %s", match[2])
	}
	match = cr50APROSupportedRE.FindStringSubmatch(output)
	if match == nil {
		return Cr50APROInfo{}, errors.Errorf("could not find %s in %s", cr50APROSupportedRE, output)
	}
	supported := match[1] == "yes"
	supportedReason, err := strconv.ParseUint(match[2], 10, 64)
	if err != nil {
		return Cr50APROInfo{}, errors.Wrapf(err, "failed to parse ap_ro_info supported from %s", match[2])
	}
	match = cr50APROHashRE.FindStringSubmatch(output)
	var hash string
	if match == nil {
		if supported {
			return Cr50APROInfo{}, errors.Wrap(err, "failed to parse hash when verification is supported")
		}
	} else {
		hash = match[1]
	}
	expected := Cr50APROInfo{}
	expected.Result = result
	expected.Supported = supported
	expected.SupportedReason = supportedReason
	expected.Hash = hash
	return expected, nil
}

// Cr50APROInfo gets the cr50 AP RO hash information
func (i *CrOSImage) Cr50APROInfo(ctx context.Context) (Cr50APROInfo, error) {
	output, err := i.safeCommand(ctx, "ap_ro_info")
	if err != nil {
		return Cr50APROInfo{}, errors.Wrap(err, "failed to parse ap_ro_info output")
	}
	return parseCr50APROInfo(output)
}

// CCDStateStandardVal is a standard value to compare against
type CCDStateStandardVal string

// CCDStateVal contains the converted state and the original string from the ccdstate output
type CCDStateVal struct {
	State CCDStateStandardVal
	Raw   string
}

// String creates a string with the converted state information and the raw value
func (v CCDStateVal) String() string {
	return fmt.Sprintf("%s (%s)", v.State, v.Raw)
}

// Standard ccdstate strings, so tests don't need to know the difference between
// the different types of on/off values
const (
	// CCDStateOn is used for on, enabled, asserted, and connected states
	CCDStateOn CCDStateStandardVal = "on"
	// CCDStateOff is used for off, disabled, deasserted, and disconnected states
	CCDStateOff CCDStateStandardVal = "off"
	// CCDStateNA used if GSC doesn't print the field in the ccdstate output
	CCDStateNA CCDStateStandardVal = "n/a"
	// CCDStateUndetectable means GSC can't detect the servo state
	CCDStateUndetectable CCDStateStandardVal = "undetectable"
	// CCDStateDebouncing GSC prints this value for a field when it's still debouncing the sate
	CCDStateDebouncing CCDStateStandardVal = "debouncing"
)

// CCDStateInfo contains ccdstate command information
type CCDStateInfo struct {
	DeepSleepDisabled CCDStateVal
	AP                CCDStateVal
	APUART            CCDStateVal
	PCR0              string
	EC                CCDStateVal
	Servo             CCDStateVal
	Rdd               CCDStateVal
	KeepAlive         CCDStateVal
	CCDModeSignal     CCDStateVal
	StateFlags        string
	CCDBlocked        string
	IsDebouncing      bool
}

// findCCDStateMap finds all of the field value pairs in the ccdstate output and makes it into a map
func findCCDStateMap(s string) (map[string]string, error) {
	var out map[string]string

	matches := ccdStateRE.FindAllStringSubmatch(s, -1)
	if matches == nil {
		return nil, errors.New("failed to parse ccdstate output")
	}

	// Map regex result to typed result
	out = make(map[string]string)
	for i := 0; i < len(matches); i++ {
		field := matches[i][1]
		val := matches[i][2]
		out[field] = val
	}

	return out, nil
}

// parseCCDStateVal converts the ccdstate field string into a CCDStateVal
func parseCCDStateVal(output string) CCDStateVal {
	val := CCDStateVal{}
	val.Raw = output
	if output == "" {
		val.State = CCDStateNA
		return val
	}

	if ccdStateOffRE.MatchString(output) {
		val.State = CCDStateOff
		return val
	}
	if ccdStateOnRE.MatchString(output) {
		val.State = CCDStateOn
		return val
	}
	if strings.Contains(output, "debouncing") {
		val.State = CCDStateDebouncing
		return val
	}
	if strings.Contains(output, "undetectable") {
		val.State = CCDStateUndetectable
		return val
	}
	return val
}

// parseCCDStateInfo converts the ccdstate output into a CCDStateInfo struct
func parseCCDStateInfo(output string) (CCDStateInfo, error) {
	ccdstateMap, err := findCCDStateMap(output)
	if err != nil {
		return CCDStateInfo{}, errors.Errorf("could not generate ccdstate map from %s", output)
	}
	ccdstate := CCDStateInfo{}
	ccdstate.DeepSleepDisabled = parseCCDStateVal(ccdstateMap["DS Dis"])
	ccdstate.AP = parseCCDStateVal(ccdstateMap["AP"])
	ccdstate.APUART = parseCCDStateVal(ccdstateMap["AP UART"])
	ccdstate.PCR0 = ccdstateMap["pcr0"]
	ccdstate.EC = parseCCDStateVal(ccdstateMap["EC"])
	ccdstate.Servo = parseCCDStateVal(ccdstateMap["Servo"])
	ccdstate.Rdd = parseCCDStateVal(ccdstateMap["Rdd"])
	ccdstate.KeepAlive = parseCCDStateVal(ccdstateMap["KeepAlive"])
	ccdstate.CCDModeSignal = parseCCDStateVal(ccdstateMap["CCD_MODE"])
	ccdstate.StateFlags = ccdstateMap["State flags"]
	ccdstate.CCDBlocked = ccdstateMap["CCD ports blocked"]
	ccdstate.IsDebouncing = strings.Contains(output, "debouncing")
	if ccdstate.CCDBlocked == "" {
		ccdstate.CCDBlocked = ccdstateMap["CCD Ports blocked"]
	}
	return ccdstate, nil
}

// CCDStateInfo gets the GSC ccdstate info
func (i *CrOSImage) CCDStateInfo(ctx context.Context) (CCDStateInfo, error) {
	output, err := i.Command(ctx, "ccdstate")
	if err != nil {
		return CCDStateInfo{}, errors.Wrap(err, "failed to parse ccdstate output")
	}
	return parseCCDStateInfo(output)
}

// WaitForStableCCDState waits until GSC isn't debouncing any of the ccdstate output
func (i *CrOSImage) WaitForStableCCDState(ctx context.Context) error {
	ccdstate := CCDStateInfo{}
	for j := 0; j < 5; j++ {
		testing.Sleep(ctx, 3*time.Second) // GoBigSleepLint: wait for GSC to poll device state signals
		ccdstate, err := i.CCDStateInfo(ctx)
		if err != nil {
			return err
		}
		if !ccdstate.IsDebouncing {
			return nil
		}
	}
	return errors.Errorf("GSC is still debouncing ccdstate: %+v", ccdstate)
}

// WipeTpmWithCCDOpen uses ccd open to wipe the TPM
func (i *CrOSImage) WipeTpmWithCCDOpen(ctx context.Context) error {
	err := i.CCDOpen(ctx)
	if err != nil {
		return err
	}
	err = i.SetCCDCapability(ctx, OpenNoTPMWipe, CapIfOpened)
	if err != nil {
		return err
	}
	err = i.CCDLock(ctx)
	if err != nil {
		return err
	}
	err = i.CCDOpen(ctx)
	if err != nil {
		return err
	}
	return i.CCDResetFactory(ctx)
}

// StrongboxState is the strongbox state
type StrongboxState uint32

const (
	// BoardConfigStrongboxEnabled is the mask for the board config StrongboxEnabled bit
	BoardConfigStrongboxEnabled = (1 << 16)
	// BoardConfigStrongboxDisabled is the mask for the board config StrongboxDisabled bit
	BoardConfigStrongboxDisabled = (1 << 17)

	// StrongboxUnset is the state when strongbox is not enabled or disabled
	StrongboxUnset StrongboxState = 0
	// StrongboxEnabled is the state when strongbox is enabled
	StrongboxEnabled StrongboxState = 1
	// StrongboxDisabled is the state when strongbox is disabled
	StrongboxDisabled StrongboxState = 2
)

// parseBoardConfig converts brdprop board cfg output into the uint64 value
func parseBoardConfig(output string) (uint32, error) {
	match := boardConfigRE.FindStringSubmatch(output)
	if match == nil {
		return 0, errors.Errorf("could not find %s in %s", boardConfigRE, output)
	}
	config, err := strconv.ParseUint(match[1], 16, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse brdprop board cfg config value from %s", match[1])
	}
	return uint32(config), nil
}

// BoardConfig gets the numerical board cfg value from the "brdprop" GSC command.
func (i *CrOSImage) BoardConfig(ctx context.Context) (uint32, error) {
	output, err := i.safeCommand(ctx, "brdprop")
	if err != nil {
		return 0, errors.Wrap(err, "failed to run GSC brdprop command")
	}
	return parseBoardConfig(output)
}

// parseStrongboxState parses the strongbox state from the board config value
func parseStrongboxState(boardConfig uint32) (StrongboxState, error) {
	enabled := (boardConfig & BoardConfigStrongboxEnabled) != 0
	disabled := (boardConfig & BoardConfigStrongboxDisabled) != 0
	if !enabled && !disabled {
		return StrongboxUnset, nil
	}
	if enabled && disabled {
		return StrongboxUnset, errors.New("Strongbox enabled and disabled bit set")
	}
	if enabled {
		return StrongboxEnabled, nil
	}
	return StrongboxDisabled, nil
}

// StrongboxState returns the board config strongbox state
func (i *CrOSImage) StrongboxState(ctx context.Context) (StrongboxState, error) {
	boardConfig, err := i.BoardConfig(ctx)
	if err != nil {
		return StrongboxUnset, errors.Wrap(err, "failed to get board config")
	}
	return parseStrongboxState(boardConfig)
}
