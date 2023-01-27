// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/prompts"
	"chromiumos/tast/local/chrome/uiauto/role"
)

var (
	yourMeetingIsReadyDialogFinder = nodewith.Name("Your meeting's ready").Role(role.Dialog).Ancestor(meetRootWebArea)
	meetingReadyPrompt             = prompts.Prompt{
		Name:              "Your meeting's ready",
		PromptFinder:      yourMeetingIsReadyDialogFinder,
		ClearButtonFinder: prompts.CloseButtonFinder.Ancestor(yourMeetingIsReadyDialogFinder),
	}

	meetKeepsYouSafeDialogFinder = nodewith.Name("Meet keeps you safe").Role(role.Dialog).Ancestor(meetRootWebArea)
	meetKeepsYouSafePrompt       = prompts.Prompt{
		Name:              "Meet keeps you safe",
		PromptFinder:      meetKeepsYouSafeDialogFinder,
		ClearButtonFinder: prompts.GotItButtonFinder.Ancestor(meetKeepsYouSafeDialogFinder),
	}

	whiteboardDialogFinder = nodewith.Name("Gather around a whiteboard").Role(role.Dialog).Ancestor(meetRootWebArea)
	whiteboardPrompt       = prompts.Prompt{
		Name:              "Gather around a whiteboard",
		PromptFinder:      whiteboardDialogFinder,
		ClearButtonFinder: prompts.GotItButtonFinder.Ancestor(whiteboardDialogFinder),
	}

	micMutedAlertFinder = nodewith.Name("Your mic is muted by your system settings").Role(role.Alert).Ancestor(meetRootWebArea)
	micMutedPrompt      = prompts.Prompt{
		Name:              "Your mic is muted by your system settings",
		PromptFinder:      micMutedAlertFinder,
		ClearButtonFinder: prompts.CloseButtonFinder.Ancestor(micMutedAlertFinder),
	}
)

// ClearPromptsForNewMeeting clears potential prompts on launching new meeting.
func (gm *GoogleMeet) ClearPromptsForNewMeeting(ctx context.Context) error {
	promptsToBeManaged := []prompts.Prompt{
		prompts.ShowNotificationsPrompt, prompts.AllowAVPermissionPrompt, meetingReadyPrompt, meetKeepsYouSafePrompt, whiteboardPrompt, micMutedPrompt,
	}
	return prompts.ClearPotentialPrompts(gm.tconn, 3*time.Second, promptsToBeManaged...)(ctx)
}
