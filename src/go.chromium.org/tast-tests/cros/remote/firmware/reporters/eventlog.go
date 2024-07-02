// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporters

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// EventlogBootMode is a int representing the DUT's boot mode found from 'elogtool list'.
type EventlogBootMode int

// Names for the different boot modes.
const (
	NormalMode EventlogBootMode = iota + 1
	DeveloperMode
	Diagnostic
	BrokenScreen
	ManualRecovery
	manualRecoveryOrBrokenScreen
)

// Event contains the contents of one line from `elogtool list`.
type Event struct {
	Timestamp time.Time
	Message   string
	Index     int
}

// DiagType is a int representing the DUT's diagnostic type found
// from 'elogtool list'.
type DiagType int

// Names for the different diagnostic type.
const (
	StorageHealth DiagType = iota + 1
	StorageTestShort
	StorageTestShortExtended
	MemoryQuick
	MemoryFull
)

// DiagResult is a int representing the DUT's diagnostic result
// found from 'elogtool list'.
type DiagResult int

// Names for the different diagnostic result.
const (
	MiniDiagPassed DiagResult = iota + 1
	MiniDiagError
	MiniDiagFailed
	MiniDiagAborted
)

// DiagLog contains the contents of diagnostics mode information from
// `elogtool list`.
type DiagLog struct {
	TypeMsg DiagType
	Result  DiagResult
}

func parseEventTime(input string) (time.Time, error) {
	var err error
	for _, timeFmt := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04:05-0700"} {
		var timestamp time.Time
		timestamp, err = time.Parse(timeFmt, input)
		if err == nil {
			return timestamp, nil
		}
	}
	return time.Time{}, err
}

// ClearEventlog runs 'elogtool clear' to clear elog.
func (r *Reporter) ClearEventlog(ctx context.Context) error {
	testing.ContextLog(ctx, "Clearing the event log")
	return r.d.Conn().CommandContext(ctx, "elogtool", "clear").Run(ssh.DumpLogOnError)
}

// GetRawEventLogs returns the result of `elogtool list` with
// the UTC timezone set.
func (r *Reporter) GetRawEventLogs(ctx context.Context) (string, error) {
	events, err := r.CommandOutput(ctx, "env", "TZ=UTC", "elogtool", "list")
	if err != nil {
		return "", errors.Wrap(err, "failed to get event logs")
	}
	return events, nil
}

// EventlogList returns the result of `elogtool list`.
// The returned events are sorted from oldest to newest.
func (r *Reporter) EventlogList(ctx context.Context) ([]Event, error) {
	output, err := r.GetRawEventLogs(ctx)
	if err != nil {
		return []Event{}, err
	}
	splitOutputEvents := strings.Split(output, "\n")
	var events []Event
	// Expecting output similar to this one:
	//  140 | 2021-09-20 15:11:55 | EC Event | Key Pressed
	//  141 | 2021-09-20 15:13:30 | System boot | 45
	//  142 | 2021-09-20 15:13:30 | System Reset
	for _, line := range splitOutputEvents {
		split := strings.SplitN(line, " | ", 3)
		if len(split) < 3 {
			return []Event{}, errors.Errorf("eventlog entry had fewer than 3 ' | ' delimiters: %q", line)
		}
		var timestamp time.Time
		// If the timestamp is missing, it is printed at 2000-00-00 00:00:00, but that is not a valid date and can't be parsed.
		if split[1] != "2000-00-00 00:00:00" {
			timestamp, err = parseEventTime(split[1])
			if err != nil {
				return []Event{}, err
			}
		}
		index, err := strconv.ParseInt(split[0], 10, 0)
		if err != nil {
			return []Event{}, errors.Errorf("failed to parse index %q", split[0])
		}
		events = append(events, Event{
			Timestamp: timestamp,
			Message:   split[2],
			Index:     int(index),
		})
	}
	return events, nil
}

// groupEventsByBoots groups the events from 'elogtool list' by system boots.
// arranging them from the oldest to the newest boot.
func groupEventsByBoots(events []Event) [][]Event {
	var results [][]Event
	reSystemBoot := regexp.MustCompile(`System boot`)
	for i := len(events) - 1; i > 0; i-- {
		match := reSystemBoot.FindStringSubmatch(events[i].Message)
		if match != nil {
			singleBoot := events[i+1:]
			results = append(results, singleBoot)
			events = events[0:i]
		}
	}
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return results
}

// checkFirmwareVbootInfo checks if the DUT uses a new version event log.
func checkFirmwareVbootInfo(eventMessage string) bool {
	var (
		reFirmwareVbootInfo *regexp.Regexp = regexp.MustCompile(`(?i)Firmware vboot info`)
	)
	firmwareVbootInfo := reFirmwareVbootInfo.FindStringSubmatch(eventMessage)
	if firmwareVbootInfo != nil {
		return true
	}
	return false
}

// findBootModeFromEvents takes a slice of events and returns a single
// boot mode found. Call groupEventsByBoots first to group events from
// 'elogtool list' for each boot.
func findBootModeFromEvents(events []Event) (EventlogBootMode, error) {
	var (
		reFirmwareBootMode   *regexp.Regexp = regexp.MustCompile(`boot_mode=([\w ]+)`)
		reDeprecatedBootMode *regexp.Regexp = regexp.MustCompile(`(Launch Diagnostics|Chrome\s?OS[\w ]+)`)
	)
	bootModesMap := map[string]EventlogBootMode{
		// Listed below are some supported boot modes, as documented in the
		// 'vboot_reference/firmware/2lib/include/2info.h' file.
		"Secure":          NormalMode,
		"Developer":       DeveloperMode,
		"Diagnostic":      Diagnostic,
		"Broken screen":   BrokenScreen,
		"Manual recovery": ManualRecovery,
		// Listed below are some deprecated boot modes, as documented in the
		// 'coreboot/util/cbfstool/eventlog.c' file.
		"ChromeOS Developer Mode": DeveloperMode,
		"ChromeOS Recovery Mode":  manualRecoveryOrBrokenScreen,
		"Launch Diagnostics":      Diagnostic,
	}
	var bootModes []EventlogBootMode
	hasVbootInfo := false
	for _, event := range events {
		var findBootModeRegexp *regexp.Regexp
		hasVbootInfo = checkFirmwareVbootInfo(event.Message)
		if hasVbootInfo {
			findBootModeRegexp = reFirmwareBootMode
		} else {
			findBootModeRegexp = reDeprecatedBootMode
		}
		firmwareBootMode := findBootModeRegexp.FindStringSubmatch(event.Message)
		if len(firmwareBootMode) == 2 {
			bootMode := strings.TrimSpace(firmwareBootMode[1])
			bootModes = append(bootModes, bootModesMap[bootMode])
		}
	}
	switch len(bootModes) {
	case 2:
		if !hasVbootInfo {
			// For old devices, an extra dev mode event may be logged
			// immediately after the rec mode event.
			// Drop 'ChromeOS Developer Mode', and only keep
			// 'ChromeOS Recovery Mode'.
			if bootModes[0] == manualRecoveryOrBrokenScreen && bootModes[1] == DeveloperMode {
				return manualRecoveryOrBrokenScreen, nil
			}
			// For old devices with firmware id between 14456 and 15025,
			// an extra dev mode event may be logged immediately before the
			// diagnostics mode event. Drop 'ChromeOS Developer Mode',
			// and only keep 'Diagnostics Mode'.
			if bootModes[0] == DeveloperMode && bootModes[1] == Diagnostic {
				return Diagnostic, nil
			}
		}
	case 0:
		if !hasVbootInfo {
			return NormalMode, nil
		}
	case 1:
		return bootModes[0], nil
	}
	return 0, errors.Errorf("unable to identify boot modes from elog events, found boot modes: %v", bootModes)
}

// GetBootModes gets the boot modes found from 'elogtool list'.
func (r *Reporter) GetBootModes(ctx context.Context, newEvents []Event) ([]EventlogBootMode, error) {
	groups := groupEventsByBoots(newEvents)
	var foundBootModes []EventlogBootMode
	for _, events := range groups {
		bootMode, err := findBootModeFromEvents(events)
		if err != nil {
			return foundBootModes, errors.Wrap(err, "failed to find boot mode")
		}
		foundBootModes = append(foundBootModes, bootMode)
	}
	return foundBootModes, nil
}

// CheckBootModes checks for boot modes found from 'elogtool list'
// against the expected ones.
func (r *Reporter) CheckBootModes(ctx context.Context, newEvents []Event, expectedBootModes []EventlogBootMode) error {
	foundBootModes, err := r.GetBootModes(ctx, newEvents)
	if err != nil {
		return errors.Wrap(err, "failed to get boot modes")
	}
	if len(foundBootModes) != len(expectedBootModes) {
		return errors.Errorf("found %d boot modes from the event log, but expected %d, found boot modes: %v", len(foundBootModes), len(expectedBootModes), foundBootModes)
	}
	for idx, val := range foundBootModes {
		if val == manualRecoveryOrBrokenScreen {
			if expectedBootModes[idx] == BrokenScreen || expectedBootModes[idx] == ManualRecovery {
				continue
			}
		} else if val == expectedBootModes[idx] {
			continue
		}
		return errors.Errorf("found %v, but expected %v", val, expectedBootModes[idx])
	}
	return nil
}

// findRecoveryReasonFromEvents takes a slice of events and returns
// recovery reasons found.
func findRecoveryReasonFromEvents(events []Event) []RecoveryReasonString {
	var (
		reRecoveryReason           *regexp.Regexp = regexp.MustCompile(`recovery_reason=.*\((.*)\)`)
		reDeprecatedRecoveryReason *regexp.Regexp = regexp.MustCompile(`Chrome\s?OS[\w ]+\|\s(.*)\s\|.0x\w+`)
	)
	var findRecoveryReasonRegexp *regexp.Regexp
	var recoveryReasonFound []RecoveryReasonString
	hasVbootInfo := false
	for _, event := range events {
		hasVbootInfo = checkFirmwareVbootInfo(event.Message)
		if hasVbootInfo {
			findRecoveryReasonRegexp = reRecoveryReason
		} else {
			findRecoveryReasonRegexp = reDeprecatedRecoveryReason
		}
		recoveryReasonInEventLog := findRecoveryReasonRegexp.FindStringSubmatch(event.Message)
		if len(recoveryReasonInEventLog) == 2 {
			recoveryReasonFound = append(recoveryReasonFound, RecoveryReasonString(recoveryReasonInEventLog[1]))
		}
	}
	return recoveryReasonFound
}

// CheckRecoveryEventExists checks if specific recovery reason exists
// in eventlog.
func (r *Reporter) CheckRecoveryEventExists(ctx context.Context, events []Event, expRecoveryReason RecoveryReason) bool {
	for _, event := range events {
		recoveryReasonFound := findRecoveryReasonFromEvents([]Event{event})
		expectedRecoveryReasonString, _ := r.GetRecoveryReasonString(ctx, expRecoveryReason)
		if len(recoveryReasonFound) != 0 && recoveryReasonFound[0] == expectedRecoveryReasonString {
			testing.ContextLog(ctx, "Found expected recovery reason: ", expectedRecoveryReasonString)
			return true
		}
	}
	return false
}

// CheckRecoveryEventsInEventLog checks for recovery reasons found from
// 'elogtool list' against the expected ones.
func (r *Reporter) CheckRecoveryEventsInEventLog(ctx context.Context, newEvents []Event, expRecoveryReasons []RecoveryReason) error {
	groups := groupEventsByBoots(newEvents)
	var foundRecoveryReasons []RecoveryReasonString
	for _, events := range groups {
		recoveryReasonsFound := findRecoveryReasonFromEvents(events)
		switch len(recoveryReasonsFound) {
		case 0:
			recoveryReasonNotRequestedString, err := r.GetRecoveryReasonString(ctx, RecoveryReasonNotRequested)
			if err != nil {
				return errors.Wrap(err, "failed to get recovery reason string")
			}
			foundRecoveryReasons = append(foundRecoveryReasons, recoveryReasonNotRequestedString)
		case 1:
			foundRecoveryReasons = append(foundRecoveryReasons, recoveryReasonsFound[0])
		default:
			return errors.Errorf("unable to identify recovery reason from elog events, found recovery reasons: %v", recoveryReasonsFound)
		}
	}
	if len(foundRecoveryReasons) != len(expRecoveryReasons) {
		return errors.Errorf("found %d recovery reason from the event log, but expected %d, found recovery reasons: %v", len(foundRecoveryReasons), len(expRecoveryReasons), foundRecoveryReasons)
	}
	for idx, val := range foundRecoveryReasons {
		expectedRecoveryReasonString, err := r.GetRecoveryReasonString(ctx, expRecoveryReasons[idx])
		if err != nil {
			return errors.Wrap(err, "failed to get recovery reason string")
		}
		if expectedRecoveryReasonString == val {
			continue
		}
		return errors.Errorf("found %v, but expected %v", val, expectedRecoveryReasonString)
	}
	return nil
}

// findDiagnosticsLogsFromEvents takes a slice of events and returns
// diagnostics logs found.
func findDiagnosticsLogsFromEvents(events []Event) ([]DiagLog, error) {
	var (
		reDiagnosticsLogs   *regexp.Regexp = regexp.MustCompile(`Diagnostics Mode`)
		reDiagTypeAndResult *regexp.Regexp = regexp.MustCompile(`type=([\w (\w)]+), result=(\w+)`)
	)
	diagnosticsTypesMap := map[string]DiagType{
		"Storage health info":          StorageHealth,
		"Storage self-test (short)":    StorageTestShort,
		"Storage self-test (extended)": StorageTestShortExtended,
		"Memory check (quick)":         MemoryQuick,
		"Memory check (full)":          MemoryFull,
	}
	diagnosticsResultsMap := map[string]DiagResult{
		"Passed":  MiniDiagPassed,
		"Error":   MiniDiagError,
		"Failed":  MiniDiagFailed,
		"Aborted": MiniDiagAborted,
	}
	var results []DiagLog
	var diagEvent Event
	for _, event := range events {
		hasDiagnosticsLogs := reDiagnosticsLogs.MatchString(event.Message)
		if hasDiagnosticsLogs {
			if diagEvent != (Event{}) {
				return results, errors.Errorf("more than 1 diagnostics event found: %s, %s", diagEvent.Message, event.Message)
			}
			diagEvent = event
		}
	}
	if diagEvent == (Event{}) {
		return results, errors.New("no diagnostics event found")
	}
	/*
		Below is one example event log from running pre-boot diagnostics:
		"Diagnostics Mode | Diagnostics Logs | type=Storage health info, result=Passed, time=0m0s | type=Memory check (quick), result=Aborted, time=0m0s".
		After splitting the string by "|", the diagnostic logs begin from the 2nd entry to the last.
	*/
	diagLogs := strings.Split(diagEvent.Message, "|")[2:]
	for _, logTypeResult := range diagLogs {
		out := reDiagTypeAndResult.FindStringSubmatch(logTypeResult)
		if len(out) != 3 {
			return results, errors.New("did not get expected numbers of match for diagnostics logs")
		}
		results = append(results, DiagLog{TypeMsg: diagnosticsTypesMap[out[1]], Result: diagnosticsResultsMap[out[2]]})
	}
	return results, nil
}

// CheckDiagnosticsLogs checks for diagnostics logs found from 'elogtool list'
// against the expected ones.
func (r *Reporter) CheckDiagnosticsLogs(ctx context.Context, newEvents []Event, expectedLogs []DiagLog) error {
	groups := groupEventsByBoots(newEvents)
	prevBootEvents := groups[len(groups)-2]
	diagLogs, err := findDiagnosticsLogsFromEvents(prevBootEvents)
	if err != nil {
		return errors.Wrap(err, "failed to find diagnostics logs from events")
	}
	if len(diagLogs) != len(expectedLogs) {
		return errors.Errorf("found %d diagnostics logs from the event log, but expected %d, found logs: %v", len(diagLogs), len(expectedLogs), diagLogs)
	}
	for idx, val := range diagLogs {
		if val != expectedLogs[idx] {
			return errors.Errorf("found %v, but expected %v", val, expectedLogs[idx])
		}
	}
	return nil
}
