// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tklauser/go-sysconf"

	mfwd "go.chromium.org/chromiumos/modemfwd"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/timing"
)

const verboseShillLogLevel = -3
const verboseShillLogScopes = "cellular+modem+device+dbus+manager"

var (
	deviceVariant = ""
)

// EnsureUptime ensures that the system has been up for at least the specified amount of time before returning.
func EnsureUptime(ctx context.Context, duration time.Duration) error {
	uptimeStr, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return errors.Wrap(err, "failed to read system uptime")
	}
	uptimeFloat, err := strconv.ParseFloat(strings.Fields(string(uptimeStr))[0], 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse system uptime %q", string(uptimeStr))
	}
	uptime := time.Duration(uptimeFloat) * time.Second
	if uptime < duration {
		testing.ContextLogf(ctx, "waiting %s uptime before starting test, current uptime: %s", duration, uptime)
		// GoBigSleepLint - wait for all daemons to stabilize before starting the test.
		if err := testing.Sleep(ctx, duration-uptime); err != nil {
			return errors.Wrap(err, "failed to wait for system uptime")
		}
	}
	return nil
}

// EnsureDaemonUptime ensures that daemon has been up for at least the specified amount of time before returning.
func EnsureDaemonUptime(ctx context.Context, job string, duration time.Duration) error {
	if !upstart.JobExists(ctx, job) {
		return nil
	}
	_, _, pid, err := upstart.JobStatus(ctx, job)
	if err != nil {
		return errors.Wrapf(err, "failed to run upstart.JobStatus for %q", job)
	}
	if pid == 0 {
		return nil
	}
	ticksPerSecond, err := sysconf.Sysconf(sysconf.SC_CLK_TCK)
	if err != nil {
		return err
	}
	// Start time relative to boot time is found in the stat file.
	statFilename := fmt.Sprintf("/proc/%d/stat", pid)
	buff, err := os.ReadFile(statFilename)
	if err != nil {
		return err
	}
	statParts := strings.Split(string(buff), " ")
	// 22nd entry in stat corresponds to start time which is the time the process
	// is started after system boot. It is expressed in clock ticks.
	startTimeTicks, err := strconv.ParseInt(statParts[21], 10, 64)
	if err != nil {
		return err
	}
	startTimeSeconds := startTimeTicks / ticksPerSecond
	endTime := time.Duration(startTimeSeconds)*time.Second + duration
	uptimeStr, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return errors.Wrap(err, "failed to read system uptime")
	}
	uptimeFloat, err := strconv.ParseFloat(strings.Fields(string(uptimeStr))[0], 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse system uptime %q", string(uptimeStr))
	}
	uptime := time.Duration(uptimeFloat) * time.Second
	if uptime < endTime {
		testing.ContextLogf(ctx, "waiting for %s before starting test, current uptime: %s", (endTime - uptime), uptime)
		// GoBigSleepLint - wait for all daemons to stabilize before starting the test.
		if err := testing.Sleep(ctx, endTime-uptime); err != nil {
			return errors.Wrap(err, "failed to wait for system uptime")
		}
	}
	return nil
}

// GetDeviceVariant gets the variant of the device using cros config.
func GetDeviceVariant(ctx context.Context) (string, error) {
	if deviceVariant != "" {
		return deviceVariant, nil
	}
	tempDutVariant, err := crosconfig.Get(ctx, "/modem", "firmware-variant")
	if crosconfig.IsNotFound(err) {
		return "", errors.Wrap(err, "firmware-variant doesn't exist")
	} else if err != nil {
		return "", errors.Wrap(err, "failed to execute cros_config")
	}
	deviceVariant = tempDutVariant
	return deviceVariant, nil
}

// GetBoard gets the board name of the DUT.
func GetBoard(ctx context.Context) (string, error) {
	device, err := getDevice(ctx)
	if err != nil {
		return "", err
	}
	return device.Board, nil
}

func getDevice(ctx context.Context) (cellularconst.DeviceInfo, error) {
	dutVariant, err := GetDeviceVariant(ctx)
	if err != nil {
		return cellularconst.DeviceInfo{}, err
	}
	device, ok := cellularconst.KnownVariants[dutVariant]
	if !ok {
		return cellularconst.DeviceInfo{}, errors.Errorf("variant %q is not in |knownVariants|", dutVariant)
	}
	return device, nil
}

// GetModel gets the model of the device using cros config.
func GetModel(ctx context.Context) (string, error) {
	tempModel, err := crosconfig.Get(ctx, "/", "name")
	if crosconfig.IsNotFound(err) {
		return "", errors.Wrap(err, "name property doesn't exist")
	} else if err != nil {
		return "", errors.Wrap(err, "failed to execute cros_config")
	}
	return tempModel, nil
}

// GetModemType gets DUT's modem type.
func GetModemType(ctx context.Context) (cellularconst.ModemType, error) {
	device, err := getDevice(ctx)
	if err != nil {
		return 0, err
	}
	return device.Modem, nil
}

// GetModemTypeFromManifestDevice converts a firmware manifest device entry into ModemType.
func GetModemTypeFromManifestDevice(device *mfwd.Device) (cellularconst.ModemType, error) {
	deviceID := device.DeviceId
	if deviceID == "usb:2cb7:0007" {
		return cellularconst.ModemTypeL850, nil
	} else if deviceID == "pci:14c3:4d75 (External)" {
		// The device ID is shared between the FM350 and RW350, so we must use the variant
		// to distinguish them
		fm350Suffix := "_" + strings.ToLower(cellularconst.ModemTypeFM350.String())
		rw350Suffix := "_" + strings.ToLower(cellularconst.ModemTypeRW350.String())
		if strings.HasSuffix(device.Variant, fm350Suffix) {
			return cellularconst.ModemTypeFM350, nil
		} else if strings.HasSuffix(device.Variant, rw350Suffix) {
			return cellularconst.ModemTypeRW350, nil
		} else {
			return cellularconst.ModemTypeUnknown, errors.Errorf("cannot determine modem type for device ID %q with variant %q", deviceID, device.Variant)
		}
	} else if deviceID == "usb:2cb7:01a0" {
		return cellularconst.ModemTypeNL668, nil
	} else if deviceID == "usb:2cb7:01a2" {
		return cellularconst.ModemTypeFM101, nil
	} else if deviceID == "usb:2c7c:0128" {
		return cellularconst.ModemTypeEM060, nil
	} else if deviceID == "usb:3731:0100" {
		return cellularconst.ModemTypeLCUK54, nil
	} else if deviceID == "usb:33f8:01a2" {
		return cellularconst.ModemTypeRW101, nil
	} else if deviceID == "usb:33f8:0115" {
		return cellularconst.ModemTypeRW135, nil
	} else if deviceID == "usb:33f8:0802" {
		return cellularconst.ModemTypeRW350, nil
	} else {
		return cellularconst.ModemTypeUnknown, errors.Errorf("cannot convert device ID %q to ModemType", deviceID)
	}
}

// IsVariantKnown checks if the DUT's variant is in |KnownVariants|.
func IsVariantKnown(ctx context.Context) error {
	if _, err := getDevice(ctx); err != nil {
		return err
	}
	return nil
}

// IsModemType checks if the DUT's modem matches  |KnownVariants|.
func IsModemType(ctx context.Context, modemType cellularconst.ModemType) (bool, error) {
	device, err := getDevice(ctx)
	if err != nil {
		return false, err
	}
	return device.Modem == modemType, nil
}

// TagKnownBug adds a tag to the error code.
func TagKnownBug(ctx context.Context, errIn error, bugNumber string) error {
	return errors.Wrapf(errIn, "known bug: %q", bugNumber)
}

// TagKnownBugOnVariant adds a tag to the error code if any of the |variants| matches the DUT's variant.
func TagKnownBugOnVariant(ctx context.Context, errIn error, bugNumber string, variants []string) error {
	dutVariant, err := GetDeviceVariant(ctx)
	if err == nil {
		for _, variant := range variants {
			if dutVariant == variant {
				return errors.Wrapf(errIn, "known bug on variant: %q bug: %q", variant, bugNumber)
			}
		}
	}
	return errIn
}

// TagKnownBugOnBoard adds a tag to the error code if any of the |boards| matches the DUT's board.
func TagKnownBugOnBoard(ctx context.Context, errIn error, bugNumber string, boards []string) error {
	dutVariant, err := GetDeviceVariant(ctx)
	device, ok := cellularconst.KnownVariants[dutVariant]
	if err == nil && ok {
		for _, board := range boards {
			if device.Board == board {
				return errors.Wrapf(errIn, "known bug on board: %q bug: %q", board, bugNumber)
			}
		}
	}
	return errIn
}

// ModemFwFilter is a filter for FW versions on a specific modem.
type ModemFwFilter uint32

// Supported modem fw filters.
const (
	ModemFwFilterL850MR5AndLower ModemFwFilter = iota
	ModemFwFilterL850MR7AndLower
	ModemFwFilterL850MR8AndLower
	ModemFwFilterL850MR8
	ModemFwFilterNL668A01
	ModemFwFilterNL668A04
	ModemFwFilterFM350MR1
	ModemFwFilterFM350MR3AndLower
	ModemFwFilterFM350MR4
	ModemFwFilterFM101MR1
	ModemFwFilterFM101MR2
	ModemFwFilterEM060V01
	ModemFwFilterLCUK54V01
	ModemFwFilterRW101MR1
	ModemFwFilterRW135MR1
	ModemFwFilterRW350MR0
	ModemFwFilterSC7180All
)

// IsMatch returns true the modem type and modem FW match the filter
func (filter ModemFwFilter) IsMatch(modemType cellularconst.ModemType, fwVersion string) bool {
	switch filter {
	case ModemFwFilterL850MR5AndLower:
		if modemType != cellularconst.ModemTypeL850 {
			return false
		}
		r := regexp.MustCompile("^18500.5001.[0-9]{2}.0[2-4].[0-9]{2}.[0-9]{2}.*")
		return r.MatchString(fwVersion)
	case ModemFwFilterL850MR7AndLower:
		if modemType != cellularconst.ModemTypeL850 {
			return false
		}
		r := regexp.MustCompile("^18500.5001.[0-9]{2}.0[5-6].[0-9]{2}.[0-9]{2}.*")
		return ModemFwFilterL850MR5AndLower.IsMatch(modemType, fwVersion) || r.MatchString(fwVersion)
	case ModemFwFilterL850MR8:
		if modemType != cellularconst.ModemTypeL850 {
			return false
		}
		r := regexp.MustCompile("^18500.5001.[0-9]{2}.07.[0-9]{2}.[0-9]{2}.*")
		return r.MatchString(fwVersion)
	case ModemFwFilterL850MR8AndLower:
		if modemType != cellularconst.ModemTypeL850 {
			return false
		}
		return ModemFwFilterL850MR7AndLower.IsMatch(modemType, fwVersion) || ModemFwFilterL850MR8.IsMatch(modemType, fwVersion)
	case ModemFwFilterNL668A01:
		if modemType == cellularconst.ModemTypeNL668 && strings.HasSuffix(fwVersion, "A01") {
			return true
		}
		return false
	case ModemFwFilterNL668A04:
		if modemType == cellularconst.ModemTypeNL668 && strings.HasSuffix(fwVersion, "A04") {
			return true
		}
		return false
	case ModemFwFilterFM350MR1:
		if modemType != cellularconst.ModemTypeFM350 {
			return false
		}
		return strings.HasPrefix(fwVersion, "81600.0000.00.29.19.16")
	case ModemFwFilterFM350MR3AndLower:
		if modemType != cellularconst.ModemTypeFM350 {
			return false
		}
		return ModemFwFilterFM350MR1.IsMatch(modemType, fwVersion) ||
			strings.HasPrefix(fwVersion, "81600.0000.00.29.21.21") || // MR2
			strings.HasPrefix(fwVersion, "81600.0000.00.29.21.24") || // MR3
			strings.HasPrefix(fwVersion, "81600.0000.00.29.23.27") // MR3 Softbank
	case ModemFwFilterFM350MR4:
		if modemType != cellularconst.ModemTypeFM350 {
			return false
		}
		return strings.HasPrefix(fwVersion, "81600.0000.00.29.23.06")
	case ModemFwFilterFM101MR1:
		if modemType != cellularconst.ModemTypeFM101 {
			return false
		}
		r := regexp.MustCompile("^1950[0-9].0000.00.01.01.[0-9]{2}.*")
		return r.MatchString(fwVersion)
	case ModemFwFilterFM101MR2:
		if modemType != cellularconst.ModemTypeFM101 {
			return false
		}
		r := regexp.MustCompile("^1950[0-9].0000.00.01.02.[0-9]{2}.*")
		return r.MatchString(fwVersion)
	case ModemFwFilterEM060V01:
		if modemType != cellularconst.ModemTypeEM060 {
			return false
		}
		r := regexp.MustCompile("^EM060KGLAAR01(A11|A12)M2G")
		return r.MatchString(fwVersion)
	case ModemFwFilterLCUK54V01:
		if modemType != cellularconst.ModemTypeLCUK54 {
			return false
		}
		r := regexp.MustCompile("^LCUK54WWDBL0102")
		return r.MatchString(fwVersion)
	case ModemFwFilterRW101MR1:
		if modemType != cellularconst.ModemTypeRW101 {
			return false
		}

		r := regexp.MustCompile("^19512.0000.00.11.02.[0-9]{2}.*")
		return r.MatchString(fwVersion)
	case ModemFwFilterRW135MR1:
		if modemType != cellularconst.ModemTypeRW135 {
			return false
		}
		r := regexp.MustCompile("^8960[0-9].0000.00.01.[0-9]{2}.[0-9]{2}.*")
		return r.MatchString(fwVersion)
	case ModemFwFilterRW350MR0:
		if modemType != cellularconst.ModemTypeRW350 {
			return false
		}
		r := regexp.MustCompile("^81601.0000.00.29.[0-9]{2}.[0-9]{2}.*")
		return r.MatchString(fwVersion)
	case ModemFwFilterSC7180All:
		if modemType != cellularconst.ModemTypeSC7180 {
			return false
		}
		// As far as I know, all versions in trogdor are the same.
		return true
	}
	panic("unhandled switch case")
}

// IsModemFirmwareKnown Test
func IsModemFirmwareKnown(ctx context.Context) error {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create modem")
	}
	fwVersion, err := modem.GetFwVersion(ctx, modem)
	if err != nil {
		return errors.Wrap(err, "failed to get FW version")
	}
	testing.ContextLog(ctx, "FW version: ", fwVersion)
	modemType, err := GetModemType(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get modem type")
	}
	switch modemType {
	case cellularconst.ModemTypeEM060:
		if ModemFwFilterEM060V01.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeLCUK54:
		if ModemFwFilterLCUK54V01.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeFM101:
		if ModemFwFilterFM101MR1.IsMatch(modemType, fwVersion) || ModemFwFilterFM101MR2.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeFM350:
		if ModemFwFilterFM350MR3AndLower.IsMatch(modemType, fwVersion) || ModemFwFilterFM350MR4.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeL850:
		if ModemFwFilterL850MR8AndLower.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeNL668:
		if ModemFwFilterNL668A01.IsMatch(modemType, fwVersion) || ModemFwFilterNL668A04.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeRW101:
		if ModemFwFilterRW101MR1.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeRW135:
		if ModemFwFilterRW135MR1.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeRW350:
		if ModemFwFilterRW350MR0.IsMatch(modemType, fwVersion) {
			return nil
		}
	case cellularconst.ModemTypeSC7180:
		if ModemFwFilterSC7180All.IsMatch(modemType, fwVersion) {
			return nil
		}
	}
	return errors.Errorf("unknown FW version %s on modem: %s", fwVersion, modemType.String())
}

// ModemFwMatch returns (true,modemType, fwVersion, nil) if the ModemFwFilter matches the DUT's Modem type
// and FW version, otherwise returns an error.
func ModemFwMatch(ctx context.Context, filter ModemFwFilter) (bool, cellularconst.ModemType, string, error) {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return false, cellularconst.ModemTypeUnknown, "", errors.Wrap(err, "failed to create modem")
	}
	fwVersion, err := modem.GetFwVersion(ctx, modem)
	if err != nil {
		return false, cellularconst.ModemTypeUnknown, "", errors.Wrap(err, "failed to get FW version")
	}
	device, err := getDevice(ctx)
	if err != nil {
		return false, cellularconst.ModemTypeUnknown, "", errors.Wrap(err, "failed to get device info")
	}
	return filter.IsMatch(device.Modem, fwVersion), device.Modem, fwVersion, nil
}

// TagKnownBugOnModem adds a tag to the error code if any of the |filters| match the DUT's Modem type and FW version.
func TagKnownBugOnModem(ctx context.Context, errIn error, bugNumber string, filter ModemFwFilter) error {
	match, modemType, fwVersion, err := ModemFwMatch(ctx, filter)
	if err != nil || !match {
		return errIn
	}
	return errors.Wrapf(errIn, "known bug on modem: %q and fw: %q bug: %q", modemType, fwVersion, bugNumber)
}

// ErrorToCleanString returns the string value of |errIn| if not nil, otherwise returns an empty string.
func ErrorToCleanString(errIn error) string {
	if errIn == nil {
		return ""
	}
	return fmt.Sprintf("%q", errIn)
}

// GetShillUpstartArgsForVerboseLogging Returns the upstart arguments to configure verbose logging in shill.
func GetShillUpstartArgsForVerboseLogging() []upstart.Arg {
	return []upstart.Arg{upstart.WithArg("SHILL_LOG_SCOPES", verboseShillLogScopes),
		upstart.WithArg("SHILL_LOG_LEVEL", strconv.Itoa(verboseShillLogLevel))}
}

// GetMMUpstartArgsForVerboseLogging Returns the upstart arguments to configure verbose logging in modemmanager.
func GetMMUpstartArgsForVerboseLogging() []upstart.Arg {
	return []upstart.Arg{upstart.WithArg("MM_LOGLEVEL", "DEBUG")}
}

func setShillLoggingConfig(ctx context.Context, level int, scopes []string) error {
	manager, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create shill manager proxy")
	}

	if err := manager.SetDebugLevel(ctx, level); err != nil {
		return errors.Wrap(err, "failed to set the debug level")
	}

	if err := manager.SetDebugTags(ctx, scopes); err != nil {
		return errors.Wrap(err, "failed to set the debug tags")
	}

	return nil
}

// SetShillVerboseLogging sets the device logging configuration in shill to verbose.
func SetShillVerboseLogging(ctx context.Context) error {
	return setShillLoggingConfig(ctx, verboseShillLogLevel, strings.Split(verboseShillLogScopes, "+"))
}

// SetShillDefaultLogging sets the device logging to its default level.
func SetShillDefaultLogging(ctx context.Context) error {
	return setShillLoggingConfig(ctx, 0, []string{})
}

// RestartModemManager  - restart modemmanager with debug logs enabled
// Return nil if restart succeeds, else return error.
func RestartModemManager(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "Helper.RestartModemManager")
	defer st.End()

	if err := upstart.RestartJob(ctx, modemmanager.JobName, GetMMUpstartArgsForVerboseLogging()...); err != nil {
		return errors.Wrap(err, "failed to restart modemmanager")
	}

	return nil
}
