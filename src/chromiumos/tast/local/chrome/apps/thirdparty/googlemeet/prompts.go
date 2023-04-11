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
	"chromiumos/tast/local/chrome/webutil"
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

	captionDialogFinder = nodewith.Name("Caption languages & translation").Role(role.Dialog).Ancestor(meetRootWebArea)
	captionPrompt       = prompts.Prompt{
		Name:              "Caption languages & translation",
		PromptFinder:      captionDialogFinder,
		ClearButtonFinder: prompts.GotItButtonFinder.Ancestor(captionDialogFinder),
	}
)

// ClearPromptsForNewMeeting clears potential prompts on launching new meeting.
func (gm *GoogleMeet) ClearPromptsForNewMeeting(ctx context.Context) error {
	promptsToBeManaged := []prompts.Prompt{
		meetingReadyPrompt, meetKeepsYouSafePrompt, whiteboardPrompt, micMutedPrompt, captionPrompt,
	}

	// ChromePermissionPrompts will merge camera and microphone permissions to one prompt.
	permissionPrompts, err := prompts.ChromePermissionPrompts(ctx, gm.conn,
		webutil.PermissionNotification,
		webutil.PermissionCamera,
		webutil.PermissionMicrophone)
	if err != nil {
		return err
	}

	promptsToBeManaged = append(promptsToBeManaged, permissionPrompts...)

	return prompts.ClearPotentialPrompts(gm.tconn, 3*time.Second, promptsToBeManaged...)(ctx)
}
