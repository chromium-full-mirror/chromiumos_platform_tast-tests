// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package printpreview provides support for controlling Chrome print preview
// directly through the UI.
package printpreview

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// Layout represents the layout setting in Chrome print preview.
type Layout int

const (
	// Portrait represents the portrait layout setting.
	Portrait Layout = iota
	// Landscape represents the landscape layout setting.
	Landscape
)

// PrintPreviewNode is the node for the top-level dialog.
var PrintPreviewNode *nodewith.Finder = nodewith.Name("Print").Role(role.Window).HasClass("RootView")

// Print sets focus on the print button in Chrome print preview and injects the
// ENTER key to start printing. This is more reliable than clicking the print
// button since notifications often block it from view.
func Print(ctx context.Context, tconn *chrome.TestConn) error {
	printButton := nodewith.Name("Print").Role(role.Button)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("find and focus print button",
		ui.WithTimeout(10*time.Second).WaitUntilExists(printButton),
		ui.WithTimeout(10*time.Second).FocusAndWait(printButton),
	)(ctx); err != nil {
		return err
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the keyboard")
	}
	defer kb.Close(ctx)
	if err := kb.Accel(ctx, "enter"); err != nil {
		return errors.Wrap(err, "failed to type enter")
	}
	return nil
}

// SelectPrinter interacts with Chrome print preview to select the printer with
// the given printerName.
func SelectPrinter(ctx context.Context, tconn *chrome.TestConn, printerName string) error {
	ui := uiauto.New(tconn)

	// Check the new "See more" button first with a timeout.
	seeMoreBtn := nodewith.Name("See more destinations").Role(role.Button)
	if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(seeMoreBtn)(ctx); err == nil {
		if err := ui.LeftClick(seeMoreBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to click See more button")
		}
	} else {
		// If it does not exist, fallback to older UI: find and expand the destination list.
		// The exact name may change based on which printer was previously selected,
		// but it will always start with "Destination ".
		dataList := nodewith.NameStartingWith("Destination ").Role(role.PopUpButton)
		if err := uiauto.Combine("find and click destination list",
			ui.WithTimeout(10*time.Second).WaitUntilExists(dataList),
			ui.LeftClick(dataList),
		)(ctx); err != nil {
			return err
		}

		// Find and click the menu item corresponding to `printerName` if it exists.
		// Note that if it does not exist, the desired printer may be behind the
		// "See more..." menu item, requiring additional clicks on behalf of the user.
		menuItem := nodewith.Name(printerName).Role(role.MenuItem)
		if err := ui.WithTimeout(1 * time.Second).LeftClick(menuItem)(ctx); err == nil {
			return nil
		}

		// Find and click the See more... menu item.
		seeMoreMenuItem := nodewith.Name("See more destinations").Role(role.MenuItem)
		if err := uiauto.Combine("find and click See more menu item",
			ui.WithTimeout(10*time.Second).WaitUntilExists(seeMoreMenuItem),
			ui.LeftClick(seeMoreMenuItem),
		)(ctx); err != nil {
			return err
		}
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return err
	}
	defer kb.Close(ctx)

	// Type the printer name into the destination box to ensure it's visible.
	searchBox := nodewith.Name("Search destinations").Role(role.SearchBox)
	if err := uiauto.Combine("select search box",
		ui.WithTimeout(10*time.Second).WaitUntilExists(searchBox),
		ui.EnsureFocused(searchBox),
	)(ctx); err != nil {
		return err
	}
	if err := kb.Type(ctx, printerName); err != nil {
		return err
	}

	// Find and select the printer.
	// TODO(b/294823934): Consider replacing the regexp with a simple string
	// if/after the default language is the same among all the test devices.
	printerList := nodewith.NameRegex(regexp.MustCompile("Print (d|D)estinations"))
	printer := nodewith.Name(printerName).Role(role.StaticText).Ancestor(printerList).First()
	if err := uiauto.Combine("find and click printer",
		ui.WithTimeout(10*time.Second).WaitUntilExists(printer),
		ui.LeftClickUntil(printer, ui.Gone(printer)),
	)(ctx); err != nil {
		return err
	}
	return nil
}

// SetLayout interacts with Chrome print preview to change the layout setting to
// the provided layout.
func SetLayout(ctx context.Context, tconn *chrome.TestConn, layout Layout) error {
	// Find and expand the layout list.
	layoutList := nodewith.Name("Layout").Role(role.ComboBoxSelect)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("find and click layout list",
		ui.WithTimeout(10*time.Second).WaitUntilExists(layoutList),
		ui.LeftClick(layoutList),
	)(ctx); err != nil {
		return err
	}

	// Find the landscape layout option to verify the layout list has expanded.
	landscapeOption := nodewith.Name("Landscape").Role(role.MenuListOption)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(landscapeOption)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for layout list to expand")
	}

	// Select the desired layout.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the keyboard")
	}
	defer kb.Close(ctx)
	var accelerator string
	switch layout {
	case Portrait:
		accelerator = "search+left"
	case Landscape:
		accelerator = "search+right"
	}
	if err := kb.Accel(ctx, accelerator); err != nil {
		return errors.Wrap(err, "failed to type accelerator")
	}
	if err := kb.Accel(ctx, "enter"); err != nil {
		return errors.Wrap(err, "failed to type enter")
	}
	return nil
}

// SetPages interacts with Chrome print preview to set the selected pages.
func SetPages(ctx context.Context, tconn *chrome.TestConn, pages string) error {
	// Find and expand the pages list.
	pageList := nodewith.Name("Pages").Role(role.ComboBoxSelect)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("find and click page list",
		ui.WithTimeout(10*time.Second).WaitUntilExists(pageList),
		ui.LeftClick(pageList),
	)(ctx); err != nil {
		return err
	}

	// Find the custom pages option to verify the pages list has expanded.
	customOption := nodewith.Name("Custom").Role(role.MenuListOption)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(customOption)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for pages list to expand")
	}

	// Select "Custom" and set the desired page range.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the keyboard")
	}
	defer kb.Close(ctx)
	if err := kb.Accel(ctx, "search+right"); err != nil {
		return errors.Wrap(err, "failed to type end")
	}
	if err := kb.Accel(ctx, "enter"); err != nil {
		return errors.Wrap(err, "failed to type enter")
	}
	// Wait for the custom pages text field to appear and become focused (this
	// happens automatically).
	textField := nodewith.Name("e.g. 1-5, 8, 11-13").Role(role.TextField).State(state.Focused, true)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(textField)(ctx); err != nil {
		return errors.Wrap(err, "failed to find custom pages text field")
	}
	if err := kb.Type(ctx, pages); err != nil {
		return errors.Wrap(err, "failed to type pages")
	}
	return nil
}

// WaitForPrintPreview waits for Print Preview to finish loading after it's
// initially opened.
func WaitForPrintPreview(tconn *chrome.TestConn) uiauto.Action {
	ui := uiauto.New(tconn)
	loadingPreviewText := nodewith.Name("Loading preview")
	printPreviewFailedText := nodewith.Name("Print preview failed")
	destinationButton := nodewith.Name("Destination").Role(role.StaticText).Ancestor(PrintPreviewNode)
	emptyAction := func(context.Context) error { return nil }
	return uiauto.Combine("wait for Print Preview to finish loading",
		uiauto.NamedAction("wait for Print Preview to appear", ui.WithTimeout(10*time.Second).WaitUntilExists(destinationButton)),
		// Wait for the loading text to appear to indicate print preview is loading.
		// Since print preview can finish loading before the loading text is found,
		// IfSuccessThen() is used with a stub "success" action just so that the
		// WaitUntilExists() error is ignored and won't fail the test.
		uiauto.IfSuccessThen(ui.WithTimeout(5*time.Second).WaitUntilExists(loadingPreviewText), emptyAction),
		// Wait for the loading text to be removed to indicate print preview is no
		// longer loading.
		ui.WithTimeout(30*time.Second).WaitUntilGone(loadingPreviewText),
		ui.Gone(printPreviewFailedText),
	)
}

// ExpandMoreSettings expands the the "More settings" section of the print
// settings window. Does nothing if the section is already expanded.
func ExpandMoreSettings(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	moreSettingsButton := nodewith.Name("More settings").Role(role.Button)
	advancedSettingsButton := nodewith.Name("Advanced settings").Role(role.Button)

	// Check whether the "More settings" section is already expanded by
	// checking whether the "Advanced settings" button is reachable. If it's
	// already expanded, return without doing anything.
	if alreadyExpanded, err := ui.IsNodeFound(ctx, advancedSettingsButton); err != nil {
		return err
	} else if alreadyExpanded {
		return nil
	}

	// If the section isn't expanded yet, expand it by left clicking on the
	// "More settings" button.
	if err := uiauto.Combine("find and click more settings button",
		ui.WithTimeout(10*time.Second).WaitUntilExists(moreSettingsButton),
		ui.EnsureFocused(moreSettingsButton),
		ui.WaitForEvent(moreSettingsButton, event.Expanded, ui.DoDefault(moreSettingsButton)),
	)(ctx); err != nil {
		return err
	}

	return nil
}

// setDropdownInternal changes the selected option of a dropdown menu to the
// desired value.
func setDropdownInternal(ui *uiauto.Context, dropdown *nodewith.Finder, value string) uiauto.Action {
	option := nodewith.Name(value).Role(role.MenuListOption).Ancestor(dropdown)

	return uiauto.Combine(fmt.Sprintf("expand dropdown and select option '%s'", value),
		ui.WithTimeout(10*time.Second).WaitUntilExists(dropdown.Focusable()),
		ui.EnsureFocused(dropdown),
		ui.DoDefault(dropdown),

		// Dropdown options can extend past the boundaries of the main window, and
		// ui.LeftClick() won't be able to click on any options that do. So
		// instead, use ui.DoDefault() to select the option. This doesn't close the
		// dropdown, so we need a separate step to re-collapse it and complete the
		// selection.
		ui.WithTimeout(10*time.Second).WaitUntilExists(option),
		ui.DoDefault(option),
		ui.DoDefault(dropdown),
		ui.WithTimeout(10*time.Second).WaitUntilExists(dropdown.Collapsed()),
	)
}

// SetDropdown selects a dropdown menu and changes its selected option to the
// desired value.
func SetDropdown(ctx context.Context, tconn *chrome.TestConn, name, value string) error {
	ui := uiauto.New(tconn)
	dropdown := nodewith.Name(name).Role(role.ComboBoxSelect)

	// Do nothing if the requested value is already selected.
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(dropdown.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "failed to find dropdown")
	}
	info, err := ui.Info(ctx, dropdown)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve dropdown info")
	}
	if info.Value == value {
		return nil
	}

	if err := setDropdownInternal(ui, dropdown, value)(ctx); err != nil {
		return errors.Wrap(err, "failed to select dropdown option")
	}

	return nil
}

// SetCheckboxStateInternal sets the checkbox state to the desired value.
func SetCheckboxStateInternal(ctx context.Context, tconn *chrome.TestConn, checkbox *nodewith.Finder, selected bool) error {
	// This function takes a bool instead of checked.Checked since you can't put
	// a checkbox in "mixed" state by clicking on it.
	var targetState checked.Checked
	if selected {
		targetState = checked.True
	} else {
		targetState = checked.False
	}

	ui := uiauto.New(tconn)

	// The checkbox could be in "mixed" state, so we might have to click it twice.
	for {
		info, err := ui.Info(ctx, checkbox)
		if err != nil {
			return err
		}
		if info.Checked == targetState {
			break
		}
		if err := uiauto.Combine(("toggle checkbox value"),
			ui.WithTimeout(10*time.Second).WaitUntilExists(checkbox.Focusable()),
			ui.EnsureFocused(checkbox),
			ui.WaitForEvent(checkbox, event.CheckedStateChanged, ui.DoDefault(checkbox)),
		)(ctx); err != nil {
			return err
		}
	}

	return nil
}

// SetCheckboxState finds a checkbox node with given name and sets its state to the desired value.
func SetCheckboxState(ctx context.Context, tconn *chrome.TestConn, name string, selected bool) error {
	checkbox := nodewith.Name(name).Role(role.CheckBox)
	return SetCheckboxStateInternal(ctx, tconn, checkbox, selected)
}

// OpenAdvancedSettings opens the advanced settings dialog.
func OpenAdvancedSettings(ctx context.Context, tconn *chrome.TestConn) error {
	// Wait for the "Advanced Settings" button, make the button visible, then
	// click the button until the advanced settings menu has opened. We will know
	// the advanced settings menu has opened when the “Advanced settings for” text
	// exists.
	advancedSettingsButton := nodewith.Name("Advanced settings").Role(role.Button)
	advancedSettingsTitle := nodewith.NameStartingWith("Advanced settings for").Role(role.StaticText)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("open advanced settings dialog",
		ui.WithTimeout(10*time.Second).WaitUntilExists(advancedSettingsButton),
		ui.EnsureFocused(advancedSettingsButton),
		ui.DoDefault(advancedSettingsButton),
		ui.WithTimeout(10*time.Second).WaitUntilExists(advancedSettingsTitle),
	)(ctx); err != nil {
		return err
	}

	return nil
}

// SetAdvancedSetting sets an option in the advanced settings dialog. Expects
// that OpenAdvancedSettings() has already been called.
func SetAdvancedSetting(ctx context.Context, tconn *chrome.TestConn, name string, value interface{}) error {
	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return err
	}

	// The individual dropdown menus in the advanced settings dialog have no
	// distinguishing attributes that the nodewith module can pick up on, so
	// instead use the search feature to isolate the desired dropdown menu.
	printPreviewDialog := nodewith.Name("Print").HasClass("RootView")
	advancedSettingsDialog := nodewith.Role(role.Dialog).Ancestor(printPreviewDialog)
	searchSettings := nodewith.Name("Search settings").Role(role.SearchBox).Ancestor(advancedSettingsDialog)
	clearButton := nodewith.Name("Clear search").Role(role.Button).Ancestor(advancedSettingsDialog)

	if err := uiauto.Combine("type the desired option in the search box",
		// Type in the search box so that the desired setting's dropdown is the only
		// one visible. This will fail if the setting's name is contained in another
		// setting's name.
		ui.WithTimeout(10*time.Second).WaitUntilExists(searchSettings),
		ui.EnsureFocused(searchSettings),
		kb.TypeAction(name))(ctx); err != nil {
		return nil
	}

	switch value := value.(type) {
	case bool:
		checkbox := nodewith.Role(role.CheckBox).Ancestor(advancedSettingsDialog)
		if err := SetCheckboxStateInternal(ctx, tconn, checkbox, value); err != nil {
			return errors.Wrap(err, "failed to set checkbox value")
		}
	case string:
		// Open the dropdown menu and select the desired option.
		dropdown := nodewith.HasClass("md-select").Ancestor(advancedSettingsDialog)
		if err := setDropdownInternal(ui, dropdown, value)(ctx); err != nil {
			return errors.Wrap(err, "failed to select dropdown option")
		}
	default:
		return errors.Errorf("unknown value type %T", value)
	}

	if err := uiauto.Combine("clear the search text",
		// Clear the search text so that this function can be called again.
		ui.WithTimeout(10*time.Second).WaitUntilExists(clearButton),
		ui.DoDefault(clearButton),
	)(ctx); err != nil {
		return err
	}

	return nil
}

// CloseAdvancedSettings closes the advanced settings dialog. Expects that
// OpenAdvancedSettings() has already been called.
func CloseAdvancedSettings(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	printPreviewDialog := nodewith.Name("Print").HasClass("RootView")
	advancedSettingsDialog := nodewith.Role(role.Dialog).Ancestor(printPreviewDialog)
	applyButton := nodewith.Name("Apply").Role(role.Button).Ancestor(advancedSettingsDialog)

	if err := uiauto.Combine("apply advanced settings and close dialog",
		ui.WithTimeout(10*time.Second).WaitUntilExists(applyButton),
		ui.DoDefault(applyButton),
	)(ctx); err != nil {
		return err
	}

	return nil
}
