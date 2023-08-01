// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prompts

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Prompt defines the structure of a typical prompt dialog.
type Prompt struct {
	Name              string           // Name of the prompt.
	PromptFinder      *nodewith.Finder // The node finder to locate prompt.
	ClearButtonFinder *nodewith.Finder // The button finder to dismiss prompt.
}

var (
	showNotificationsPromptFinder = nodewith.NameContaining("Show notifications").HasClass("RootView").Role(role.AlertDialog)
	avPermPromptFinder            = nodewith.NameRegex(regexp.MustCompile(".*Use your (microphone|camera).*")).HasClass("RootView").Role(role.AlertDialog).First()
	avMeetPermPromptFinder        = nodewith.NameContaining("hear you in the meeting").Role(role.Dialog).First()
	leaveSitePromptFinder         = nodewith.Name("Leave site?").ClassName("RootView").Role(role.Dialog).First()
)

// General dismiss button finders for prompts.
var (
	// There are multiple buttons that contain the "Allow" string.
	// To distinguish multiple buttons, the regular expression must contain
	// all possible strings and NameContaining("Allow") can't be used.
	AllowAVButtonRe     = regexp.MustCompile("^(Allow|Allow this time|Allow microphone and camera|Allow microphone)$")
	AllowAVButtonFinder = nodewith.NameRegex(AllowAVButtonRe).Role(role.Button)
	AllowButtonFinder   = nodewith.Name("Allow").Role(role.Button)
	DismissButtonFinder = nodewith.Name("Dismiss").Role(role.Button)
	CloseButtonFinder   = nodewith.Name("Close").Role(role.Button)
	GotItButtonFinder   = nodewith.Name("Got it").Role(role.Button)
	LeaveButtonFinder   = nodewith.Name("Leave").Role(role.Button)
)

// ShowNotificationsPrompt represents the browser prompt to request permission for allowing notification.
var ShowNotificationsPrompt = Prompt{
	Name:              "show notifications",
	PromptFinder:      showNotificationsPromptFinder,
	ClearButtonFinder: AllowButtonFinder.Ancestor(showNotificationsPromptFinder),
}

// AllowAVPermissionPrompt represents the browser prompt to request permission for audio/video.
var AllowAVPermissionPrompt = Prompt{
	Name:              "allow microphone and camera",
	PromptFinder:      avPermPromptFinder,
	ClearButtonFinder: AllowAVButtonFinder.Ancestor(avPermPromptFinder),
}

// AllowMicrophoneAndCameraPermissionPrompt represents the browser prompt to request permission for microphone and camera.
var AllowMicrophoneAndCameraPermissionPrompt = Prompt{
	Name:         "do you want people to see and hear you in the meeting?",
	PromptFinder: avMeetPermPromptFinder,
	// There are two types of dialogs that should be closed.
	// Possible ancestors are |avPermPromptFinder| and |avMeetPermPromptFinder|.
	// So use First() instead of Ancestor().
	ClearButtonFinder: AllowAVButtonFinder.First(),
}

// LeaveSitePrompt represents the browser prompt to request permission for leaving site.
var LeaveSitePrompt = Prompt{
	Name:              "leave site",
	PromptFinder:      leaveSitePromptFinder,
	ClearButtonFinder: LeaveButtonFinder.Ancestor(leaveSitePromptFinder),
}

// ClearPotentialPrompts clears one or more potential prompts disorderly.
// It assumes no prompts shown up once idle time reached.
func ClearPotentialPrompts(tconn *chrome.TestConn, idleDuration time.Duration, promptCandidates ...Prompt) action.Action {
	ui := uiauto.New(tconn)
	// Find any existing prompt in certain time and returns the index. Returns -1 if no prompt found.
	findPrompt := func(ctx context.Context, candidates []Prompt) (int, error) {
		if len(candidates) == 0 {
			testing.ContextLog(ctx, "No more prompts to be cleared")
			return -1, nil
		}

		var promptFinders = []*nodewith.Finder{}
		for _, prompt := range promptCandidates {
			promptFinders = append(promptFinders, prompt.PromptFinder)
		}

		foundPromptFinder, err := ui.WithTimeout(idleDuration).FindAnyExists(ctx, promptFinders...)
		if err != nil {
			// Return if no prompt found.
			if strings.Contains(err.Error(), nodewith.ErrNotFound) || strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
				return -1, nil
			}
			return -1, err
		}

		for i, candidate := range candidates {
			if foundPromptFinder == candidate.PromptFinder {
				return i, nil
			}
		}
		return -1, errors.Errorf("failed to match prompt finder %v", foundPromptFinder)
	}

	// Return only if no prompt found.
	return func(ctx context.Context) error {
		tmpCandidates := promptCandidates
		for {
			i, err := findPrompt(ctx, tmpCandidates)
			if err != nil {
				return errors.Wrap(err, "failed to find prompt")
			}
			if i < 0 {
				return nil
			}

			testing.ContextLogf(ctx, "Clearing prompt %q", tmpCandidates[i].Name)
			if err := DismissPrompt(ui, tmpCandidates[i])(ctx); err != nil {
				return errors.Wrapf(err, "failed to clear %q", tmpCandidates[i].Name)
			}
			tmpCandidates = append(tmpCandidates[:i], tmpCandidates[i+1:]...)
		}
	}
}

// DismissPrompt waits for a prompt shown up and dismiss it.
func DismissPrompt(ui *uiauto.Context, prompt Prompt) action.Action {
	return ui.DoDefaultUntil(
		prompt.ClearButtonFinder,
		ui.WithTimeout(3*time.Second).WaitUntilGone(prompt.PromptFinder))
}

// ChromePermissionPrompts returns the a list of potential prompts of Chrome permissions.
// It checks the permission status first and only returns the applicable prompts.
func ChromePermissionPrompts(ctx context.Context, conn *chrome.Conn, permissionNames ...webutil.PermissionName) ([]Prompt, error) {
	var permissionPrompts = []Prompt{}

	for _, permissionName := range permissionNames {
		var prompt Prompt
		switch permissionName {
		// Camera and Microphone use same prompt.
		case webutil.PermissionCamera, webutil.PermissionMicrophone:
			prompt = AllowAVPermissionPrompt
		case webutil.PermissionNotification:
			prompt = ShowNotificationsPrompt
		default:
			return permissionPrompts, errors.Errorf("Prompt of %q permission is not defined", permissionName)
		}

		isPrompt, err := webutil.IsPermissionPrompt(ctx, conn, permissionName)
		if err != nil {
			return permissionPrompts, errors.Wrapf(err, "failed to check permission status of %q", permissionName)
		}

		if isPrompt {
			// Append prompt handling only if it does not exist.
			promptExists := false
			for _, promptToBeHandled := range permissionPrompts {
				if prompt == promptToBeHandled {
					promptExists = true
					break
				}
			}
			if !promptExists {
				permissionPrompts = append(permissionPrompts, prompt)
			}
		}
	}
	return permissionPrompts, nil
}

// ActionAndGrantPermissionIfRequired returns an action that performing a user action which potentially triggers a permission prompt.
// It grants the permission if required.
func ActionAndGrantPermissionIfRequired(tconn *chrome.TestConn, conn *chrome.Conn, performAction action.Action, permissionName webutil.PermissionName) action.Action {
	return func(ctx context.Context) error {
		permissionPrompts, err := ChromePermissionPrompts(ctx, conn, permissionName)
		if err != nil {
			return errors.Wrapf(err, "failed to query prompts of permission %q", permissionName)
		}

		// No permission needs to be granted.
		if len(permissionPrompts) == 0 {
			return performAction(ctx)
		}

		ui := uiauto.New(tconn)
		if err := ui.RetryUntil(
			performAction,
			ui.WithTimeout(5*time.Second).WaitUntilExists(permissionPrompts[0].PromptFinder),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to perform action for prompt")
		}

		if err := DismissPrompt(ui, permissionPrompts[0])(ctx); err != nil {
			return errors.Wrapf(err, "failed to grant permission %q", permissionName)
		}
		return nil
	}
}
