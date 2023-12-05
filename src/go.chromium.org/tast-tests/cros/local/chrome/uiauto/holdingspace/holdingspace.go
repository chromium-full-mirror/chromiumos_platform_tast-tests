// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/errors"
)

// Class names.
const downloadsSectionClassName = "DownloadsSection"
const filesAppChipClassName = "FilesAppChip"
const holdingSpaceItemChipViewClassName = "HoldingSpaceItemChipView"
const holdingSpaceItemScreenCaptureViewClassName = "HoldingSpaceItemScreenCaptureView"
const holdingSpaceTrayClassName = "HoldingSpaceTray"
const menuItemViewClassName = "MenuItemView"
const pinnedFilesBubbleClassName = "PinnedFilesBubble"
const pinnedFilesSectionClassName = "PinnedFilesSection"
const recentFilesBubbleClassName = "RecentFilesBubble"
const screenCapturesSectionClassName = "ScreenCapturesSection"

// FindChip returns a finder which locates a holding space chip node.
func FindChip() *nodewith.Finder {
	return nodewith.HasClass(holdingSpaceItemChipViewClassName)
}

// FindContextMenuItem returns a finder which locates a holding space context
// menu item node.
func FindContextMenuItem() *nodewith.Finder {
	return nodewith.HasClass(menuItemViewClassName)
}

// FindDownloadChip returns a finder which locates a holding space download chip
// node.
func FindDownloadChip() *nodewith.Finder {
	return nodewith.Ancestor(nodewith.HasClass(downloadsSectionClassName)).
		ClassName(holdingSpaceItemChipViewClassName)
}

// FindPinnedFileChip returns a finder which locates a holding space pinned file
// chip node.
func FindPinnedFileChip() *nodewith.Finder {
	return nodewith.Ancestor(nodewith.HasClass(pinnedFilesSectionClassName)).
		ClassName(holdingSpaceItemChipViewClassName)
}

// FindPinnedFilesBubble returns a finder which locates the pinned files bubble node.
func FindPinnedFilesBubble() *nodewith.Finder {
	return nodewith.HasClass(pinnedFilesBubbleClassName)
}

// FindPinnedFilesSectionFilesAppChip returns a finder which locates the holding
// space pinned files section Files app chip node.
func FindPinnedFilesSectionFilesAppChip() *nodewith.Finder {
	return nodewith.Ancestor(nodewith.HasClass(filesAppChipClassName).Ancestor(
		nodewith.HasClass(pinnedFilesSectionClassName))).Name("Open Files")
}

// FindPinnedFilesSectionFilesAppPrompt returns a finder which locates the
// holding space pinned files section Files app prompt node.
func FindPinnedFilesSectionFilesAppPrompt() *nodewith.Finder {
	return nodewith.Ancestor(nodewith.HasClass(pinnedFilesSectionClassName)).
		Name("You can pin your important files here. Open Files app to get started.")
}

// FindRecentFilesBubble returns a finder which locates the recent files bubble
// node.
func FindRecentFilesBubble() *nodewith.Finder {
	return nodewith.HasClass(recentFilesBubbleClassName)
}

// FindScreenCaptureView returns a finder which locates a holding space screen
// capture view node.
func FindScreenCaptureView() *nodewith.Finder {
	return nodewith.Ancestor(nodewith.HasClass(screenCapturesSectionClassName)).
		ClassName(holdingSpaceItemScreenCaptureViewClassName)
}

// FindTray returns a finder which locates the holding space tray node.
func FindTray() *nodewith.Finder {
	return nodewith.HasClass(holdingSpaceTrayClassName)
}

// ResetHoldingSpaceOptions is defined in autotest_private.idl.
type ResetHoldingSpaceOptions struct {
	MarkTimeOfFirstAdd bool `json:"markTimeOfFirstAdd"`
}

// ResetHoldingSpace calls autotestPrivate to remove all items in the holding
// space model and resets all holding space prefs.
func ResetHoldingSpace(ctx context.Context, tconn *chrome.TestConn,
	options ResetHoldingSpaceOptions) error {
	if err := tconn.Call(ctx, nil,
		"tast.promisify(chrome.autotestPrivate.resetHoldingSpace)", options); err != nil {
		return errors.Wrap(err, "failed to reset holding space")
	}
	return nil
}
