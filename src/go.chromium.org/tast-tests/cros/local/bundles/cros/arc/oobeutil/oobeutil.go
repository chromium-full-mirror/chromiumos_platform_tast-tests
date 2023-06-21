// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package oobeutil implements some functions used to go through OOBE screens.
// TODO(crbug.com/1327981): Use OOBE test API and move this package to `tast/local/bundles/cros/oobe` directory.
package oobeutil

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CompleteOnboardingFlow function goes through the onboarding flow screens.
func CompleteOnboardingFlow(ctx context.Context, ui *uiauto.Context) error {
	const (
		termTimeout            = 30 * time.Second
		anyDialogTimeout       = 5 * time.Second
		anyActionButtonTimeout = 1 * time.Minute
	)
	consolidatedConsentHeader := nodewith.Name("Review these terms and control your data").Role(role.Dialog)
	if err := ui.WithTimeout(termTimeout).WaitUntilExists(consolidatedConsentHeader)(ctx); err != nil {
		return err
	}

	// In lower resolution screens, a `see more` button is shown and the accept button is hidden until the `see more` button is clicked.
	acceptAndContinue := nodewith.Name("Accept and continue").Role(role.Button)
	acceptButtonFound, err := ui.IsNodeFound(ctx, acceptAndContinue)
	if err != nil {
		return err
	}
	if !acceptButtonFound {
		focusedButton := nodewith.State(state.Focused, true).Role(role.Button)
		if err := ui.LeftClick(focusedButton)(ctx); err != nil {
			return err
		}
	}

	if err := uiauto.Combine("accept and continue",
		ui.WaitUntilExists(acceptAndContinue),
		ui.LeftClickUntil(acceptAndContinue, ui.Gone(acceptAndContinue)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to accept terms")
	}

	anyDialog := nodewith.First().Role(role.Dialog)
	anyActionButton := nodewith.NameRegex(regexp.MustCompile(
		"Skip|" +
			"No thanks|" +
			"Next|" +
			"Accept and continue|" +
			"Turn on sync|" +
			"Get started")).First().Role(role.Button)

	lastActionTime := time.Now()
	for {
		if err := ui.Exists(anyActionButton)(ctx); err == nil {
			// Some action button is detected. Click it
			testing.ContextLog(ctx, "Detected action button")
			if err := ui.LeftClickUntil(anyActionButton, ui.Gone(anyActionButton))(ctx); err != nil {
				return errors.Wrap(err, "failed to click button")
			}

			testing.ContextLog(ctx, "Action button has been clicked")
			lastActionTime = time.Now()
			continue
		}

		if err := ui.WithTimeout(anyDialogTimeout).WaitUntilExists(anyDialog)(ctx); err == nil {
			// Some dialog is still shown.
			if time.Since(lastActionTime) > anyActionButtonTimeout {
				return errors.New("failed to detect action button")
			}
			continue
		}

		// Double sure any dialog is gone.
		if err := ui.Gone(anyDialog)(ctx); err != nil {
			return errors.Wrap(err, "failed to confirm dialog is gone")
		}

		break
	}

	return nil
}
