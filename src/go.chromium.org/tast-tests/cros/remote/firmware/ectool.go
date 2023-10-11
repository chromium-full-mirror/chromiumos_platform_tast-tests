// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements functions to interact with the DUT's embedded controller (EC)
// via the host command `ectool`.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// ECToolName specifies which of the many Chromium EC based MCUs ectool will
// be communicated with.
// Some options are cros_ec, cros_fp, cros_pd, cros_scp, and cros_ish.
type ECToolName string

const (
	// ECToolNameMain selects the main EC using cros_ec.
	ECToolNameMain ECToolName = "cros_ec"
	// ECToolNameFingerprint selects the FPMCU using cros_fp.
	ECToolNameFingerprint ECToolName = "cros_fp"
)

// ECTool allows for interaction with the host command `ectool`.
type ECTool struct {
	dut  *dut.DUT
	name ECToolName
}

// NewECTool creates an ECTool.
func NewECTool(d *dut.DUT, name ECToolName) *ECTool {
	return &ECTool{dut: d, name: name}
}

// Regexps to capture values outputted by ectool version.
var (
	reFirmwareCopy  = regexp.MustCompile(`Firmware copy:\s*(RO|RW)`)
	reROVersion     = regexp.MustCompile(`RO version:\s*(\S+)\s`)
	reRWVersion     = regexp.MustCompile(`RW version:\s*(\S+)\s`)
	reECHash        = regexp.MustCompile(`status:\s*(\S+)\s*type:\s*(\S+)\s*offset:\s*(\S+)\s*size:\s*(\S+)\s*hash:\s*(\S+)\s*`)
	reTabletModeAng = regexp.MustCompile(`tablet_mode_angle=(\d+) hys=(\d+)`)
	reFlashSize     = regexp.MustCompile(`FlashSize\s*(\d+)`)
	reChipInfo      = regexp.MustCompile(`vendor:\s*(\S+)\s*name:\s*(\S+)\s*revision:\s*(\S*)`)
	reI2CLookup     = regexp.MustCompile(`Bus: I2C; Port: (\S+); Address: (\S+)`)
	reTempInfo      = regexp.MustCompile(`(\d+):\s+\d+\s+(\S+)`)
	reSensorTemp    = regexp.MustCompile(`\S+\s+([0-9]+) K`)
	reBatteryInfo   = regexp.MustCompile(`\s*(\S[^\r\n]+)(:|\s)\s+(\S[^\r\n]+)`)
)

// Command return the prebuilt ssh Command with options and args applied.
func (ec *ECTool) Command(ctx context.Context, args ...string) *ssh.Cmd {
	args = append([]string{"--name=" + string(ec.name)}, args...)
	return ec.dut.Conn().CommandContext(ctx, "ectool", args...)
}

// Version returns the EC version of the active firmware.
func (ec *ECTool) Version(ctx context.Context) (string, error) {
	output, err := ec.Command(ctx, "version").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "running 'ectool version' on DUT")
	}

	// Parse output to determine whether RO or RW is the active firmware.
	match := reFirmwareCopy.FindSubmatch(output)
	if len(match) == 0 {
		return "", errors.Errorf("did not find firmware copy in 'ectool version' output: %s", output)
	}
	var reActiveFWVersion *regexp.Regexp
	switch string(match[1]) {
	case "RO":
		reActiveFWVersion = reROVersion
	case "RW":
		reActiveFWVersion = reRWVersion
	default:
		return "", errors.Errorf("unexpected match from reFirmwareCopy: got %s; want RO or RW", match[1])
	}

	// Parse either RO version line or RW version line, depending on which is active, to find the active firmware version.
	match = reActiveFWVersion.FindSubmatch(output)
	if len(match) == 0 {
		return "", errors.Errorf("failed to match regexp %s in ectool version output: %s", reActiveFWVersion, output)
	}
	return string(match[1]), nil
}

type hashinfo struct {
	Status string
	Type   string
	Offset string
	Size   string
	Hash   string
}

// Hash returns the EC hash of the active firmware.
func (ec *ECTool) Hash(ctx context.Context, args ...string) (*hashinfo, error) {
	cmdAndArgs := append([]string{string("echash")}, args...)
	outBytes, err := ec.Command(ctx, cmdAndArgs...).Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "running 'ectool echash' on DUT")
	}

	out := string(outBytes)
	// Parse output to determine whether RO or RW is the active firmware.
	match := reECHash.FindStringSubmatch(out)
	if len(match) != 6 {
		return nil, errors.Errorf("could not parse 'ectool echash' output: %s", out)
	}

	info := hashinfo{
		Status: match[1],
		Type:   match[2],
		Offset: match[3],
		Size:   match[4],
		Hash:   match[5],
	}

	return &info, nil
}

// BatteryCutoff runs the ectool batterycutoff command.
func (ec *ECTool) BatteryCutoff(ctx context.Context) error {
	if err := ec.Command(ctx, "batterycutoff").Start(); err != nil {
		return errors.Wrap(err, "running 'ectool batterycutoff' on DUT")
	}
	return nil
}

// SaveTabletModeAngles runs 'ectool motionsense tablet_mode_angle' to save the current angles for tablet mode.
func (ec *ECTool) SaveTabletModeAngles(ctx context.Context) (string, string, error) {
	out, err := ec.Command(ctx, "motionsense", "tablet_mode_angle").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", "", errors.Wrap(err, "running 'ectool motionsense tablet_mode_angle' on DUT")
	}
	matches := reTabletModeAng.FindStringSubmatch(string(out))
	if len(matches) != 3 {
		return "", "", errors.Errorf("unable to retrieve tablet mode angles from 'ectool motionsense tablet_mode_angle' output: %s", out)
	}
	return matches[1], matches[2], nil
}

// ForceTabletModeAngle emulates rotation angles to change DUT's tablet mode setting.
func (ec *ECTool) ForceTabletModeAngle(ctx context.Context, tabletModeAngle, hys string) error {
	if err := ec.Command(ctx, "motionsense", "tablet_mode_angle", tabletModeAngle, hys).Start(); err != nil {
		return errors.Wrap(err, "failed to set tablet_mode_angle to 0")
	}
	return nil
}

// FlashSize retrieves the flashsize in bytes from the ectool flashinfo command.
func (ec *ECTool) FlashSize(ctx context.Context) (int, error) {
	out, err := ec.Command(ctx, "flashinfo").Output()
	if err != nil {
		return 0, errors.Wrap(err, "failed to get flashinfo output")
	}
	match := reFlashSize.FindSubmatch(out)
	if match == nil {
		return 0, errors.Errorf("failed to match regexp %s in ectool flashinfo output: %s", reFlashSize, string(out))
	}
	return strconv.Atoi(string(match[1]))
}

type chipinfo struct {
	Vendor   string
	Name     string
	Revision string
}

// ChipInfo retrieves the chipinfo from the ectool chipinfo command.
func (ec *ECTool) ChipInfo(ctx context.Context) (*chipinfo, error) {
	out, err := ec.Command(ctx, "chipinfo").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get chipinfo output")
	}
	match := reChipInfo.FindSubmatch(out)
	if match == nil {
		return nil, errors.Errorf("failed to match regexp %s in ectool chipinfo output: %s", reChipInfo, string(out))
	}
	info := chipinfo{
		Vendor:   string(match[1]),
		Name:     string(match[2]),
		Revision: string(match[3]),
	}
	return &info, nil
}

// GpioName type holds commands for 'ectool gpioget'.
type GpioName string

const (
	// ECCbiWp for the 'ectool gpioget ec_cbi_wp' cmd.
	ECCbiWp GpioName = "ec_cbi_wp"
	// ENPP3300POGO for the 'ectool gpioget EN_PP3300_POGO' cmd.
	ENPP3300POGO GpioName = "EN_PP3300_POGO"
	// ENBASE for the 'ectool gpioget EN_BASE' cmd.
	ENBASE GpioName = "EN_BASE"
	// PP3300DXBASE for the 'ectool gpioget PP3300_DX_BASE' cmd.
	PP3300DXBASE GpioName = "PP3300_DX_BASE"
	// ECEDPBLEN for the 'ectool gpioget EC_EDP_BL_EN' cmd.
	ECEDPBLEN GpioName = "EC_EDP_BL_EN"
	// BLDISABLEL for the 'ectool gpioget BL_DISABLE_L' cmd.
	BLDISABLEL GpioName = "BL_DISABLE_L"
	// ENBLOD for the 'ectool gpioget EN_BL_OD' cmd.
	ENBLOD GpioName = "EN_BL_OD"
	// ENABLEBACKLIGHT for the 'ectool gpioget ENABLE_BACKLIGHT' cmd.
	ENABLEBACKLIGHT GpioName = "ENABLE_BACKLIGHT"
	// ENABLEBACKLIGHTL for the 'ectool gpioget ENABLE_BACKLIGHT_L' cmd.
	ENABLEBACKLIGHTL GpioName = "ENABLE_BACKLIGHT_L"
	// ECBLENOD for the 'ectool gpioget EC_BL_EN_OD' cmd.
	ECBLENOD GpioName = "EC_BL_EN_OD"
	// BASEPWREN for the 'ectool gpioget BASE_PWR_EN' cmd.
	BASEPWREN GpioName = "BASE_PWR_EN"
)

// FindGPIOs iterates through a passed in list of gpios, and returns the state of the gpios that exist.
func (ec *ECTool) FindGPIOs(ctx context.Context, gpios []GpioName) (map[GpioName]string, error) {
	// Create a local map to save the gpios found and their current values from the passed in list.
	results := make(map[GpioName]string)
	for _, name := range gpios {
		reFoundGpio := regexp.MustCompile(fmt.Sprintf(`GPIO\s*%s\s*=\s*(\d+)`, string(name)))
		// ectool gpioget fails with EC result 2, and exit code 1 when the gpio is not found.
		out, err := ec.Command(ctx, "gpioget", string(name)).CombinedOutput()
		if err == nil {
			match := reFoundGpio.FindStringSubmatch(string(out))
			if match == nil {
				return nil, errors.Errorf("failed to find gpio %q in %s", name, strings.TrimSpace(string(out)))
			}
			gpioVal := match[1]
			results[name] = gpioVal
		} else if !strings.HasPrefix(string(out), "EC result") {
			return nil, errors.Wrapf(err, "ectool failed: %s", string(out))
		}
	}
	return results, nil
}

// I2CLookupInfo is a way to access the port and address of the i2c.
type I2CLookupInfo struct {
	Port    int
	Address int
}

// I2CLookup runs ectool locatechip 0 0 to get Port and Address for I2C.
func (ec *ECTool) I2CLookup(ctx context.Context) (*I2CLookupInfo, error) {
	out, err := ec.Command(ctx, "locatechip", "0", "0").Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "running 'ectool locatechip 0 0' on DUT")
	}
	match := reI2CLookup.FindSubmatch(out)
	if match == nil || len(match) == 0 {
		return nil, errors.Wrapf(err, "lookup for I2C failed, got %q", string(out))
	}

	parsedPort, err := strconv.ParseInt(string(match[1]), 0, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse port val %q as int", string(match[1]))
	}
	parsedAddr, err := strconv.ParseInt(string(match[2]), 0, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse addr val %q as int", string(match[2]))
	}

	return &I2CLookupInfo{Port: int(parsedPort), Address: int(parsedAddr)}, nil
}

// I2CCmd type holds commands for interacting with i2c using the ectool.
type I2CCmd string

const (
	// I2CRead for the 'ectool i2cread <8 | 16> <port> <addr8> <offset>' cmd.
	I2CRead I2CCmd = "i2cread"
	// I2CSpeed for the 'ectool i2cspeed <port> [speed in kHz]' cmd.
	I2CSpeed I2CCmd = "i2cspeed"
	// I2CWrite for the 'ectool i2cwrite <8 | 16> <port> <addr8> <offset> <data>' cmd.
	I2CWrite I2CCmd = "i2cwrite"
	// I2Cxfer for the 'ectool i2cxfer <port> <addr7> <read_count> [bytes...]' cmd.
	I2Cxfer I2CCmd = "i2cxfer"
)

// I2C runs the 'ectool i2c*' with provided command and args.
func (ec *ECTool) I2C(ctx context.Context, cmd I2CCmd, args ...string) (string, error) {
	cmdAndArgs := append([]string{string(cmd)}, args...)
	out, err := ec.Command(ctx, cmdAndArgs...).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "running 'ectool %s' on DUT with args %v", string(cmd), args)
	}
	return string(out), nil
}

// CBICmd type holds commands for interacting with cbi using the ectool.
type CBICmd string

const (
	// CBIGet for the 'ectool cbi get <tag> [get_flag]'.
	CBIGet CBICmd = "get"
	// CBISet for the 'ectool cbi set <tag> <value/string> <size> [set_flag]'.
	CBISet CBICmd = "set"
	// CBIRemove for the 'ectool cbi remove <tag> [set_flag]'.
	CBIRemove CBICmd = "remove"
)

// CBI runs the 'ectool cbi' with provided command and args.
func (ec *ECTool) CBI(ctx context.Context, cmd CBICmd, args ...string) (string, error) {
	cmdAndArgs := []string{"cbi", string(cmd)}
	cmdAndArgs = append(cmdAndArgs, args...)
	testing.ContextLogf(ctx, "Running cmd: 'ectool %s'", strings.Join(cmdAndArgs, " "))
	out, err := ec.Command(ctx, cmdAndArgs...).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "running 'ectool %s' on DUT with args %v, got: %v", string(cmd), args, string(out))
	}
	return string(out), nil
}

// CBIBinCmd type holds commands for interacting with cbi using the ectool.
type CBIBinCmd string

const (
	// CBIBinRead for the 'ectool cbibin read <file> <size>'.
	CBIBinRead CBIBinCmd = "read"
	// CBIBinWrite for the 'ectool cbibin write <file> <size>'.
	CBIBinWrite CBIBinCmd = "write"
)

// CBIBin runs the 'ectool cbibin' with provided command and args.
func (ec *ECTool) CBIBin(ctx context.Context, cmd CBIBinCmd, args ...string) (string, error) {
	cmdAndArgs := []string{"cbibin", string(cmd)}
	cmdAndArgs = append(cmdAndArgs, args...)
	testing.ContextLogf(ctx, "Running cmd: 'ectool %s'", strings.Join(cmdAndArgs, " "))
	out, err := ec.Command(ctx, cmdAndArgs...).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "running 'ectool %s' on DUT with args %v, got: %v", string(cmd), args, string(out))
	}
	return string(out), nil
}

// BCFGCmd type holds commands for interacting with BCFG using the ectool.
type BCFGCmd string

const (
	// BCFGGet for the 'ectool bcfg get'.
	BCFGGet BCFGCmd = "get"
	// BCFGSet for the 'ectool bcfg set'.
	BCFGSet BCFGCmd = "set"
)

// BCFG runs the 'ectool bcfg' with provided command and args.
func (ec *ECTool) BCFG(ctx context.Context, cmd BCFGCmd, args ...string) (string, error) {
	cmdAndArgs := []string{"bcfg", string(cmd)}
	cmdAndArgs = append(cmdAndArgs, args...)
	testing.ContextLogf(ctx, "Running cmd: 'ectool %s'", strings.Join(cmdAndArgs, " "))
	out, err := ec.Command(ctx, cmdAndArgs...).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "running 'ectool %s' on DUT with args %v, got: %v", strings.Join(cmdAndArgs, " "), args, string(out))
	}
	return string(out), nil
}

// TempsInfo parses `tempsinfo all` from ectool, returns map of the sensor name to sensor id.
// Note, this is opposite of what tempsinfo displays to make it easier to search by sensor name.
func (ec *ECTool) TempsInfo(ctx context.Context) (map[string]int, error) {
	out, err := ec.Command(ctx, "tempsinfo", "all").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tempsinfo output")
	}

	lines := strings.Split(string(out), "\n")
	infoMap := make(map[string]int, len(lines))
	for _, line := range lines {
		if match := reTempInfo.FindStringSubmatch(line); match != nil {
			idx, _ := strconv.Atoi(match[1])
			infoMap[match[2]] = idx
		}
	}

	return infoMap, nil
}

// Temps gets `temps <id>` from ectool which has temp for sensor_id.
func (ec *ECTool) Temps(ctx context.Context, id int) (int, error) {
	out, err := ec.Command(ctx, "temps", strconv.Itoa(id)).Output()
	if err != nil {
		return -1, errors.Wrap(err, "failed to get temp output")
	}

	if match := reSensorTemp.FindStringSubmatch(string(out)); match != nil {
		temp, _ := strconv.Atoi(match[1])
		return temp, nil
	}
	return -1, errors.Errorf("failed to parse sensor temp from output %v", string(out))
}

// Battery returns a parsed map of the Battery # to various info associated with that battery.
func (ec *ECTool) Battery(ctx context.Context) (map[string]string, error) {
	/*
		Output looks like:
			Battery 0 info:
				OEM name:               Sunwoda
				Model number:           L20D3PG2
				Chemistry   :           LiP
				Serial number:          55E6
				Design capacity:        3735 mAh
				Last full charge:       3803 mAh
				Design output voltage   11250 mV     <-- Note: Some of these don't have colons/separators,
				Cycle count             18                     which makes it tricky to parse.
				Present voltage         12668 mV
				Present current         0 mA
				Remaining capacity      3802 mAh
				Desired voltage         0 mV
				Desired current         0 mA
				Flags                   0x0b AC_PRESENT BATT_PRESENT CHARGING
			...

		TODO(tij@): Update to support situations with more than one battery eg.
			"Battery n info:..."
	*/

	out, err := ec.Command(ctx, "battery").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get battery output from ectool")
	}

	battMap := make(map[string]string)

	lines := strings.Split(string(out), "\n")

	for _, line := range lines[1:] {
		if match := reBatteryInfo.FindStringSubmatch(line); match != nil {
			key := strings.TrimSpace(strings.Replace(match[1], ":", "", 1))
			val := strings.TrimSpace(match[3])
			battMap[key] = val
		}
	}

	return battMap, nil
}

// Lightbar runs the lightbar commands from ectool.
// Example inputs: `lightbar`, `lightbar on|off|init` and `lightbar 4 255 255 255`.
func (ec *ECTool) Lightbar(ctx context.Context, args ...string) (string, error) {
	out, err := ec.Command(ctx, append([]string{"lightbar"}, args...)...).Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to run lightbar cmd in ectool")
	}

	return string(out), nil
}

// SetFanRPM uses `pwmsetfanrpm` to set fanspeed to given rpm.
func (ec *ECTool) SetFanRPM(ctx context.Context, rpm int) error {
	out, err := ec.Command(ctx, "pwmsetfanrpm", strconv.Itoa(rpm)).Output()
	if err != nil {
		return errors.Wrap(err, "failed to run pwmsetfanrpm cmd in ectool")
	}

	if !strings.Contains(string(out), "Fan target RPM set") {
		return errors.Errorf("failed to set fan rpm, got output: %v", string(out))
	}

	return nil
}

// GetFanRPM uses `pwmgetfanrpm` to get fanspeed for fan(s).
func (ec *ECTool) GetFanRPM(ctx context.Context) (map[int]int, error) {
	reMultipleFanRPM := regexp.MustCompile(`Fan\s+(\d+)\s+RPM:\s+(\d+)`)
	reSingleFanRPM := regexp.MustCompile(`Current fan RPM:\s+(\d+)`)

	fanSpeeds := make(map[int]int)

	out, err := ec.Command(ctx, "pwmgetfanrpm").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to run pwmgetfanrpm cmd in ectool")
	}

	if match := reSingleFanRPM.FindStringSubmatch(string(out)); match != nil {
		fanRPM, _ := strconv.Atoi(match[2])
		fanSpeeds[0] = fanRPM
		// If output only shown for current fan, return immediately since no more fan speeds to parse.
		return fanSpeeds, nil
	}

	/*
		If output in format:
			Fan <id> RPM: <speed>
			...
		read all speeds.
	*/
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if match := reMultipleFanRPM.FindStringSubmatch(line); match != nil {
			fanIdx, _ := strconv.Atoi(match[1])
			fanRPM, _ := strconv.Atoi(match[2])
			fanSpeeds[fanIdx] = fanRPM
		}
	}

	return fanSpeeds, nil
}

// AutoFanCtrl uses `autofanctrl` to reset fanspeed to automatic setting.
func (ec *ECTool) AutoFanCtrl(ctx context.Context) error {
	if err := ec.Command(ctx, "autofanctrl").Run(); err != nil {
		return errors.Wrap(err, "failed to set fan speed to auto")
	}

	return nil
}

// FlashProtect runs the 'ectool flashprotect'.
func (ec *ECTool) FlashProtect(ctx context.Context, args ...string) (string, error) {
	cmdAndArgs := []string{"flashprotect"}
	cmdAndArgs = append(cmdAndArgs, args...)
	testing.ContextLogf(ctx, "Running cmd: 'ectool %s'", strings.Join(cmdAndArgs, " "))
	out, err := ec.Command(ctx, cmdAndArgs...).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "running 'ectool %s' on DUT, got: %v", strings.Join(cmdAndArgs, " "), string(out))
	}
	return string(out), nil
}

// ECToolBaseState specifies the base state.
type ECToolBaseState string

const (
	// BaseAttach force base state to attached.
	BaseAttach = "attach"
	// BaseDetach force base state to detached.
	BaseDetach = "detach"
	// BaseAuto remove the force base state, and EC will determine the base state with physical setup.
	BaseAuto = "reset"
)

// SetBaseState runs the `ectool basestate` command to set the status of tablet mode.
func (ec *ECTool) SetBaseState(ctx context.Context, state ECToolBaseState) error {
	_, err := ec.Command(ctx, "basestate", string(state)).Output(ssh.DumpLogOnError)
	return err
}

// GetECUptime returns the current ec up time in seconds as a float.
func (ec *ECTool) GetECUptime(ctx context.Context) (float64, error) {
	out, err := ec.Command(ctx, "uptimeinfo").Output(ssh.DumpLogOnError)
	if err != nil {
		return -1, errors.Wrapf(err, "running 'ectool uptimeinfo' on DUT, got: %v", string(out))
	}

	outstr := string(out)
	// The ectool uptimeinfo out looks like `EC uptime: 240.381 seconds`.
	match := regexp.MustCompile(`EC uptime: (\d+\.\d+) seconds`).FindStringSubmatch(outstr)
	if match == nil {
		return -1, errors.Wrapf(err, "failed to parse ec uptime, got %s", outstr)
	}

	uptime, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to parse ec uptime to int, got %s", match[1])
	}

	return uptime, nil
}

// GetPanicInfo returns the panic info from the EC as a string
func (ec *ECTool) GetPanicInfo(ctx context.Context) (string, error) {
	out, err := ec.Command(ctx, "panicinfo").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "running 'ectool panicinfo' on DUT, got: %v", string(out))
	}

	outstr := string(out)

	return outstr, nil
}

// GetAPResetCount returns the number of AP resets since EC boot.
func (ec *ECTool) GetAPResetCount(ctx context.Context) (int, error) {
	out, err := ec.Command(ctx, "uptimeinfo").Output(ssh.DumpLogOnError)
	if err != nil {
		return -1, errors.Wrapf(err, "running 'ectool uptimeinfo' on DUT, got: %v", string(out))
	}

	outstr := string(out)
	// Find AP reset string like `AP resets since EC boot: 3`.
	match := regexp.MustCompile(`AP resets since EC boot: (\d+)`).FindStringSubmatch(outstr)
	if match == nil {
		return -1, errors.Wrapf(err, "failed to parse ec uptime, got %s", outstr)
	}

	apResetCount, err := strconv.Atoi(match[1])
	if err != nil {
		return -1, errors.Wrapf(err, "failed to parse ec uptime to int, got %s", match[1])
	}

	return apResetCount, nil
}
