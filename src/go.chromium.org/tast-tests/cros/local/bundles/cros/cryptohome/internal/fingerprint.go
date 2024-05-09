// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/testing"
)

func promptWithAsterisks(ctx context.Context, promptString string) {
	// This will look like:
	// *******************
	// * <prompt string> *
	// *******************
	promptString = "* " + promptString + " *"
	asterisks := strings.Repeat("*", len(promptString))
	testing.ContextLog(ctx, asterisks)
	testing.ContextLog(ctx, promptString)
	testing.ContextLog(ctx, asterisks)
}

// PromptFingerTouch prints the finger touch prompt.
func PromptFingerTouch(ctx context.Context, fingerName string) {
	promptWithAsterisks(ctx, "Please press your "+fingerName)
}

// PromptFingerEnroll prints the finger enroll prompt.
func PromptFingerEnroll(ctx context.Context, fingerName string) {
	promptWithAsterisks(ctx, "Please enroll your "+fingerName)
}

// PromptFingerMatch prints the finger match prompt.
func PromptFingerMatch(ctx context.Context, fingerName string) {
	promptWithAsterisks(ctx, "Please repeatedly press your "+fingerName)
}

// PromptFingerLift prints the finger lift prompt.
func PromptFingerLift(ctx context.Context) {
	promptWithAsterisks(ctx, "Please lift your finger")
	// GoBigSleepLint: we can't detect finger up event, so sleep 2 seconds and
	// assume the user has lifted their finger.
	testing.Sleep(ctx, 2*time.Second)
}
