// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prompts

import (
	"context"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

// Prompt defines the structure of a typical prompt dialog.
type Prompt struct {
	Name              string           // Name of the prompt.
	PromptFinder      *nodewith.Finder // The node finder to locate prompt.
	ClearButtonFinder *nodewith.Finder // The button finder to dismiss prompt.
}

var (
	showNotificationsPromptFinder = nodewith.NameContaining("Show notifications").ClassName("RootView").Role(role.AlertDialog)
	avPermPromptFinder            = nodewith.NameRegex(regexp.MustCompile(".*Use your (microphone|camera).*")).ClassName("RootView").Role(role.AlertDialog).First()
)

// General dismiss button finders for prompts.
var (
	AllowButtonFinder   = nodewith.Name("Allow").Role(role.Button)
	DismissButtonFinder = nodewith.Name("Dismiss").Role(role.Button)
	CloseButtonFinder   = nodewith.Name("Close").Role(role.Button)
	GotItButtonFinder   = nodewith.Name("Got it").Role(role.Button)
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
	ClearButtonFinder: AllowButtonFinder.Ancestor(avPermPromptFinder),
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
			if strings.Contains(err.Error(), nodewith.ErrNotFound) {
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
