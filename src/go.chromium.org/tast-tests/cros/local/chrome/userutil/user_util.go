// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package userutil provides functions that help with management of users
package userutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/localstate"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TakingOwnershipTimeout is the maximum amount of time for taking ownership of the device after login.
const TakingOwnershipTimeout = 20 * time.Second

// CreateUser creates a new session with a given name and password, considering extra options. It immediately
// closes the session, so it should be used only for creating new users, not logging in.
func CreateUser(ctx context.Context, username, password string, extraOpts ...chrome.Option) error {
	cr, err := login(ctx, username, password, extraOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to create new user")
	}

	cr.Close(ctx)

	return nil
}

// CreateDeviceOwner creates a user like the CreateUser function, but before closing the session
// it waits until the user becomes device owner.
func CreateDeviceOwner(ctx context.Context, username, password string, extraOpts ...chrome.Option) error {
	cr, err := login(ctx, username, password, extraOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to create new user")
	}
	defer cr.Close(ctx)

	return WaitForOwnership(ctx, cr)
}

// Login creates a new user session using provided credentials. If the user doesn't exist it creates a new one.
// It always keeps the current state, and doesn't support other options.
func Login(ctx context.Context, username, password string) (*chrome.Chrome, error) {
	return login(ctx, username, password, chrome.KeepState())
}

func login(ctx context.Context, username, password string, extraOpts ...chrome.Option) (*chrome.Chrome, error) {
	creds := chrome.Creds{User: username, Pass: password}
	opts := append([]chrome.Option{chrome.FakeLogin(creds)}, extraOpts...)

	return chrome.New(ctx, opts...)
}

// GetKnownEmailsFromLocalState returns a map of users that logged in on the device, based on the LoggedInUsers from the LocalState file.
func GetKnownEmailsFromLocalState() (map[string]bool, error) {
	// Local State is a json-like structure, from which we will need only LoggedInUsers field.
	type LocalState struct {
		Emails []string `json:"LoggedInUsers"`
	}
	var localState LocalState
	if err := localstate.Unmarshal(browser.TypeAsh, &localState); err != nil {
		return nil, errors.Wrap(err, "failed to extract Local State")
	}
	knownEmails := make(map[string]bool)
	for _, email := range localState.Emails {
		knownEmails[email] = true
	}
	return knownEmails, nil
}

// WaitForOwnership waits for up to TakingOwnershipTimeout for the current user to become device owner.
// Normally this should take less then a few seconds.
func WaitForOwnership(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "creating login test API connection failed")
	}

	testing.ContextLog(ctx, "Waiting for the user to become device owner")

	var pollOpts = &testing.PollOptions{Interval: time.Second, Timeout: TakingOwnershipTimeout}
	var status struct {
		IsOwner bool
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.loginStatus)()`, &status); err != nil {
			// This is caused by failure to run javascript to get login status
			// quit polling.
			return testing.PollBreak(err)
		}
		testing.ContextLogf(ctx, "User is owner: %t", status.IsOwner)

		if !status.IsOwner {
			return errors.New("user did not become device owner yet")
		}

		return nil
	}, pollOpts)
}

// RemoveUserOnLoginScreen finds the user pod on login screen and removes it.
func RemoveUserOnLoginScreen(ctx context.Context, tLoginConn *chrome.TestConn, cr *chrome.Chrome, user string) error {
	ui := uiauto.New(tLoginConn)
	// Wait for user pods to be available.
	// Remove user pod by clicking remove button twice.
	// Check that user pod was deleted.
	removeButton := nodewith.Name("Remove account").Role(role.Button)
	confirmRemovalText := nodewith.NameContaining("All files and local data associated with this user will be permanently deleted").Role(role.StaticText)
	if err := uiauto.Combine("Remove user from using their pod",
		ui.WaitUntilExists(nodewith.Name(user).Role(role.Button)),
		ui.LeftClick(nodewith.Name("Open remove dialog for "+user).Role(role.Button)),
		ui.LeftClick(removeButton),
		ui.WaitUntilExists(confirmRemovalText),
		ui.LeftClick(removeButton),
		ui.WaitUntilGone(nodewith.Name(user).Role(role.Button)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to remove user from using their pod")
	}
	return nil
}

// CheckDeviceOwnerIsNotRemoved finds the user pod on login screen
// and checks to see that remove button is not there.
func CheckDeviceOwnerIsNotRemoved(ctx context.Context, tLoginConn *chrome.TestConn, cr *chrome.Chrome, user string) error {
	ui := uiauto.New(tLoginConn)
	// try to delete device owner - it should not be possible
	if err := ui.WaitUntilExists(nodewith.Name(user).Role(role.Button))(ctx); err != nil {
		return errors.Wrap(err, "user pod not available after wait")

	}
	if err := ui.LeftClick(nodewith.Name(user).Role(role.Button))(ctx); err != nil {
		return errors.Wrap(err, "could not click on user pod")
	}

	removeButtonFound, err := ui.IsNodeFound(ctx, nodewith.Name("Open remove dialog for "+user).Role(role.Button))
	if err != nil {
		return errors.Wrap(err, "removed button could not be looked up")
	}
	if removeButtonFound {
		return errors.Wrap(err, "removed user pod is unexpectedly found for device owner")
	}
	return nil
}
