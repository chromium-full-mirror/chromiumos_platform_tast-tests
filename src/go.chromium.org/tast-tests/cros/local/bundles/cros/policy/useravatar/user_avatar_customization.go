// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package useravatar contains helpers to verify user avatar policies.
package useravatar

import (
	"context"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	chooseFromFileButtonName = "Choose a file"
	takePhotoButtonName      = "Take a photo"
	takeVideoButtonName      = "Create a looping video"
	profileImageName         = "Google profile photo"

	avatarButtonContainerClass = "avatar-button-container"
	imageContainerClass        = "image-container"

	// AnnotationHashCode is the hashcode of network annotation: signed_in_profile_avatar
	AnnotationHashCode = "108903331"
)

// CustomizationTestCase defines test expectations based on the value of policy
// UserAvatarCustomizationSelectorsEnabled.
type CustomizationTestCase struct {
	Name                      string
	ShouldFindAnnotation      bool
	ShouldFindCustomSelectors bool
	Policy                    *policy.UserAvatarCustomizationSelectorsEnabled
}

// TestCases returns the list of CustomizationTestCase objects on which
// UserAvatarCustomizationSelectorsEnabled policy is tested.
func TestCases() []CustomizationTestCase {
	// The "disabled" test case must be the first element of the returned list.
	// Otherwise, policy.NetworkRequestMonitor test will use the wrong test case.
	return []CustomizationTestCase{
		{
			Name:                      "disabled",
			ShouldFindAnnotation:      false,
			ShouldFindCustomSelectors: false,
			Policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Val: false},
		},
		{
			Name:                      "unset",
			ShouldFindAnnotation:      true,
			ShouldFindCustomSelectors: true,
			Policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Stat: policy.StatusUnset},
		},
		{
			Name:                      "enabled",
			ShouldFindAnnotation:      true,
			ShouldFindCustomSelectors: true,
			Policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Val: true},
		},
	}
}

// TriggerUserAvatarCustomization opens the user avatar customization app and
// verifies that certain avatar selectors are disabled based on policy value.
func TriggerUserAvatarCustomization(ctx context.Context, _ *chrome.Chrome,
	_ *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, paramIndex int) error {

	param := TestCases()[paramIndex]
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// Open user avatar personalization app. Note: We retry here because sometimes the button
	// to open the user avatar subpage does not load properly.
	breadcrumbAvatar := personalization.BreadcrumbNodeFinder(personalization.AvatarSubpageName)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Close any previously opened personalization apps.
		if err := ash.CloseAllWindows(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close app window")
		}

		// Open avatar subpage.
		if err := uiauto.Combine("open avatar subpage",
			personalization.OpenPersonalizationHub(ui),
			personalization.OpenAvatarSubpage(ui),
			ui.WithTimeout(3*time.Second).WaitUntilExists(breadcrumbAvatar),
		)(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to open avatar subpage: ", err)
			return errors.Wrap(err, "failed to open avatar subpage")
		}

		return nil // exit successfully
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to open avatar personalization app")
	}

	chooseFromFileSelector := selectorFinder(chooseFromFileButtonName, avatarButtonContainerClass)
	takePhotoSelector := selectorFinder(takePhotoButtonName, avatarButtonContainerClass)
	takeVideoSelector := selectorFinder(takeVideoButtonName, avatarButtonContainerClass)
	profileImageSelector := selectorFinder(profileImageName, imageContainerClass)

	if param.ShouldFindCustomSelectors {
		if err := uiauto.Combine("Verify custom selectors are shown",
			ui.WaitUntilExists(chooseFromFileSelector),
			ui.WaitUntilExists(takePhotoSelector),
			ui.WaitUntilExists(takeVideoSelector),
			ui.WaitUntilExists(profileImageSelector),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify custom selectors are shown")
		}
	} else {
		if err := uiauto.Combine("Verify custom selectors are not shown",
			ui.WaitUntilGone(chooseFromFileSelector),
			ui.WaitUntilGone(takePhotoSelector),
			ui.WaitUntilGone(takeVideoSelector),
			ui.WaitUntilGone(profileImageSelector),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify custom selectors are not shown")
		}
	}
	return nil
}

func selectorFinder(name, class string) *nodewith.Finder {
	return nodewith.Role(role.ListBoxOption).Name(name).HasClass(class)
}
