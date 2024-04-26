// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlputil

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy/reportingutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Action represents the supported DLP actions.
type Action int

const (
	// ClipboardCopyPaste identifies a clipboard copy and paste action.
	ClipboardCopyPaste Action = iota
	// Printing identifies a printing action.
	Printing
	// Screenshot identifies a screenshot action.
	Screenshot
	// Screenshare identifies a screenshare action.
	Screenshare
	// Files identifies a files action.
	Files
)

// String returns a string representation of `Action`.
func (action Action) String() string {
	switch action {
	case ClipboardCopyPaste:
		return "CLIPBOARD"
	case Printing:
		return "PRINTING"
	case Screenshot:
		return "SCREENSHOT"
	case Screenshare:
		return "SCREENCAST"
	case Files:
		return "FILES"
	default:
		return fmt.Sprintf("String() not defined for Action %d", int(action))
	}
}

const (
	// RestrictionReportReportingEnabledUsernameAsh is the path to the secret username having report restriction level for all components, reporting enabled, and using Ash as primary browser.
	RestrictionReportReportingEnabledUsernameAsh = "dlp.restriction_level_report_reporting_enabled_username"
	// RestrictionReportReportingEnabledPasswordAsh is the path to the secret password having report restriction level for all components, reporting enabled, and using Ash as primary browser.
	RestrictionReportReportingEnabledPasswordAsh = "dlp.restriction_level_report_reporting_enabled_password"
	// RestrictionReportReportingEnabledUsernameLacros is the path to the secret username having report restriction level for all components, reporting enabled, and using Lacros as primary browser.
	RestrictionReportReportingEnabledUsernameLacros = "dlp.restriction_level_report_reporting_enabled_username_lacros"
	// RestrictionReportReportingEnabledPasswordLacros is the path to the secret password having report restriction level for all components, reporting enabled, and using Lacros as primary browser.
	RestrictionReportReportingEnabledPasswordLacros = "dlp.restriction_level_report_reporting_enabled_password_lacros"
	// RestrictionBlockReportingEnabledUsernameAsh is the path to the secret username having block restriction level for all components, reporting enabled, and using Ash as primary browser.
	RestrictionBlockReportingEnabledUsernameAsh = "dlp.restriction_level_block_reporting_enabled_username"
	// RestrictionBlockReportingEnabledPasswordAsh is the path to the secret password having block restriction level for all components, reporting enabled, and using Ash as primary browser.
	RestrictionBlockReportingEnabledPasswordAsh = "dlp.restriction_level_block_reporting_enabled_password"
	// RestrictionWarnReportingEnabledUsernameAsh is the path to the secret username having warn restriction level for all components, reporting enabled, and using Ash as primary browser.
	RestrictionWarnReportingEnabledUsernameAsh = "dlp.restriction_level_warn_reporting_enabled_username"
	// RestrictionWarnReportingEnabledPasswordAsh is the path to the secret password having warn restriction level for all components, reporting enabled, and using Ash as primary browser.
	RestrictionWarnReportingEnabledPasswordAsh = "dlp.restriction_level_warn_reporting_enabled_password"
)

// EventsBundle contains an events vector per restriction level and it is populated by `retrieveEvents`.
type EventsBundle struct {
	block       []reportingutil.InputEvent
	report      []reportingutil.InputEvent
	warn        []reportingutil.InputEvent
	warnProceed []reportingutil.InputEvent
}

// Size returns the total number of events stored.
func (events EventsBundle) Size() int {
	return len(events.block) + len(events.report) + len(events.warn) + len(events.warnProceed)
}

// EventsCounts contains expected counts of report events per restriction level.
type EventsCounts struct {
	Block       int
	Report      int
	Warn        int
	WarnProceed int
}

// Sum returns the sum of event counts across restrictions.
func (counts EventsCounts) Sum() int {
	return counts.Block + counts.Report + counts.Warn + counts.WarnProceed
}

const (
	// HTMLFile is the path of the HTML download page.
	HTMLFile = "dlp/download.html"

	// DataFile is the path of the file that's downloaded in the test.
	DataFile = "dlp/data.txt"

	// RemoteHTMLFile is the name of the remote HTML file.
	RemoteHTMLFile = "download.html"

	// RemoteDataFile is the name of the remote file that's downloaded in the test.
	RemoteDataFile = "data.txt"
)

// RetrieveEvents returns events for every restriction level having a timestamp greater than `testStartTime` with the given `clientID`.
func RetrieveEvents(ctx context.Context, customerID, APIKey, clientID string, testStartTime time.Time) (*EventsBundle, error) {

	// Retrieve all DLP events stored in the server side.
	dlpEvents, err := reportingutil.LookupEvents(ctx, customerID, clientID, APIKey, "DLP_EVENTS", testStartTime)
	if err != nil {
		return nil, errors.Wrap(err, "failed to look up events")
	}

	// Reduce events to those associate to `clientID` and starting after `testStartTime`.
	prunedEvents, err := reportingutil.PruneEvents(ctx, dlpEvents, func(e reportingutil.InputEvent) bool {
		return true
	})
	if err != nil {
		return nil, errors.New("failed to prune events")
	}

	var events EventsBundle

	// Organize events according to their mode.
	for _, event := range prunedEvents {
		mode := event.WrappedEncryptedData.DlpPolicyEvent.Mode
		switch {
		case mode == "BLOCK":
			events.block = append(events.block, event)
		case mode == "REPORT":
			events.report = append(events.report, event)
		case mode == "WARN":
			events.warn = append(events.warn, event)
		case mode == "WARN_PROCEED":
			events.warnProceed = append(events.warnProceed, event)
		default:
			return nil, errors.New("unsupported event mode")
		}
	}

	return &events, nil

}

// validateReportEvents is a helper function that checks whether events array contains the correct number of events for a given restriction and mode.
func validateReportEvents(ctx context.Context, events []reportingutil.InputEvent, restriction, mode string, want int) error {
	if len(events) != want {
		testing.ContextLogf(ctx, "Unexpected %s events = got %d, want %d", mode, len(events), want)
		return errors.Errorf("unexpected number of %s events = got %d, want %d", mode, len(events), want)
	}

	if len(events) > 0 {
		if actualRestriction := events[0].WrappedEncryptedData.DlpPolicyEvent.Restriction; actualRestriction != restriction {
			testing.ContextLogf(ctx, "Unexpected restriction = got %v, want %s", actualRestriction, restriction)
			return errors.Errorf("unexpected restriction = got %v, want %s", actualRestriction, restriction)
		}
	}

	return nil
}

// ValidateReportEvents checks whether events array contains the correct events.
func ValidateReportEvents(ctx context.Context, action Action, events *EventsBundle, expectedCounts *EventsCounts) error {

	var firstErr error

	if err := validateReportEvents(ctx, events.block, Action.String(action), "BLOCK", expectedCounts.Block); err != nil {
		firstErr = err
	}

	if err := validateReportEvents(ctx, events.report, Action.String(action), "REPORT", expectedCounts.Report); err != nil && firstErr == nil {
		firstErr = err
	}

	if err := validateReportEvents(ctx, events.warn, Action.String(action), "WARN", expectedCounts.Warn); err != nil && firstErr == nil {
		firstErr = err
	}

	if err := validateReportEvents(ctx, events.warnProceed, Action.String(action), "WARN_PROCEED", expectedCounts.WarnProceed); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}
