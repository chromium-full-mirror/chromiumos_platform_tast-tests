// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// TODO: Refactor the file name. b:228780486

// Package shimlessrmaapp contains drivers for controlling the ui of Shimless RMA SWA.
package shimlessrmaapp

import (
	"context"
	"os"
	"os/user"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var rootNode = nodewith.Name(apps.ShimlessRMA.Name).Role(role.Window)

var nextButton = nodewith.Name("Next >").Role(role.Button)
var cancelButton = nodewith.Name("Cancel").Role(role.Button)

const pollingInterval = 2 * time.Second
const pollingTimeout = 15 * time.Second
const waitUITimeout = 20 * time.Second

// Titles of pages.
// role.Heading nodes with this text are found to confirm a page loaded.
const (
	welcomePageTitle  = "Chromebook repair"
	updateOSPageTitle = "Make sure Chrome OS is up to date"
)

const (
	stateFile = "/mnt/stateful_partition/unencrypted/rma-data/state"
	testFile  = "/var/lib/rmad/.test"
)

// RMAApp represents an instance of the Shimless RMA App.
type RMAApp struct {
	ui    *uiauto.Context
	tconn *chrome.TestConn
	// TODO(gavinwill): launched and all support for running the app manually
	// should be removed once the app launch at boot cls land.
	launched bool
}

// CreateEmptyStateFile creates a valid empty state file.
func CreateEmptyStateFile() error {
	return CreateStateFile("{}\n")
}

// CreateStateFile creates a state file with contents |state|.
func CreateStateFile(state string) error {
	uid, err := getRmadUID()
	if err != nil {
		return errors.Wrap(err, "failed to get rmad UID")
	}

	// Deletes the state file, if it exists.
	RemoveStateFile()

	f, err := os.Create(stateFile)
	if err != nil {
		return errors.Wrapf(err, "failed to create state file %q", stateFile)
	}
	defer f.Close()
	l, err := f.WriteString(state)
	if err != nil {
		return errors.Wrapf(err, "failed to write state to file %q", stateFile)
	}
	if l < 3 {
		return errors.Errorf("State written to file %q is too short: expected at least 3 bytes, got %d", stateFile, l)
	}
	if err := f.Chown(uid, -1); err != nil {
		return errors.Wrapf(err, "failed to change ownership of state file %q to UID %d", stateFile, uid)
	}
	return nil
}

// RemoveStateFile deletes the state file, if it exists.
func RemoveStateFile() error {
	return os.Remove(stateFile)
}

// CreateTestFile creates an empty test file telling Shimless RMA to run in testing mode.
func CreateTestFile() error {
	f, err := os.Create(testFile)
	if err != nil {
		return errors.Wrapf(err, "failed to create test file %q", testFile)
	}
	defer f.Close()
	return nil
}

// RemoveTestFile deletes the test file, if it exists.
func RemoveTestFile() error {
	return os.Remove(testFile)
}

// Launch launches the Shimless RMA App and returns it.
// An error is returned if the app fails to launch.
// TODO(gavinwill): This method and all support for running the app manually
// should be removed once the app launch at boot cls land.
func Launch(ctx context.Context, tconn *chrome.TestConn) (*RMAApp, error) {
	// Launch the Shimless RMA App.
	if err := apps.Launch(ctx, tconn, apps.ShimlessRMA.ID); err != nil {
		return nil, errors.Wrap(err, "failed to launch Shimless RMA app")
	}
	r, err := App(ctx, tconn)
	if err != nil {
		return r, errors.Wrap(err, "failed to get app instance after launch")
	}
	// Find the main Shimless RMA window
	if err := r.ui.WithTimeout(waitUITimeout).WaitUntilExists(rootNode)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find Shimless RMA window after launch")
	}
	r.launched = true
	return r, nil
}

// App returns an existing instance of the Shimless RMA app.
// An error is returned if the app cannot be found.
func App(ctx context.Context, tconn *chrome.TestConn) (*RMAApp, error) {
	// Create a uiauto.Context with default timeout.
	ui := uiauto.New(tconn)
	return &RMAApp{tconn: tconn, ui: ui, launched: false}, nil
}

// Close closes the Shimless RMA App.
func (r *RMAApp) Close(ctx context.Context) error {
	// Close the Shimless RMA App.
	if err := apps.Close(ctx, r.tconn, apps.ShimlessRMA.ID); err != nil {
		return errors.Wrap(err, "failed to close Shimless RMA app")
	}

	// Wait for window to close.
	if err := r.ui.WithTimeout(time.Minute).WaitUntilGone(rootNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for Shimless RMA window to close")
	}
	return nil
}

// WaitForStateFileDeleted returns a function that waits for the state file to be deleted.
func (r *RMAApp) WaitForStateFileDeleted() uiauto.Action {
	return r.waitForFileDeleted(stateFile)
}

// WaitForWelcomePageToLoad returns a function that waits for the Welcome state page to load.
func (r *RMAApp) WaitForWelcomePageToLoad() uiauto.Action {
	return r.WaitForPageToLoad(welcomePageTitle, waitUITimeout)
}

// WaitForUpdateOSPageToLoad returns a function that waits for the Update OS state page to load.
func (r *RMAApp) WaitForUpdateOSPageToLoad() uiauto.Action {
	return r.WaitForPageToLoad(updateOSPageTitle, waitUITimeout)
}

// WaitForPageToLoad returns a function that waits for the a page with title |pageTitle| to load.
func (r *RMAApp) WaitForPageToLoad(pageTitle string, timeout time.Duration) uiauto.Action {
	title := nodewith.NameContaining(pageTitle).Role(role.Heading)
	return r.ui.WithTimeout(timeout).WaitUntilExists(title)
}

// LeftClickNextButton returns a function that clicks the next button.
func (r *RMAApp) LeftClickNextButton() uiauto.Action {
	return r.leftClickButton(nextButton.Visible())
}

// LeftClickCancelButton returns a function that clicks the cancel button.
func (r *RMAApp) LeftClickCancelButton() uiauto.Action {
	return r.leftClickButton(cancelButton.Visible())
}

// LeftClickButton returns a function that clicks a button.
func (r *RMAApp) LeftClickButton(label string) uiauto.Action {
	return r.leftClickButton(nodewith.NameContaining(label).Role(role.Button).Visible())
}

// LeftClickToggleButton returns a function that clicks a button.
func (r *RMAApp) LeftClickToggleButton(label string) uiauto.Action {
	return r.leftClickButton(nodewith.NameContaining(label).Role(role.ToggleButton).Visible())
}

// WaitUntilButtonEnabled returns a function that waits |timeout| for a button to be enabled.
func (r *RMAApp) WaitUntilButtonEnabled(label string, timeout time.Duration) uiauto.Action {
	return r.waitUntilEnabled(nodewith.NameContaining(label).Role(role.Button).Visible(), timeout)
}

// LeftClickRadioButton returns a function that clicks a radio button.
func (r *RMAApp) LeftClickRadioButton(label string) uiauto.Action {
	// TODO(b/230692945): Can we add RadioButton as role?
	radioGroup := nodewith.Role(role.RadioGroup)
	return r.ui.LeftClick(nodewith.NameContaining(label).Ancestor(radioGroup).First())
}

// LeftClickLink returns a function that clicks a link.
func (r *RMAApp) LeftClickLink(label string) uiauto.Action {
	return r.ui.LeftClick(nodewith.NameContaining(label).Role(role.Link).Visible())
}

// RetrieveTextByPrefix returns a text which has a certain prefix.
func (r *RMAApp) RetrieveTextByPrefix(ctx context.Context, prefix string) (*uiauto.NodeInfo, error) {
	node, err := r.ui.Info(ctx, nodewith.NameStartingWith(prefix).Role(role.StaticText))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve text node with prefix %q", prefix)
	}
	return node, nil
}

// EnterIntoTextInput enters text into text input.
func (r *RMAApp) EnterIntoTextInput(textInputName, content string) uiauto.Action {
	var textInputFinder = nodewith.Role(role.TextField).Name(textInputName)

	return uiauto.Combine("type keyword to enter content to text input",
		r.ui.LeftClickUntil(textInputFinder, r.ui.WaitUntilExists(textInputFinder.Focused())),
		func(ctx context.Context) error {
			keyboard, err := input.Keyboard(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get keyboard")
			}
			defer keyboard.Close(ctx)

			return keyboard.Type(ctx, content)
		},
	)
}

// LeftClickGenericContainer returns a function that clicks a Generic Container.
func (r *RMAApp) LeftClickGenericContainer(label string) uiauto.Action {
	return r.ui.LeftClick(nodewith.NameContaining(label).Role(role.GenericContainer).Visible().First())
}

func getRmadUID() (int, error) {
	u, err := user.Lookup("rmad")
	if err != nil {
		return -1, errors.Wrap(err, "failed to lookup user 'rmad'")
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to convert rmad UID %q to integer", u.Uid)
	}
	return uid, nil
}

func (r *RMAApp) waitUntilEnabled(button *nodewith.Finder, timeout time.Duration) uiauto.Action {
	if r.launched {
		button = button.Ancestor(rootNode)
	}
	return uiauto.Combine("waiting for enabled button",
		r.ui.WithTimeout(waitUITimeout).WaitUntilExists(button),
		func(ctx context.Context) error {
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := r.ui.CheckRestriction(button, restriction.Disabled)(ctx); err == nil {
					return errors.Errorf("Button is still in %s state", restriction.Disabled)
				}
				return nil
			}, &testing.PollOptions{Timeout: timeout, Interval: pollingInterval}); err != nil {
				return errors.Wrap(err, "Button failed to become enabled within timeout")
			}
			return nil
		})
}

func (r *RMAApp) leftClickButton(button *nodewith.Finder) uiauto.Action {
	if r.launched {
		button = button.Ancestor(rootNode)
	}
	return uiauto.Combine("waiting to trigger left click on button",
		r.ui.WithTimeout(waitUITimeout).WaitUntilExists(button),
		r.ui.FocusAndWait(button),
		func(ctx context.Context) error {
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := r.ui.CheckRestriction(button, restriction.Disabled)(ctx); err == nil {
					return errors.Errorf("Button is still in %s state", restriction.Disabled)
				}
				return nil
			}, &testing.PollOptions{Timeout: pollingTimeout, Interval: pollingInterval}); err != nil {
				return errors.Wrap(err, "Button failed to become enabled within timeout")
			}
			return nil
		},
		// TODO(b/371463651): Click the button with mouse event after b/371463651 is resolved.
		func(ctx context.Context) error {
			// Keyboard to input key inputs.
			keyboard, err := input.Keyboard(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get keyboard for button click")
			}
			defer keyboard.Close(ctx)

			return keyboard.Accel(ctx, "Enter")
		},
	)
}

func (r *RMAApp) waitForFileDeleted(fileName string) uiauto.Action {
	return func(ctx context.Context) error {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := os.Stat(fileName); err == nil {
				return errors.Errorf("File %s was not deleted; it still exists", fileName)
			} else if !os.IsNotExist(err) {
				return errors.Wrapf(err, "error checking file %q existence", fileName)
			}
			return nil
		}, &testing.PollOptions{Timeout: pollingTimeout, Interval: pollingInterval}); err != nil {
			return errors.Wrapf(err, "File %q was not deleted within timeout", fileName)
		}
		return nil
	}
}

// SetDropdown selects a dropdown menu and changes its selected option to the
// desired value.
func (r *RMAApp) SetDropdown(name, value string) uiauto.Action {
	dropdown := nodewith.Name(name).Role(role.ComboBoxSelect)
	option := nodewith.Name(value).Role(role.MenuListOption)

	return uiauto.Combine("expand dropdown and select option",
		r.ui.DoDefaultUntil(dropdown, r.ui.WithTimeout(10*time.Second).WaitUntilExists(option)),
		r.ui.DoDefault(option),
		r.ui.DoDefault(dropdown),
	)
}
