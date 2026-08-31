// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ash

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SavedDeskType enum distinguishes sved desk type
type SavedDeskType int

const (
	// Template represents desk saved by "Save desk as a template" button
	Template SavedDeskType = iota
	// SaveAndRecall represents desk saved by "Save desk for later" button
	SaveAndRecall
)

// WindowMovementDirection enum describes the movement of windows between desks
// relative to the desk's position on the desks bar.
type WindowMovementDirection string

const (
	// WindowMovementDirectionRight represents moving windows to the desk on the
	// right in the desk bar.
	WindowMovementDirectionRight WindowMovementDirection = "Right"
	// WindowMovementDirectionLeft represents moving windows to the desk on the
	// left in the desk bar.
	WindowMovementDirectionLeft WindowMovementDirection = "Left"
)

// DesksInfo holds overall desks information.
// https://cs.chromium.org/chromium/src/chrome/common/extensions/api/autotest_private.idl
type DesksInfo struct {
	ActiveDeskIndex int      `json:"activeDeskIndex"`
	NumDesks        int      `json:"numDesks"`
	IsAnimating     bool     `json:"isAnimating"`
	DeskContainers  []string `json:"deskContainers"`
}

func isSavedDeskUIRevampEnabled(ctx context.Context, ac *uiauto.Context) bool {
	uiRevampEnabled, err := ac.IsFeatureEnabled(ctx, "SavedDeskUiRevamp")
	if err != nil {
		testing.ContextLog(ctx, "Failed to check if SavedDeskUiRevamp is enabled, assuming true")
		return true
	}
	return uiRevampEnabled
}

func deskButton(ctx context.Context, ac *uiauto.Context) *nodewith.Finder {
	return nodewith.HasClass("DeskIconButton")
}

// NewDeskButton returns a `Finder` for the new desk button.
func NewDeskButton(ctx context.Context, ac *uiauto.Context) *nodewith.Finder {
	return deskButton(ctx, ac).Name("New Desk")
}

// DeskLibraryButton returns the library button from the desks bar.
func DeskLibraryButton(ctx context.Context, ac *uiauto.Context) *nodewith.Finder {
	return deskButton(ctx, ac).Name("Library")
}

// DeskDialog returns a `Finder` for the saved desk dialog where the title
// starts with `name`.
func DeskDialog(ctx context.Context, ac *uiauto.Context, name string) *nodewith.Finder {
	finder := nodewith.HasClass("Label").NameStartingWith(name)
	return finder.Ancestor(nodewith.HasClass("SystemDialogDelegateView").First())
}

// CreateNewDesk requests Ash to create a new Virtual Desk which would fail if
// the maximum number of desks have been reached.
func CreateNewDesk(ctx context.Context, tconn *chrome.TestConn) error {
	success := false
	if err := tconn.Call(ctx, &success, "tast.promisify(chrome.autotestPrivate.createNewDesk)"); err != nil {
		return err
	}
	if !success {
		return errors.New("failed to create a new desk")
	}
	return nil
}

// CleanUpDesks removes all but one desk.
func CleanUpDesks(ctx context.Context, tconn *chrome.TestConn) error {
	// To remove all but one desk, we invoke chrome.autotestPrivate.removeActiveDesk
	// repeatedly until it returns false. It is guaranteed to return true as long as
	// there is more than one desk (see DesksController::CanRemoveDesks in chromium).
	for success := true; success; {
		if err := tconn.Call(ctx, &success, "tast.promisify(chrome.autotestPrivate.removeActiveDesk)"); err != nil {
			return errors.Wrap(err, "failed to remove desk")
		}
		// GoBigSleepLint: It's used as a warm-up step which is part of the
		// performance testing logic. In overview mode, there is no desk removal
		// animation to wait for, and so chrome.autotestPrivate.removeActiveDesk
		// returns immediately, but we still need to allow at least a brief moment
		// for the desk removal to be fully processed. If we just call
		// chrome.autotestPrivate.removeActiveDesk in a tight loop in overview mode,
		// it may repeatedly return true forever. See b/253687177.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			return errors.Wrap(err, "failed to wait a second")
		}
	}
	return nil
}

// ActivateDeskAtIndex requests Ash to activate the Virtual Desk at the given index.
// It waits for the desk-switch animation to complete. This call will fail if index is
// invalid, or its the index of the already active desk.
func ActivateDeskAtIndex(ctx context.Context, tconn *chrome.TestConn, index int) error {
	success := false
	if err := tconn.Call(ctx, &success, "tast.promisify(chrome.autotestPrivate.activateDeskAtIndex)", index); err != nil {
		return err
	}
	if !success {
		return errors.Errorf("failed to activate desk at index %v", index)
	}
	return nil
}

// RemoveActiveDesk requests Ash to remove the currently active desk and waits for the
// desk-removal animation to complete. This call will fail if the currently active desk
// is the last available desk which cannot be removed.
func RemoveActiveDesk(ctx context.Context, tconn *chrome.TestConn) error {
	success := false
	if err := tconn.Call(ctx, &success, "tast.promisify(chrome.autotestPrivate.removeActiveDesk)"); err != nil {
		return err
	}
	if !success {
		return errors.New("failed to remove the active desk")
	}
	return nil
}

// ActivateAdjacentDesksToTargetIndex requests Ash to keep activating the adjacent
// Virtual Desk until the one at the given index is reached. It waits for the chain
// of desk-switch animations to complete. This call will fail if index is invalid,
// or it is the index of the already active desk.
func ActivateAdjacentDesksToTargetIndex(ctx context.Context, tconn *chrome.TestConn, index int) error {
	success := false
	if err := tconn.Call(ctx, &success, "tast.promisify(chrome.autotestPrivate.activateAdjacentDesksToTargetIndex)",
		index); err != nil {
		return err
	}
	if !success {
		return errors.Errorf("failed to activate desk at index %v", index)
	}
	return nil
}

// GetDeskCount asks Ash's DesksController for information about the
// open desks. This call will fail if the returned desk count is less
// than 1, which should never happen.
func GetDeskCount(ctx context.Context, tconn *chrome.TestConn) (int, error) {
	desks, err := GetDesksInfo(ctx, tconn)
	if err != nil {
		return 0, err
	}
	if desks.NumDesks < 1 {
		return desks.NumDesks, errors.Errorf("unexpected desk count of %d, there should always be at least 1 desk", desks.NumDesks)
	}
	return desks.NumDesks, nil
}

// GetDesksInfo retrieves desks information defined in DesksInfo
// from the autotestPrivate API.
func GetDesksInfo(ctx context.Context, tconn *chrome.TestConn) (DesksInfo, error) {
	var desks DesksInfo
	err := tconn.Call(ctx, &desks, "tast.promisify(chrome.autotestPrivate.getDesksInfo)")
	return desks, err
}

// GetSavedDeskIndexForName returns the index of the saved desk for savedDeskName.
func GetSavedDeskIndexForName(ctx context.Context, ac *uiauto.Context, savedDeskName string) (int, error) {
	savedDeskNameView := nodewith.ClassName("SavedDeskNameView")
	savedDeskNameViewInfo, err := ac.NodesInfo(ctx, savedDeskNameView)
	if err != nil {
		return -1, errors.Wrap(err, "failed to find saved desks")
	}

	for i, info := range savedDeskNameViewInfo {
		if info.Value == savedDeskName {
			return i, nil
		}
	}
	return -1, errors.Errorf("failed to find index for saved desk with name %s", savedDeskName)
}

// FindDeskMiniViews returns a list of DeskMiniView nodes.
func FindDeskMiniViews(ctx context.Context, ac *uiauto.Context) ([]uiauto.NodeInfo, error) {
	deskMiniViews := nodewith.ClassName("DeskMiniView")
	deskMiniViewsInfo, err := ac.NodesInfo(ctx, deskMiniViews)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find all desk mini views")
	}
	return deskMiniViewsInfo, nil
}

// MoveActiveWindowToAdjacentDesk moves the active window at the time of call
// to the right or the left desk (as indicated by `direction`) and then waits
// for the window movement animation to complete.
func MoveActiveWindowToAdjacentDesk(ctx context.Context, tconn *chrome.TestConn, direction WindowMovementDirection) error {
	// If we are moving windows to the left, then we will want to use the left
	// bracket "[" in our shortcut call, otherwise if we are moving right we will
	// use the right bracket "]" instead.
	bracket := "["

	if direction == WindowMovementDirectionRight {
		bracket = "]"
	}

	// We save the active window to a variable here so that we can access its ID
	// to wait for its animation when it is being moved.
	activeWindow, err := GetActiveWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get active window")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create keyboard")
	}
	defer kb.Close(ctx)

	if err := kb.Accel(ctx, "Search+Shift+"+bracket); err != nil {
		return errors.Wrapf(err, "failed to move active window to %s desk", direction)
	}

	return WaitForCondition(ctx, tconn, func(window *Window) bool {
		return window.ID == activeWindow.ID && !window.OnActiveDesk && !window.IsAnimating
	}, defaultPollOptions)
}

// SaveCurrentDesk saves the current desk as `kTemplate` or `kSaveAndRecall`.
// This assumes overview grid is live now.
// TODO(yongshun): Check if in overview mode and error out early if not.
func SaveCurrentDesk(ctx context.Context, ac *uiauto.Context, savedDeskType SavedDeskType, savedDeskName string) error {
	var saveDeskButton *nodewith.Finder
	var savedDeskGridView *nodewith.Finder
	defaultDeskButton := nodewith.ClassName("DefaultDeskButton")
	savedDeskUIRevampEnabled := isSavedDeskUIRevampEnabled(ctx, ac)
	if savedDeskUIRevampEnabled {
		deskMiniView := nodewith.ClassName("DeskMiniView")
		deskPreviewView := nodewith.ClassName("DeskPreviewView").NameRegex(regexp.MustCompile(("(?i).* Active desk\\.")))
		deskActionButton := nodewith.ClassName("DeskActionButton").Name("Open context menu")

		// Navigate to the save desk buttons.
		if err := uiauto.Combine(
			"navigate to the save desk buttons",
			ac.DoDefault(defaultDeskButton),
			ac.WaitUntilExists(deskMiniView),
			ac.MouseMoveTo(deskPreviewView, 0),
			ac.WaitUntilExists(deskActionButton),
			ac.DoDefault(deskActionButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to navigate to save desk buttons")
		}
	}

	if savedDeskType == Template {
		if savedDeskUIRevampEnabled {
			saveDeskButton = nodewith.ClassName("MenuItemView").Name("Save desk as a template")
		} else {
			saveDeskButton = nodewith.ClassName("SavedDeskSaveDeskButton").Nth(0)
		}
		savedDeskGridView = nodewith.ClassName("SavedDeskGridView").Nth(0)

	} else if savedDeskType == SaveAndRecall {
		if savedDeskUIRevampEnabled {
			saveDeskButton = nodewith.ClassName("MenuItemView").Name("Save desk for later")
		} else {
			saveDeskButton = nodewith.ClassName("SavedDeskSaveDeskButton").Nth(1)
		}
		savedDeskGridView = nodewith.ClassName("SavedDeskGridView").Nth(1)

	} else {
		return errors.New("unknown savedDeskType, must be `kTemplate' or 'kSaveAndRecall'")
	}

	focusedNameView := nodewith.ClassName("SavedDeskNameView").Ancestor(savedDeskGridView).Focused()

	// Save a desk.
	if err := uiauto.Combine(
		"save a desk",
		ac.DoDefault(saveDeskButton),
		// Wait for the saved desk grid to show up.
		ac.WaitUntilExists(savedDeskGridView),
		// Wait for the name view of the newly added item to be focused.
		ac.WaitUntilExists(focusedNameView),
		// Wait for the animation of the saved desk grid item to finish.
		ac.WaitForLocation(focusedNameView),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to save a desk")
	}

	// Type savedDeskName and press "Enter".
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot create keyboard")
	}
	defer kb.Close(ctx)

	// Set a delay of 50ms between keystrokes to prevent dropping characters.
	// Certain UI fields may drop rapid keystrokes if the view is still settling
	// or performing layout updates.
	kb.AdditionDelay10msIncrementCounter = 5

	if err := kb.Type(ctx, savedDeskName); err != nil {
		return errors.Wrapf(err, "cannot type %q: ", savedDeskName)
	}

	// Verify that the text field's value has been fully and correctly updated.
	// This helps catch partial or dropped keystrokes before pressing "Enter".
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		info, err := ac.Info(ctx, focusedNameView)
		if err != nil {
			return err
		}
		if info.Value != savedDeskName {
			return errors.Errorf("got %q, want %q", info.Value, savedDeskName)
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Second, Interval: 100 * time.Millisecond}); err != nil {
		return errors.Wrapf(err, "saved desk name not correctly fully typed: %q", savedDeskName)
	}

	if err := kb.Accel(ctx, "Enter"); err != nil {
		return errors.Wrap(err, "cannot press 'Enter'")
	}
	return nil
}

// WaitForSavedDeskSync waits 30 seconds for the saved desk sync.
// This assumes overview grid is live now.
// TODO(yongshun): Check if in overview mode and error out early if not.
func WaitForSavedDeskSync(ctx context.Context, ac *uiauto.Context) {
	libraryButton := nodewith.Name("Library")
	ac.WithTimeout(30 * time.Second).WaitUntilExists(libraryButton)(ctx)
}

// IsLibraryButtonVisible returns if the library button is visible.
// This assumes overview grid is live now.
// TODO(yongshun): Check if in overview mode and error out early if not.
func IsLibraryButtonVisible(ctx context.Context, ac *uiauto.Context) (bool, error) {
	libraryButton := nodewith.Name("Library")

	libraryButtonInfo, err := ac.NodesInfo(ctx, libraryButton)
	if err != nil {
		return false, errors.Wrap(err, "failed to try to find the library button")
	}

	return len(libraryButtonInfo) != 0, nil
}

// EnterLibraryPage enters the library page from desk bar.
// This assumes overview grid is live now.
// TODO(yongshun): Check if in overview mode and error out early if not.
func EnterLibraryPage(ctx context.Context, ac *uiauto.Context) error {
	libraryButton := DeskLibraryButton(ctx, ac)
	savedDeskGridView := nodewith.ClassName("SavedDeskGridView").Nth(0)

	var visible bool
	var err error
	if visible, err = IsLibraryButtonVisible(ctx, ac); err != nil {
		return errors.Wrap(err, "failed to check if library button is visible")
	}
	if !visible {
		return errors.Wrap(err, "library button is not visible")
	}

	// Show the saved desk grid.
	if err = uiauto.Combine(
		"show the saved desk grid",
		ac.DoDefault(libraryButton),
		// Wait for the saved desk grid to show up.
		ac.WaitUntilExists(savedDeskGridView),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to show the saved desk grid")
	}

	return nil
}

// FindSavedDesks returns a list of saved desk nodes.
// This assumes library page is live now.
func FindSavedDesks(ctx context.Context, ac *uiauto.Context) ([]uiauto.NodeInfo, error) {
	savedDeskItemView := nodewith.ClassName("SavedDeskItemView")
	savedDeskItemViewInfo, err := ac.NodesInfo(ctx, savedDeskItemView)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find SavedDeskItemView")
	}
	return savedDeskItemViewInfo, nil
}

// LaunchSavedDesk verifies the existence of a saved desk then launches the saved desk of index.
// This assumes library page is live now.
func LaunchSavedDesk(ctx context.Context, ac *uiauto.Context, savedDeskName string, index int) error {
	savedDesk := nodewith.ClassName("SavedDeskItemView").Nth(index)
	savedDeskMiniView :=
		nodewith.ClassName("DeskMiniView").Name(fmt.Sprintf("Desk: %s", savedDeskName))

	savedDeskIndex, err := GetSavedDeskIndexForName(ctx, ac, savedDeskName)
	if err != nil {
		return errors.Wrap(err, "failed to find saved desks")
	}

	savedDeskNameView := nodewith.ClassName("SavedDeskItemView").Nth(savedDeskIndex)

	// Launch the saved desk.
	if err := uiauto.Combine(
		"launch the saved desk",
		// Verify the existence of the saved desk.
		ac.WaitUntilExists(savedDeskNameView),
		ac.DoDefault(savedDesk),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to launch a saved desk")
	}

	// Press enter.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot create keyboard")
	}
	defer kb.Close(ctx)
	if err := kb.Accel(ctx, "Enter"); err != nil {
		return errors.Wrap(err, "cannot press 'Enter'")
	}

	// Wait for the new desk to appear.
	if err := uiauto.Combine(
		"wait for the saved desk",
		ac.WaitUntilExists(savedDeskMiniView),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to launch a saved desk")
	}

	return nil
}

// VerifySavedDesk verifies that the saved desks are expected as the given savedDeskNames.
// This assumes library page is live now.
func VerifySavedDesk(ctx context.Context, ac *uiauto.Context, savedDeskNames []string) error {
	savedDeskNameView := nodewith.ClassName("SavedDeskNameView")

	// Verify the saved desk count and name.
	savedDeskNameViewInfo, err := ac.NodesInfo(ctx, savedDeskNameView)
	if err != nil {
		return errors.Wrap(err, "failed to find SavedDeskNameView")
	}
	if len(savedDeskNameViewInfo) != len(savedDeskNames) {
		return errors.Wrapf(err, "found inconsistent number of saved desk(s): got %v, want %v", len(savedDeskNameViewInfo), len(savedDeskNames))
	}
	for i, info := range savedDeskNameViewInfo {
		if info.Value != savedDeskNames[i] {
			return errors.Wrapf(err, "found inconsistent saved desk name at index %v: got %s, want %s", i, info.Value, savedDeskNames[i])
		}
	}

	return nil
}

// DeleteAllSavedDesks cleans up saved desks by using a series of mouse and keyboard events to delete each desk.
// This assumes library page is live now.
func DeleteAllSavedDesks(ctx context.Context, ac *uiauto.Context, tconn *chrome.TestConn) error {
	savedDesk := nodewith.ClassName("SavedDeskItemView")
	closeButton := nodewith.ClassName("IconButton").Name("Delete")
	deleteDialog := DeskDialog(ctx, ac, "Delete")

	// Define keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed create keyboard")
	}
	defer kb.Close(ctx)

	// Find all saved desks.
	savedDeskInfo, err := FindSavedDesks(ctx, ac)
	if err != nil {
		return errors.Wrap(err, "failed to find saved desks")
	}

	// Delete all saved desks one by one.
	for i := range savedDeskInfo {
		firstSavedDesk := savedDesk.First()
		firstCloseButton := closeButton.First()
		if err := ac.MouseMoveTo(firstSavedDesk, 0)(ctx); err != nil {
			return errors.Wrapf(err, "failed to mouse over to saved desk at position %v", i+1)
		}
		if err := uiauto.Combine(
			"Delete saved desks",
			ac.WaitUntilExists(firstCloseButton),
			ac.DoDefault(firstCloseButton),
			ac.WaitUntilExists(deleteDialog),
			kb.AccelAction("Enter"),
		)(ctx); err != nil {
			return errors.Wrapf(err, "fail to delete saved desk at position %v", i+1)
		}
	}

	return nil
}

// DeleteDeskTemplateByName deletes desk template with savedDeskName.
func DeleteDeskTemplateByName(ctx context.Context, ac *uiauto.Context, tconn *chrome.TestConn, savedDeskName string) error {
	savedDeskIndex, err := GetSavedDeskIndexForName(ctx, ac, savedDeskName)
	if err != nil {
		return errors.Wrap(err, "failed to find saved desks")
	}

	savedDesk := nodewith.ClassName("SavedDeskNameView").Nth(savedDeskIndex)
	closeButton := nodewith.ClassName("IconButton").Name("Delete").Nth(savedDeskIndex)
	deleteDialog := DeskDialog(ctx, ac, "Delete")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed create keyboard")
	}
	defer kb.Close(ctx)

	// Delete the save and recall desk template.
	if err := ac.MouseMoveTo(savedDesk, 0)(ctx); err != nil {
		return errors.Wrap(err, "failed to mouse over to designated desk")
	}
	if err := uiauto.Combine(
		"Delete desk template by name",
		ac.WaitUntilExists(closeButton),
		ac.DoDefault(closeButton),
		ac.WaitUntilExists(deleteDialog),
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		return errors.Wrap(err, "fail to desk template by its name")
	}

	return nil
}

// ExitAndReenterLibrary exits and reenters the library view.
func ExitAndReenterLibrary(ctx context.Context, ac *uiauto.Context, tconn *chrome.TestConn) error {
	if err := SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		return errors.Wrap(err, "failed to exit overview mode")
	}
	if err := SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		return errors.Wrap(err, "failed to enter overview mode")
	}
	if err := EnterLibraryPage(ctx, ac); err != nil {
		return errors.Wrap(err, "failed to enter library page")
	}

	return nil
}

// WaitUntilDesksFinishAnimating waits for any desks animations to
// finish animating on the Chrome side.
func WaitUntilDesksFinishAnimating(ctx context.Context, tconn *chrome.TestConn) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		desksInfo, err := GetDesksInfo(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get desks info"))
		}
		if !desksInfo.IsAnimating {
			return nil
		}
		return errors.New("desks are still animating")
	}, defaultPollOptions)
}

// WaitForDesk waits until a given desk is active and animations have stopped.
func WaitForDesk(tconn *chrome.TestConn, deskIndex int) uiauto.Action {
	return func(ctx context.Context) error {
		start := time.Now()
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			di, err := GetDesksInfo(ctx, tconn)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to get desks info"))
			}
			if di.ActiveDeskIndex != deskIndex || di.IsAnimating {
				elapsed := time.Since(start)
				return errors.Errorf("At desk=%d, animating=%t. Waiting for desk=%d and no animation (elapsed %s)", di.ActiveDeskIndex, di.IsAnimating, deskIndex, elapsed)
			}
			return nil
		}, defaultPollOptions); err != nil {
			return err
		}
		return nil
	}
}
