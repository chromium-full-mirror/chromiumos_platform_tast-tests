// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package meetcuj contains the test code for Meet/MeetMultitasking CUJ.
package meetcuj

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ToggleFileMenuButton toggles the "File" menu button for press and release metrics.
func ToggleFileMenuButton(ctx context.Context, ui *uiauto.Context, pc pointer.Context, inTabletMode bool) error {
	fileMenu := nodewith.Name("File").Role(role.MenuItem).HasClass("menu-button").First()
	menuContainer := nodewith.Role(role.MenuBar).HasClass("goog-container").First()
	if !inTabletMode {
		if err := ui.MouseMoveTo(fileMenu, 500*time.Millisecond)(ctx); err != nil {
			return errors.Wrap(err, "failed to move mouse to File menu button")
		}
	}

	clickFileMenu := pc.Click(fileMenu)
	waitForFileMenu := ui.WithTimeout(10 * time.Second).WaitUntilExists(menuContainer)
	return uiauto.NamedAction("toggle file menu button",
		// If the File menu doesn't appear, maybe it's because the click
		// only focused the page. Then we just need to click again.
		ui.WithTimeout(time.Minute).RetryUntil(clickFileMenu, waitForFileMenu),
	)(ctx)
}

// EnsureElementGetsScrolled ensures element gets scrolled.
func EnsureElementGetsScrolled(ctx context.Context, conn *chrome.Conn, element string) error {
	testing.ContextLog(ctx, "Ensure element gets scrolled")
	var scrollTop int
	if err := conn.Eval(ctx, fmt.Sprintf("parseInt(%s.scrollTop)", element), &scrollTop); err != nil {
		return errors.Wrap(err, "failed to get the number of pixels that the scrollbar is scrolled vertically")
	}
	if scrollTop == 0 {
		return errors.Errorf("%s is not getting scrolled", element)
	}
	return nil
}
