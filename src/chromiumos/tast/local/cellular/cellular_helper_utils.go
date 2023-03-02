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

	"chromiumos/tast/common/cellularconst"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/crosconfig"
	"chromiumos/tast/local/modemmanager"
	"chromiumos/tast/local/shill"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

const verboseShillLogLevel = -3
const verboseShillLogScopes = "cellular+modem+device+dbus+manager"

type deviceInfo struct {
	ModemVariant string
	Board        string
	Modem        cellularconst.ModemType
}

var (
	deviceVariant = ""
)

var (
	knownVariants = map[string]deviceInfo{
		"anahera_l850":       {"anahera_l850", "brya", cellularconst.ModemTypeL850},
		"brya_fm350":         {"brya_fm350", "brya", cellularconst.ModemTypeFM350},
		"brya_l850":          {"brya_l850", "brya", cellularconst.ModemTypeL850},
		"crota_fm101":        {"crota_fm101", "brya", cellularconst.ModemTypeFM101},
		"primus_l850":        {"primus_l850", "brya", cellularconst.ModemTypeL850},
		"redrix_fm350":       {"redrix_fm350", "brya", cellularconst.ModemTypeFM350},
		"redrix_l850":        {"redrix_l850", "brya", cellularconst.ModemTypeL850},
		"vell_fm350":         {"vell_fm350", "brya", cellularconst.ModemTypeFM350},
		"astronaut":          {"astronaut", "coral", cellularconst.ModemTypeL850},
		"krabby_fm101":       {"krabby_fm101", "corsola", cellularconst.ModemTypeFM101},
		"rusty_fm101":        {"rusty_fm101", "corsola", cellularconst.ModemTypeFM101},
		"steelix_fm101":      {"steelix_fm101", "corsola", cellularconst.ModemTypeFM101},
		"beadrix_nl668am":    {"beadrix_nl668am", "dedede", cellularconst.ModemTypeNL668},
		"boten":              {"boten", "dedede", cellularconst.ModemTypeL850},
		"bugzzy_l850gl":      {"bugzzy_l850gl", "dedede", cellularconst.ModemTypeL850},
		"bugzzy_nl668am":     {"bugzzy_nl668am", "dedede", cellularconst.ModemTypeNL668},
		"cret":               {"cret", "dedede", cellularconst.ModemTypeL850},
		"drawper_l850gl":     {"drawper_l850gl", "dedede", cellularconst.ModemTypeL850},
		"kracko_nl668am":     {"kracko_nl668am", "dedede", cellularconst.ModemTypeNL668},
		"kracko_fm101_cat12": {"kracko_fm101_cat12", "dedede", cellularconst.ModemTypeFM101},
		"kracko_fm101_cat6":  {"kracko_fm101_cat6", "dedede", cellularconst.ModemTypeFM101},
		"metaknight":         {"metaknight", "dedede", cellularconst.ModemTypeL850},
		"sasuke":             {"sasuke", "dedede", cellularconst.ModemTypeL850},
		"sasuke_nl668am":     {"sasuke_nl668am", "dedede", cellularconst.ModemTypeNL668},
		"sasukette":          {"sasukette", "dedede", cellularconst.ModemTypeL850},
		"storo360_l850gl":    {"storo360_l850gl", "dedede", cellularconst.ModemTypeL850},
		"storo360_nl668am":   {"storo360_nl668am", "dedede", cellularconst.ModemTypeNL668},
		"storo_l850gl":       {"storo_l850gl", "dedede", cellularconst.ModemTypeL850},
		"storo_nl668am":      {"storo_nl668am", "dedede", cellularconst.ModemTypeNL668},
		"guybrush360_l850":   {"guybrush360_l850", "guybrush", cellularconst.ModemTypeL850},
		"guybrush_fm350":     {"guybrush_fm350", "guybrush", cellularconst.ModemTypeFM350},
		"nipperkin":          {"nipperkin", "guybrush", cellularconst.ModemTypeL850},
		"jinlon":             {"jinlon", "hatch", cellularconst.ModemTypeL850},
		"evoker_sc7280":      {"evoker_sc7280", "herobrine", cellularconst.ModemTypeSC7280},
		"herobrine_sc7280":   {"herobrine_sc7280", "herobrine", cellularconst.ModemTypeSC7280},
		"hoglin_sc7280":      {"hoglin_sc7280", "herobrine", cellularconst.ModemTypeSC7280},
		"piglin_sc7280":      {"piglin_sc7280", "herobrine", cellularconst.ModemTypeSC7280},
		"villager_sc7280":    {"villager_sc7280", "herobrine", cellularconst.ModemTypeSC7280},
		"zoglin_sc7280":      {"zoglin_sc7280", "herobrine", cellularconst.ModemTypeSC7280},
		"zombie_sc7280":      {"zombie_sc7280", "herobrine", cellularconst.ModemTypeSC7280},
		"gooey":              {"gooey", "keeby", cellularconst.ModemTypeL850},
		"dood":               {"dood", "octopus", cellularconst.ModemTypeL850},
		"droid":              {"droid", "octopus", cellularconst.ModemTypeL850},
		"fleex":              {"fleex", "octopus", cellularconst.ModemTypeL850},
		"garg":               {"garg", "octopus", cellularconst.ModemTypeL850},
		"craask_fm101":       {"craask_fm101", "nissa", cellularconst.ModemTypeFM101},
		"nivviks_fm101":      {"nivviks_fm101", "nissa", cellularconst.ModemTypeFM101},
		"pujjo_fm101":        {"pujjo_fm101", "nissa", cellularconst.ModemTypeFM101},
		"arcada":             {"arcada", "sarien", cellularconst.ModemTypeL850},
		"sarien":             {"sarien", "sarien", cellularconst.ModemTypeL850},
		"coachz":             {"coachz", "strongbad", cellularconst.ModemTypeSC7180},
		"quackingstick":      {"quackingstick", "strongbad", cellularconst.ModemTypeSC7180},
		"kingoftown":         {"kingoftown", "trogdor", cellularconst.ModemTypeSC7180},
		"lazor":              {"lazor", "trogdor", cellularconst.ModemTypeSC7180},
		"limozeen":           {"limozeen", "trogdor", cellularconst.ModemTypeSC7180},
		"pazquel":            {"pazquel", "trogdor", cellularconst.ModemTypeSC7180},
		"pazquel360":         {"pazquel360", "trogdor", cellularconst.ModemTypeSC7180},
		"skyrim_fm101":       {"skyrim_fm101", "skyrim", cellularconst.ModemTypeFM101},
		"vilboz":             {"vilboz", "zork", cellularconst.ModemTypeNL668},
		"vilboz360":          {"vilboz360", "zork", cellularconst.ModemTypeL850},
	}
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

func getDevice(ctx context.Context) (deviceInfo, error) {
	dutVariant, err := GetDeviceVariant(ctx)
	if err != nil {
		return deviceInfo{}, err
	}
	device, ok := knownVariants[dutVariant]
	if !ok {
		return deviceInfo{}, errors.Errorf("variant %q is not in |knownVariants|", dutVariant)
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

// IsVariantKnown checks if the DUT's variant is in |knownVariants|.
func IsVariantKnown(ctx context.Context) error {
	if _, err := getDevice(ctx); err != nil {
		return err
	}
	return nil
}

// IsModemType checks if the DUT's modem matches  |knownVariants|.
func IsModemType(ctx context.Context, modemType cellularconst.ModemType) (bool, error) {
	device, err := getDevice(ctx)
	if err != nil {
		return false, err
	}
	return device.Modem == modemType, nil
}

// TagKnownBugOnVariant adds a tag to the error code if any of the |variants| matches the DUT's variant.
func TagKnownBugOnVariant(ctx context.Context, errIn error, bugNumber string, variants []string) error {
	dutVariant, err := GetDeviceVariant(ctx)
	if err == nil {
		for _, variant := range variants {
			if dutVariant == variant {
				return errors.Wrapf(err, "known bug on variant: %q bug: %q", variant, bugNumber)
			}
		}
	}
	return errIn
}

// TagKnownBugOnBoard adds a tag to the error code if any of the |boards| matches the DUT's board.
func TagKnownBugOnBoard(ctx context.Context, errIn error, bugNumber string, boards []string) error {
	dutVariant, err := GetDeviceVariant(ctx)
	device, ok := knownVariants[dutVariant]
	if err == nil && ok {
		for _, board := range boards {
			if device.Board == board {
				return errors.Wrapf(errIn, "known bug on board: %q bug: %q", board, bugNumber)
			}
		}
	}
	return errIn
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
	device, ok := knownVariants[dutVariant]
	if err == nil && ok {
		for _, modem := range modems {
			if device.Modem == modem {
				return errors.Wrapf(errIn, "known bug on modem: %q bug: %q", modem, bugNumber)
			}
		}
	}
	return errIn
}
