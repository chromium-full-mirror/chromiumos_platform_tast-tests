// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DeskSwitcher holds all the necessary variables used by the DeskSwitch functions.
type DeskSwitcher struct {
	tconn                 *chrome.TestConn
	recorder              *cujrecorder.Recorder
	outDir                string
	systemTraceConfigPath string
	deskSwitchWorkflows   []DeskSwitchWorkflow // The list of desk switching actions to be performed.
	deskSwitchingDuration time.Duration        // The duration how long we should run each workflow for.
	onVisitActions        []action.Action      // The list of actions to be performed on the corresponding desk.
	expectedNumWindows    int                  // The total number of windows that should exist.
	ActiveDesk            int                  // The index of the currently active desk.
}

// NewDeskSwitcher creates an instance of DeskSwitcher.
func NewDeskSwitcher(tconn *chrome.TestConn, recorder *cujrecorder.Recorder, outDir, systemTraceConfigPath string, deskSwitchingDuration time.Duration,
	deskSwitchWorkflows []DeskSwitchWorkflow, onVisitActions []action.Action, expectedNumWindows, activeDesk int) *DeskSwitcher {
	return &DeskSwitcher{
		tconn:                 tconn,
		recorder:              recorder,
		outDir:                outDir,
		systemTraceConfigPath: systemTraceConfigPath,
		deskSwitchWorkflows:   deskSwitchWorkflows,
		deskSwitchingDuration: deskSwitchingDuration,
		onVisitActions:        onVisitActions,
		expectedNumWindows:    expectedNumWindows,
		ActiveDesk:            activeDesk,
	}
}

// DeskSwitch switches the desks with specific desk switching actions, performs the corresponding actions on each desk
// and records related metrics.
func (d *DeskSwitcher) DeskSwitch(ctx context.Context) error {
	// Get a list of metrics to collect for each test phase.
	ashMetrics, browserMetrics := cujrecorder.GetShortenedPerformanceMetrics()
	ashMetrics = append(ashMetrics, "Ash.Desks.AnimationLatency.DeskActivation", "Ash.Desks.AnimationSmoothness.DeskActivation")

	ui := uiauto.New(d.tconn)

	manageTracing := func(ctx context.Context, switcher DeskSwitchWorkflow, cycles int) error {
		if !switcher.RecordTrace {
			return nil
		}
		if cycles == 0 {
			if err := d.recorder.StartTracing(ctx, d.outDir, d.systemTraceConfigPath); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
		} else if cycles == 4 {
			if err := d.recorder.StopTracing(ctx); err != nil {
				return errors.Wrap(err, "failed to stop tracing")
			}
		}
		return nil
	}

	for _, switcher := range d.deskSwitchWorkflows {
		info, err := ash.GetDesksInfo(ctx, d.tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get the desk info")
		}
		if startDesk := switcher.Itinerary[0]; info.ActiveDeskIndex != startDesk {
			if err := ash.ActivateDeskAtIndex(ctx, d.tconn, startDesk); err != nil {
				return errors.Wrapf(err, "failed to activate desk %d with the autotest API", startDesk)
			}
			d.ActiveDesk = startDesk
		}

		d.recorder.Annotate(ctx, "Cycle_through_desks_with_"+switcher.Name)

		stopSnapshot, err := d.recorder.StartSnapshot(ctx, switcher.Name, ashMetrics, browserMetrics)
		if err != nil {
			return errors.Wrapf(err, "failed to start snapshot for %s", switcher.Name)
		}

		var (
			i      = 0
			cycles = 0
		)
		for endTime := time.Now().Add(d.deskSwitchingDuration); time.Now().Before(endTime); {
			// Manage tracing based on the cycle count.
			// See go/trace-in-cuj-tests about rules for tracing.
			if err := manageTracing(ctx, switcher, cycles); err != nil {
				return errors.Wrap(err, "failed to manage tracing")
			}

			i = (i + 1) % len(switcher.Itinerary)
			nextDesk := switcher.Itinerary[i]
			switchToNextDesk := func(ctx context.Context) error {
				info, err := ash.GetDesksInfo(ctx, d.tconn)
				if err != nil {
					return errors.Wrap(err, "failed to get the desk info")
				}
				testing.ContextLogf(ctx, "Active desk index: %d, target desk index: %d", info.ActiveDeskIndex, nextDesk)

				if info.ActiveDeskIndex == nextDesk {
					testing.ContextLog(ctx, "The active desk index is already the next desk")
					return nil
				}
				if err := switcher.Run(ctx, info.ActiveDeskIndex, nextDesk); err != nil {
					return errors.Wrapf(err, "failed to switch to the next desk using %s", switcher.Name)
				}
				if err := ash.WaitForDesk(d.tconn, nextDesk)(ctx); err != nil {
					return errors.Wrapf(err, "failed to wait for the %d desk to be active", nextDesk)
				}
				return nil
			}

			if err := uiauto.Retry(3, switchToNextDesk)(ctx); err != nil {
				return err
			}

			d.ActiveDesk = nextDesk

			// Give a few seconds for the current desk to stabilize
			// before interacting with it.
			if err := ui.WithInterval(time.Second).WithTimeout(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to wait for current desk to stabilize: ", err)
			}

			if err := d.onVisitActions[d.ActiveDesk](ctx); err != nil {
				return errors.Wrapf(err, "failed to perform unique action on desk %d", d.ActiveDesk)
			}
			cycles++
		}

		if err := stopSnapshot(ctx); err != nil {
			return errors.Wrapf(err, "failed to stop snapshot for %s", switcher.Name)
		}

		// Ensure that none of the windows crashed during the test.
		if err := d.verifyWindowCount(ctx); err != nil {
			return errors.Wrap(err, "failed to verify windows count")
		}

		testing.ContextLogf(ctx, "Switched desk by %s %d times", switcher.Name, cycles)
	}

	return nil
}

// verifyWindowCount verifies the count of current windows matches |expectedNumWindows|.
func (d *DeskSwitcher) verifyWindowCount(ctx context.Context) error {
	ws, err := ash.GetAllWindows(ctx, d.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get all windows")
	}
	if len(ws) != d.expectedNumWindows {
		return errors.Errorf("unexpected number of open windows, got %d, expected %d", len(ws), d.expectedNumWindows)
	}
	return nil
}
