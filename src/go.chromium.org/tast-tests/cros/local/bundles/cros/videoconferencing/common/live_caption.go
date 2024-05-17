// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	liveCaptionBubble  = nodewith.ClassName("CaptionBubbleFrameView")
	liveCaptionContent = nodewith.Name("Hello").Role(role.StaticText)
)

// PlayAudioAndVerifyBuble plays audio and verify the live caption shows up.
func PlayAudioAndVerifyBuble(ctx context.Context, tconn *chrome.TestConn, playAudioAction func(ctx context.Context) error) error {
	// Play audio and wait for the caption.
	// This need to be triggered multiple times because of its flakiness.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Play audio.
		if err := playAudioAction(ctx); err != nil {
			return errors.Wrap(err, "failed to play audio")
		}

		// Wait for the buble to show.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			ui := uiauto.New(tconn)
			return uiauto.Combine("Wait for the bubble and content",
				ui.WaitUntilExists(liveCaptionBubble),
				ui.WaitUntilExists(liveCaptionContent),
			)(ctx)
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to wait for the caption buble and content")
		}
		return nil

	}, &testing.PollOptions{Timeout: 60 * time.Second, Interval: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to validate live caption")
	}

	return nil
}
