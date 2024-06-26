// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/cryptohome/cleanup"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	fileName   = "test.txt"
	folderName = "test"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FilesAppContextMenuPinAndUnpin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that pinning to Holding Space from the Files app works",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-d1cdf1fd-1bf3-42ee-acef-5858f9ceb074",
		}},
		Params: []testing.Param{
			{
				Val: fileName,
			},
			{
				Name: "folder",
				Val:  folderName,
			},
		},
	})
}

// FilesAppContextMenuPinAndUnpin tests the functionality of pinning files to
// Holding Space from the Files app.
func FilesAppContextMenuPinAndUnpin(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to log in to Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	fsapp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Could not open filesapp: ", err)
	}

	if err := verifyTip(ctx, tconn, true /* tabletModeEnabled */); err != nil {
		s.Fatal("Failed to verify tip in tablet mode: ", err)
	}
	if err := verifyTip(ctx, tconn, false /* tabletModeEnabled */); err != nil {
		s.Fatal("Failed to verify tip in clamshell mode: ", err)
	}

	targetName := s.Param().(string)
	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's MyFiles path: ", err)
	}
	targetPath := filepath.Join(myFilesPath, targetName)

	if err := createTarget(targetPath, strings.HasSuffix(targetName, "txt") /* isFile */); err != nil {
		s.Fatalf("Failed to create %q: %v", targetPath, err)
	}
	defer os.Remove(targetPath)

	uia := uiauto.New(tconn)
	maximizeButton := nodewith.Name("Maximize").Role(role.Button).Ancestor(filesapp.WindowFinder(apps.FilesSWA.ID))
	// To prevent the "Pin to shelf" option from being hidden, maximize the Files app.
	if err := uiauto.IfSuccessThen(
		uia.WaitUntilExists(maximizeButton),
		uia.LeftClick(maximizeButton),
	)(ctx); err != nil {
		s.Fatal("Failed to maximize the Files app: ", err)
	}

	// Pin and unpin the item using the context menu in the files app. Unpinning
	// the item should make the Tray disappear, since nothing else is pinned.
	if err := uiauto.Combine("pin and unpin item from shelf via Files app context menu",
		fsapp.ClickContextMenuItem(targetName, "Pin to shelf"),
		uia.LeftClick(holdingspace.FindTray()),
		uia.WaitUntilExists(holdingspace.FindPinnedFileChip().Name(targetName)),
		fsapp.ClickContextMenuItem(targetName, "Unpin from shelf"),
		uia.WaitUntilGone(holdingspace.FindTray()),
		uia.EnsureGoneFor(holdingspace.FindTray(), time.Second),
	)(ctx); err != nil {
		s.Fatalf("Failed to pin and unpin item %q: %v", targetName, err)
	}
}

// verifyTip verifies that the Files app displays the expected educational tip.
func verifyTip(ctx context.Context, tconn *chrome.TestConn, tabletModeEnabled bool) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	cleanupFunc, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletModeEnabled)
	if err != nil {
		return errors.Wrapf(err, "failed to ensure tablet mode enabled is %t", tabletModeEnabled)
	}
	defer cleanupFunc(cleanupCtx)

	freeSpace, err := disk.FreeSpace(cleanup.UserHome)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve free disk space")
	}

	// In most cases, the holding space tip is expected to take priority.
	expectedTip := nodewith.Name("Create a shortcut for your files").Role(role.StaticText)
	unexpectedTip := nodewith.Name("Caution: These files are temporary and may be automatically deleted to free up disk space.").Role(role.StaticText)

	// The low disk space tip takes priority when there is <= 1 GiB remaining.
	if freeSpace <= 1*cleanup.GiB {
		expectedTip, unexpectedTip = unexpectedTip, expectedTip
	}

	ui := uiauto.New(tconn)
	return uiauto.Combine("verify tip",
		ui.WaitUntilExists(expectedTip),
		ui.EnsureGoneFor(unexpectedTip, 1*time.Second))(ctx)
}

// createTarget creates the target if it is not in the specific file path.
func createTarget(targetPath string, isFile bool) error {
	if isFile {
		// Create our file, with appropriate permissions so we can delete later.
		return ioutil.WriteFile(targetPath, []byte("Per aspera, ad astra"), 0644)
	}

	return os.Mkdir(targetPath, 0755)
}
