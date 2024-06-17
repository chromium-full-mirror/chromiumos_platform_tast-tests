// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package taskmanager contains functions related to the task manager.
package taskmanager

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// rootFinder is the finder for the Task Manager.
	rootFinder = nodewith.Name("Task Manager").HasClass("TaskManagerView")

	// EndProcessFinder is the finder for End Process button.
	EndProcessFinder = nodewith.Name("End process").Role(role.Button).FinalAncestor(rootFinder)
)

// TaskManager holds the resources required to operate on the Task Manager.
type TaskManager struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
	kb    *input.KeyboardEventWriter
	app   apps.App
}

// New returns an instance of TaskManager.
func New(tconn *chrome.TestConn, kb *input.KeyboardEventWriter) *TaskManager {
	return &TaskManager{
		tconn: tconn,
		ui:    uiauto.New(tconn),
		kb:    kb,
		app:   apps.TaskManager,
	}
}

// Open opens the task manager.
func (tm *TaskManager) Open(ctx context.Context) error {
	if err := tm.kb.Accel(ctx, "Search+Esc"); err != nil {
		return errors.Wrap(err, "failed to press key to open the Task Manager")
	}

	return tm.ui.WaitUntilExists(rootFinder)(ctx)
}

// WaitUntilStable waits task manager ui to become stable.
func (tm *TaskManager) WaitUntilStable(ctx context.Context) error {
	// Process's ui location and state might be unstable if it is originally offscreen or invisible.
	// Maximize task manager window to show as many processes as possible as a workaround.
	w, err := ash.FindOnlyWindow(ctx, tm.tconn, func(w *ash.Window) bool {
		return w.AppID == tm.app.ID
	})
	if err != nil {
		return errors.Wrap(err, "failed to find Task Manager window")
	}
	if err := ash.SetWindowStateAndWait(ctx, tm.tconn, w.ID, ash.WindowStateMaximized); err != nil {
		return errors.Wrap(err, "failed to maximize Task Manager")
	}

	processRows := nodewith.HasClass("AXVirtualView").Role(role.Row).State(state.Multiselectable, true)
	const threshold = 3
	var contentsLoaded int
	var numberOfRows int

	return testing.Poll(ctx, func(ctx context.Context) error {
		processesInfo, err := tm.ui.NodesInfo(ctx, processRows)
		if err != nil {
			return testing.PollBreak(err)
		}

		if numberOfRows == len(processesInfo) {
			contentsLoaded++
			if contentsLoaded >= threshold {
				return nil
			}
			return errors.New("contents have stopped loading but not stabilized yet")
		}

		numberOfRows = len(processesInfo)
		contentsLoaded = 0
		return errors.New("contents have not stopped loading")
	}, &testing.PollOptions{Interval: 3 * time.Second, Timeout: time.Minute})
}

// Close closes the task manager.
func (tm *TaskManager) Close(ctx context.Context, tconn *chrome.TestConn) error {
	return apps.Close(ctx, tconn, tm.app.ID)
}

// FindProcess returns the finder of a process node in the task manager.
func FindProcess() *nodewith.Finder {
	return nodewith.HasClass("AXVirtualView").Role(role.GridCell).Ancestor(rootFinder)
}

// FindNthProcess returns the finder of the nth row and the first column of the process node in the task manager.
func FindNthProcess(ctx context.Context, ui *uiauto.Context, nth int) (*nodewith.Finder, error) {
	columnHeader := nodewith.HasClass("AXVirtualView").Role(role.ColumnHeader).Ancestor(rootFinder)
	columns, err := ui.NodesInfo(ctx, columnHeader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the information of the column header")
	}
	return FindProcess().Nth(len(columns) * nth), nil
}

// SelectProcess selects the specific process in the task manager.
func (tm *TaskManager) SelectProcess(nameInTaskManager string) uiauto.Action {
	return func(ctx context.Context) error {
		var lastFocusedNode *uiauto.NodeInfo
		// The count of times that focused node unchanged.
		count := 0
		// The threshold of focused node unchanged times, used to examine if the taskmanager has scrolled to bottom.
		const threshold = 3

		return testing.Poll(ctx, func(c context.Context) error {
			// Press down key to select process one by one in the task manager list until the target process is focused.
			if err := tm.kb.AccelAction("Down")(ctx); err != nil {
				return testing.PollBreak(err)
			}

			focusedProcess := FindProcess().Focused()
			focusedInfo, err := tm.ui.Info(ctx, focusedProcess)
			if err != nil {
				return errors.Wrap(err, "failed to obtain the information of focused process node")
			}

			if focusedInfo.Name == nameInTaskManager {
				return nil
			}

			if reflect.DeepEqual(focusedInfo, lastFocusedNode) {
				// The taskmanager won't ever be stable, and the focused node might switch back to the previous one automatically.
				// It is necessary to use a threshold to examine if the taskmanager has scrolled to the bottom.
				if count >= threshold {
					return testing.PollBreak(errors.New("scrolled to bottom"))
				}
				count++
			} else {
				count = 0
			}

			lastFocusedNode = focusedInfo
			return errors.New("target is not focused")
		}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 500 * time.Millisecond})
	}
}

// TerminateProcess terminates the process.
func (tm *TaskManager) TerminateProcess(nameInTaskManager string) uiauto.Action {
	return uiauto.Combine("end process",
		tm.ui.LeftClick(FindProcess().First()),
		tm.SelectProcess(nameInTaskManager),
		tm.ui.DoDefault(EndProcessFinder), // The end process button on the lower right of the maximized task manager window might be hidden by the notifications. Use DoDefault to trigger the button.
	)
}

type columnHeader string

const (
	// TaskColumn is the name of the "Task" column.
	TaskColumn columnHeader = "Task"
	// MemoryColumn is the name of the "Memory footprint" column.
	MemoryColumn columnHeader = "Memory footprint"
	// CPUColumn is the name of the "CPU" column.
	CPUColumn columnHeader = "CPU"
	// NetworkColumn is the name of the "Network" column.
	NetworkColumn columnHeader = "Network"
	// ProcessIDColumn is the name of the "Process ID" column.
	ProcessIDColumn columnHeader = "Process ID"
)

// OrderBy orders the processes in the task manager by the given column header.
func (tm *TaskManager) OrderBy(column columnHeader, ascending bool) uiauto.Action {
	columnHeaderFinder := nodewith.Name(string(column)).HasClass("AXVirtualView").Role(role.ColumnHeader).Ancestor(rootFinder)

	order := "ascending"
	if !ascending {
		order = "descending"
	}
	itemsOrderedText := nodewith.Name(fmt.Sprintf("Column %s sorted in %s order.", column, order)).Role(role.StaticText).Ancestor(rootFinder)

	return tm.ui.LeftClickUntil(columnHeaderFinder, tm.ui.Exists(itemsOrderedText))
}

// Reopen closes and reopens the task manager.
func (tm *TaskManager) Reopen(ctx context.Context) error {
	if err := tm.Close(ctx, tm.tconn); err != nil {
		return errors.Wrap(err, "failed to close the task manager")
	}
	if err := ash.WaitForAppClosed(ctx, tm.tconn, tm.app.ID); err != nil {
		return errors.Wrapf(err, "app %s is not closed", tm.app.Name)
	}
	if err := tm.Open(ctx); err != nil {
		return errors.Wrap(err, "failed to re-open the task manager")
	}
	return nil
}
