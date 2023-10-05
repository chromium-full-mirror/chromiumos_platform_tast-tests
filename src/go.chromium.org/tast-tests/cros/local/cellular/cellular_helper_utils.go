// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/tklauser/go-sysconf"

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

func assignLastIntValueAndDropKey(d LabelMap, to *int, key string) LabelMap {
	if v, ok := getLastIntValue(d, key); ok {
		*to = v
	}
	delete(d, key)
	return d
}

func getLastIntValue(d LabelMap, key string) (int, bool) {
	if s, ok := getLastStringValue(d, key); ok {
		if c, err := strconv.Atoi(s); err == nil {
			return c, true
		}
	}
	return -1, false
}

func assignLastBoolValueAndDropKey(d LabelMap, to *bool, key string) LabelMap {
	if v, ok := getLastBoolValue(d, key); ok {
		*to = v
	}
	delete(d, key)
	return d
}

func getLastBoolValue(d LabelMap, key string) (bool, bool) {
	if s, ok := getLastStringValue(d, key); ok {
		return strings.ToLower(s) == "true", true
	}
	return false, false
}

func assignLastStringValueAndDropKey(d LabelMap, to *string, key string) LabelMap {
	if v, ok := getLastStringValue(d, key); ok {
		*to = v
	}
	delete(d, key)
	return d
}

func getLastStringValue(d LabelMap, key string) (string, bool) {
	if vs, ok := d[key]; ok {
		if len(vs) > 0 {
			return vs[len(vs)-1], true
		}
		return "", false
	}
	return "", false
}

func getLabelMap(labels []string) LabelMap {
	dims := make(LabelMap)
	for _, label := range labels {
		val := strings.SplitN(label, ":", 2)
		switch len(val) {
		case 1:
			dims[val[0]] = append(dims[val[0]], "")
		case 2:
			dims[val[0]] = append(dims[val[0]], val[1])
		}
	}
	return dims
}

// GetCellularCarrierFromHostInfoLabels return the current carrier name from host_info_labels, else return empty string
func GetCellularCarrierFromHostInfoLabels(ctx context.Context, labels []string) string {
	if c, ok := getLastStringValue(getLabelMap(labels), "carrier"); ok {
		return c
	}
	return ""
}

// GetStarfishMappingFromHostInfoLabels return the starfish slot mapping from host_info_labels, else return empty string
func GetStarfishMappingFromHostInfoLabels(ctx context.Context, labels []string) string {
	if c, ok := getLastStringValue(getLabelMap(labels), "starfish_slot_mapping"); ok {
		return c
	}
	return ""
}

// GetDevicePoolFromHostInfoLabels return the current device pool name from host_info_labels, else return empty string
func GetDevicePoolFromHostInfoLabels(ctx context.Context, labels []string) []string {
	var pools []string
	d := getLabelMap(labels)
	for _, v := range d["pool"] {
		pools = append(pools, v)
	}
	return pools
}

// EnsureUptime ensures that the system has been up for at least the specified amount of time before returning.
func EnsureUptime(ctx context.Context, duration time.Duration) error {
	uptimeStr, err := ioutil.ReadFile("/proc/uptime")
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
	buff, err := ioutil.ReadFile(statFilename)
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
	uptimeStr, err := ioutil.ReadFile("/proc/uptime")
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

// GetModemInfoFromHostInfoLabels populate Modem info from host_info_labels
func GetModemInfoFromHostInfoLabels(ctx context.Context, labels []string) *ModemInfo {
	var modemInfo ModemInfo
	d := getLabelMap(labels)
	if c, ok := getLastStringValue(d, "modem_type"); ok {
		modemInfo.Type = c
	}
	if c, ok := getLastStringValue(d, "modem_imei"); ok {
		modemInfo.IMEI = c
	}
	if c, ok := getLastStringValue(d, "modem_supported_bands"); ok {
		modemInfo.SupportedBands = c
	}
	if c, ok := getLastStringValue(d, "modem_sim_count"); ok {
		if v, err := strconv.Atoi(c); err == nil {
			modemInfo.SimCount = v
		} else {
			modemInfo.SimCount = 0
		}
	}
	return &modemInfo
}

// GetSIMInfoFromHostInfoLabels populate SIM info from host_info_labels
func GetSIMInfoFromHostInfoLabels(ctx context.Context, labels []string) []*SIMInfo {
	d := getLabelMap(labels)
	numSim := len(d["sim_slot_id"])
	simInfo := make([]*SIMInfo, numSim)

	for i, v := range d["sim_slot_id"] {
		simID := v
		s := &SIMInfo{}
		if j, err := strconv.Atoi(v); err == nil {
			s.SlotID = j
		}

		lv := "sim_" + simID + "_type"
		d = assignLastStringValueAndDropKey(d, &s.Type, lv)

		lv = "sim_" + simID + "_eid"
		d = assignLastStringValueAndDropKey(d, &s.EID, lv)

		lv = "sim_" + simID + "_test_esim"
		d = assignLastBoolValueAndDropKey(d, &s.TestEsim, lv)

		lv = "sim_" + simID + "_num_profiles"
		numProfiles := 0
		d = assignLastIntValueAndDropKey(d, &numProfiles, lv)

		s.ProfileInfo = make([]*SIMProfileInfo, numProfiles)
		for j := 0; j < numProfiles; j++ {
			s.ProfileInfo[j] = &SIMProfileInfo{}
			profileID := strconv.Itoa(j)
			lv = "sim_" + simID + "_" + profileID + "_iccid"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].ICCID, lv)

			lv = "sim_" + simID + "_" + profileID + "_pin"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].SimPin, lv)

			lv = "sim_" + simID + "_" + profileID + "_puk"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].SimPuk, lv)

			lv = "sim_" + simID + "_" + profileID + "_carrier_name"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].CarrierName, lv)

			lv = "sim_" + simID + "_" + profileID + "_own_number"
			d = assignLastStringValueAndDropKey(d, &s.ProfileInfo[j].OwnNumber, lv)
		}
		simInfo[i] = s
	}

	return simInfo
}

// GetLabelsAsStringArray returns the labels as a string array
func GetLabelsAsStringArray(ctx context.Context, cmd func(name string) (val string, ok bool), labelName string) ([]string, error) {
	labelsStr, ok := cmd(labelName)
	if !ok {
		return nil, errors.New("failed to read autotest_host_info_labels")
	}

	var labels []string
	if err := json.Unmarshal([]byte(labelsStr), &labels); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal label string")
	}

	return labels, nil
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

// GetModemType gets DUT's modem type.
func GetModemType(ctx context.Context) (cellularconst.ModemType, error) {
	device, err := getDevice(ctx)
	if err != nil {
		return 0, err
	}
	return device.Modem, nil
}

// GetModemTypeFromDeviceID converts a USB Device ID into ModemType.
func GetModemTypeFromDeviceID(deviceID string) (cellularconst.ModemType, error) {
	if deviceID == "usb:2cb7:0007" {
		return cellularconst.ModemTypeL850, nil
	} else if deviceID == "pci:14c3:4d75 (External)" {
		return cellularconst.ModemTypeFM350, nil
	} else if deviceID == "usb:2cb7:01a0" {
		return cellularconst.ModemTypeNL668, nil
	} else if deviceID == "usb:2cb7:01a2" {
		return cellularconst.ModemTypeFM101, nil
	} else if deviceID == "usb:2c7c:030b" {
		return cellularconst.ModemTypeEM060, nil
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

// TagKnownBugOnModemType adds a tag to the error code if any of the |modems| matches the DUT's Modem type.
func TagKnownBugOnModemType(ctx context.Context, errIn error, bugNumber string, modems []cellularconst.ModemType) error {
	dutVariant, err := GetDeviceVariant(ctx)
	device, ok := cellularconst.KnownVariants[dutVariant]
	if err == nil && ok {
		for _, modem := range modems {
			if device.Modem == modem {
				return errors.Wrapf(errIn, "known bug on modem: %q bug: %q", modem, bugNumber)
			}
		}
	}
	return errIn
}
