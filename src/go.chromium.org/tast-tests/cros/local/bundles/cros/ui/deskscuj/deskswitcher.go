// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// deskSwitchWorkflow represents a workflow for switching between desks.
// |run| switches to the "next" desk, which is defined by the |itinerary|.
// |run| takes in the currently active desk and the expected next desk,
// and activates the next desk.
type deskSwitchWorkflow struct {
	name        string // name is a name satisfying the conditions for recorder.Annotate.
	itinerary   []int
	run         func(context.Context, int, int) error
	recordTrace bool // recordTrace indicates whether to record trace.
}

// getKeyboardSearchBracketWorkflow returns the workflow for switching
// between desks using Search+[ and Search+].
func getKeyboardSearchBracketWorkflow(tconn *chrome.TestConn, kw *input.KeyboardEventWriter) deskSwitchWorkflow {
	return deskSwitchWorkflow{
		name:      "Search-Bracket",
		itinerary: []int{0, 1, 2, 3, 2, 1},
		run: func(ctx context.Context, fromDesk, toDesk int) error {
			var direction string
			switch toDesk {
			case fromDesk - 1:
				direction = "Search+["
			case fromDesk + 1:
				direction = "Search+]"
			default:
				return errors.Errorf("invalid Search+Bracket desk switch: from %d to %d", fromDesk, toDesk)
			}
			return kw.Accel(ctx, direction)
		},
		recordTrace: true,
	}
}

// getKeyboardSearchNumberWorkflow returns the workflow for switching
// between desks using Search+Shift+Number.
func getKeyboardSearchNumberWorkflow(tconn *chrome.TestConn, kw *input.KeyboardEventWriter) deskSwitchWorkflow {
	return deskSwitchWorkflow{
		name:      "Search-Shift-Number",
		itinerary: []int{0, 1, 2, 3, 2, 1},
		run: func(ctx context.Context, fromDesk, toDesk int) error {
			if fromDesk == toDesk {
				return errors.Errorf("invalid target desk, can't switch from desk %d to itself", fromDesk)
			}

			// Shortcuts are 1-indexed, so offset currDesk by 1.
			return kw.Accel(ctx, fmt.Sprintf("Shift+Search+%d", toDesk+1))
		},
	}
}

// getOverviewWorkflow returns the workflow for switching between desks
// by entering overview mode and selecting the next desk.
func getOverviewWorkflow(tconn *chrome.TestConn, ac *uiauto.Context, setOverviewModeAndWait action.Action) deskSwitchWorkflow {
	return deskSwitchWorkflow{
		name:      "Overview",
		itinerary: []int{0, 1, 2, 3, 2, 1},
		run: func(ctx context.Context, fromDesk, toDesk int) error {
			if fromDesk == toDesk {
				return errors.Errorf("invalid target desk, can't switch from desk %d to itself", fromDesk)
			}

			if err := setOverviewModeAndWait(ctx); err != nil {
				return errors.Wrap(err, "failed to enter overview mode")
			}

			desksInfo, err := ash.FindDeskMiniViews(ctx, ac)
			if err != nil {
				return errors.Wrap(err, "failed to get desk previews")
			}

			if numDesks := len(desksInfo); toDesk >= numDesks || toDesk < 0 {
				return errors.Errorf("invalid target desk: got %d, expected desk index between 0 and %d", toDesk, numDesks-1)
			}

			if err := mouse.Click(tconn, desksInfo[toDesk].Location.CenterPoint(), mouse.LeftButton)(ctx); err != nil {
				return errors.Wrapf(err, "failed to click on the desk preview for desk %d", toDesk)
			}

			if err := ash.WaitForOverviewState(ctx, tconn, ash.Hidden, time.Minute); err != nil {
				return errors.Wrap(err, "failed to exit overview mode")
			}

			return nil
		},
	}
}
