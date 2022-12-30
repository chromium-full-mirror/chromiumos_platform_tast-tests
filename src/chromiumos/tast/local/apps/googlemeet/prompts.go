// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"regexp"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

type promptType struct {
	name              string
	promptFinder      *nodewith.Finder
	clearButtonFinder *nodewith.Finder
}

var (
	generalAllowButtonFinder   = nodewith.Name("Allow").Role(role.Button)
	generalDismissButtonFinder = nodewith.Name("Dismiss").Role(role.Button)
	generalCloseButtonFinder   = nodewith.Name("Close").Role(role.Button)
	generalGotItButtonFinder   = nodewith.Name("Got it").Role(role.Button)

	showNotificationsPromptFinder = nodewith.NameContaining("Show notifications").ClassName("RootView").Role(role.AlertDialog)
	allowNotificationPrompt       = promptType{
		name:              "allow notifications",
		promptFinder:      showNotificationsPromptFinder,
		clearButtonFinder: generalAllowButtonFinder.Ancestor(showNotificationsPromptFinder),
	}

	avPermFinder  = nodewith.NameRegex(regexp.MustCompile(".*Use your (microphone|camera).*")).ClassName("RootView").Role(role.AlertDialog).First()
	allowAVPrompt = promptType{
		name:              "allow microphone and camera",
		promptFinder:      avPermFinder,
		clearButtonFinder: generalAllowButtonFinder.Ancestor(avPermFinder),
	}

	yourMeetingIsReadyDialogFinder = nodewith.Name("Your meeting's ready").Role(role.Dialog).Ancestor(meetingWebview)
	meetingReadyPrompt             = promptType{
		name:              "Your meeting's ready",
		promptFinder:      yourMeetingIsReadyDialogFinder,
		clearButtonFinder: generalCloseButtonFinder.Ancestor(yourMeetingIsReadyDialogFinder),
	}

	meetKeepsYouSafeDialogFinder = nodewith.Name("Meet keeps you safe").Role(role.Dialog).Ancestor(meetingWebview)
	meetKeepsYouSafePrompt       = promptType{
		name:              "Meet keeps you safe",
		promptFinder:      meetKeepsYouSafeDialogFinder,
		clearButtonFinder: generalGotItButtonFinder.Ancestor(meetKeepsYouSafeDialogFinder),
	}

	whiteboardDialogFinder = nodewith.Name("Gather around a whiteboard").Role(role.Dialog).Ancestor(meetingWebview)
	whiteboardPrompt       = promptType{
		name:              "Gather around a whiteboard",
		promptFinder:      whiteboardDialogFinder,
		clearButtonFinder: generalGotItButtonFinder.Ancestor(whiteboardDialogFinder),
	}
)

// clearPrompts clears one or more prompts in "Google Meet" disorderly.
func (gm *GoogleMeet) clearPrompts(promptCandidates ...promptType) action.Action {
	// Find any existing prompt in certain time and returns the index. Returns -1 if no prompt found.
	findPrompt := func(ctx context.Context, candidates []promptType) int {
		foundPrompt := -1
		if len(candidates) == 0 {
			testing.ContextLog(ctx, "No more prompts to be cleared")
			return foundPrompt
		}

		testing.Poll(ctx, func(ctx context.Context) error {
			for index, promptToBeManaged := range candidates {
				// Continue to check the next if alert not found.
				if err := gm.ui.Exists(promptToBeManaged.promptFinder)(ctx); err == nil {
					foundPrompt = index
					return nil
				}
			}
			// Return an error to continue poll.
			return errors.New("continue to search until timeout")
		}, &testing.PollOptions{Timeout: shortUITimeout})
		return foundPrompt
	}

	// Return only if no prompt found.
	return func(ctx context.Context) error {
		tmpCandidates := promptCandidates
		for {
			i := findPrompt(ctx, tmpCandidates)
			if i < 0 {
				return nil
			}

			testing.ContextLogf(ctx, "Clearing prompt %q", tmpCandidates[i].name)
			if err := gm.ui.DoDefaultUntil(tmpCandidates[i].clearButtonFinder, gm.ui.WithTimeout(shortUITimeout).WaitUntilGone(tmpCandidates[i].promptFinder))(ctx); err != nil {
				return errors.Wrapf(err, "failed to clear %q", tmpCandidates[i].name)
			}
			tmpCandidates = append(tmpCandidates[:i], tmpCandidates[i+1:]...)
		}
	}
}

// ClearPromptsForNewMeeting clears potential prompts on launching new meeting.
func (gm *GoogleMeet) ClearPromptsForNewMeeting(ctx context.Context) error {
	promptsToBeManaged := []promptType{
		allowNotificationPrompt, allowAVPrompt, meetingReadyPrompt, meetKeepsYouSafePrompt, whiteboardPrompt,
	}
	return gm.clearPrompts(promptsToBeManaged...)(ctx)
}
