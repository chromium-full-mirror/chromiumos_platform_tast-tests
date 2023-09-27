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
	"go.chromium.org/tast/core/testing"
)

// EventlogBootMode is a string representing the DUT's boot mode found from 'elogtool list'.
type EventlogBootMode string

const (
	// Listed below are some supported boot modes, as documented in the
	// 'vboot_reference/firmware/2lib/include/2info.h' file.
	NormalMode     EventlogBootMode = "Secure"
	DeveloperMode  EventlogBootMode = "Developer"
	Diagnostic     EventlogBootMode = "Diagnostic"
	BrokenScreen   EventlogBootMode = "Broken screen"
	ManualRecovery EventlogBootMode = "Manual recovery"

	// Listed below are some deprecated boot modes, as documented in the
	// 'coreboot/util/cbfstool/eventlog.c' file.
	ChromeOSRecoveryMode  EventlogBootMode = "ChromeOS Recovery Mode"
	ChromeOSDeveloperMode EventlogBootMode = "ChromeOS Developer Mode"
)

// Event contains the contents of one line from `elogtool list`.
type Event struct {
	Timestamp time.Time
	Message   string
	Index     int
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

// EventlogList returns the result of `elogtool list`.
// The returned events are sorted from oldest to newest.
func (r *Reporter) EventlogList(ctx context.Context) ([]Event, error) {
	output, err := r.CommandOutputLines(ctx, "env", "TZ=UTC", "elogtool", "list")
	if err != nil {
		return []Event{}, err
	}
	var events []Event
	// Expecting output similar to this one:
	//  140 | 2021-09-20 15:11:55 | EC Event | Key Pressed
	//  141 | 2021-09-20 15:13:30 | System boot | 45
	//  142 | 2021-09-20 15:13:30 | System Reset
	for _, line := range output {
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

// EventlogListAfter returns a list of events that occurred after a given index.
func (r *Reporter) EventlogListAfter(ctx context.Context, previousEvent Event) ([]Event, error) {
	events, err := r.EventlogList(ctx)
	if err != nil {
		return []Event{}, errors.Wrap(err, "reporting events")
	}
	// EventlogList reports events from oldest to newest.
	// Iterate through the events in reverse order to return only the newest ones.
	for i := len(events) - 1; i > 0; i-- {
		if events[i].Timestamp.Before(previousEvent.Timestamp) || (events[i].Timestamp.Equal(previousEvent.Timestamp) && events[i].Index <= previousEvent.Index) {
			return events[i+1:], nil
		}
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

// findBootModesFromEvents takes a slice of event and returns the boot modes found.
// Call groupEventsByBoots first to group events from 'elogtool list' for each boot.
func findBootModesFromEvents(events []Event) []string {
	var (
		reFirmwareVbootInfo *regexp.Regexp = regexp.MustCompile(`(?i)Firmware vboot info`)
		reFirmwareBootMode  *regexp.Regexp = regexp.MustCompile(`boot_mode=([\w ]+)`)
		reChromeOSBootMode  *regexp.Regexp = regexp.MustCompile(`(Chrome\s?OS[\w ]+)`)
	)
	var bootModes []string
	for _, event := range events {
		var findBootModeRegexp *regexp.Regexp
		firmwareVbootInfo := reFirmwareVbootInfo.FindStringSubmatch(event.Message)
		if firmwareVbootInfo != nil {
			findBootModeRegexp = reFirmwareBootMode
		} else {
			findBootModeRegexp = reChromeOSBootMode
		}
		firmwareBootMode := findBootModeRegexp.FindStringSubmatch(event.Message)
		if len(firmwareBootMode) == 2 {
			bootModes = append(bootModes, strings.TrimSpace(firmwareBootMode[1]))
		}
	}
	return bootModes
}

// CheckBootModes checks for boot modes found from 'elogtool list'
// against the expected ones.
func (r *Reporter) CheckBootModes(ctx context.Context, newEvents []Event, expectedBootModes []EventlogBootMode) error {
	groups := groupEventsByBoots(newEvents)
	var foundBootModes []string
	for _, events := range groups {
		results := findBootModesFromEvents(events)
		if len(results) >= 2 {
			if len(results) == 2 && results[0] == string(ChromeOSRecoveryMode) && results[1] == string(ChromeOSDeveloperMode) {
				// For old devices, the dev mode event is logged immediately after the rec mode event.
				// Drop 'ChromeOS Developer Mode', and only keep 'ChromeOS Recovery Mode'.
				foundBootModes = append(foundBootModes, results[0])
			} else {
				// For most of the devices, expected to get one boot mode in a system boot.
				return errors.Errorf("got unexpected numbers of boot mode in a boot, got %d", len(results))
			}
		} else {
			foundBootModes = append(foundBootModes, results...)
		}
	}
	testing.ContextLog(ctx, "Found boot modes: ", foundBootModes)
	if len(foundBootModes) != len(expectedBootModes) {
		return errors.Errorf("found %d boot modes from the event log, but expected %d", len(foundBootModes), len(expectedBootModes))
	}
	for idx, val := range foundBootModes {
		if val != string(expectedBootModes[idx]) {
			return errors.Errorf("found %s, but expected %s", val, expectedBootModes[idx])
		}
	}
	return nil
}
