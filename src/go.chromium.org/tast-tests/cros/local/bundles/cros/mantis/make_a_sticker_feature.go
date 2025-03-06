// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/constant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	makeAStickerExpectedResult = "make_a_sticker_result_20250306.png"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MakeAStickerFeature,
		Desc: "Verify make a sticker functionality",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.DefaultTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{constant.ImageTestFileName, makeAStickerExpectedResult},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
	})
}

func MakeAStickerFeature(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(constant.ImageTestFileName), constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	defer os.Remove(filepath.Join(downloadsPath, constant.ImageTestFileName))

	ui := uiauto.New(tconn)

	if err := util.OpenEditWithAIPanel(ctx, ui); err != nil {
		s.Fatal("Failed to open edit with AI panel: ", err)
	}

	// The initialization process might be very quick, so the spinner
	// and progress bar might not be detected by UI automation.
	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	if err := util.WaitForProgressBar(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for progress bar: ", err)
	}

	makeAStickerButton := nodewith.Role(role.Button).Name("Make a Sticker").Ancestor(galleryapp.RootFinder).First()
	if err := ui.DoDefault(makeAStickerButton)(ctx); err != nil {
		s.Fatal("Unable to open 'Make a Sticker' panel: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	// Draw a scribble on the image
	if err := util.DrawOnImage(ctx, tconn, ui); err != nil {
		s.Fatal("Cannot draw on the image: ", err)
	}

	// Click on the make a sticker button
	if err := ui.LeftClickUntil(makeAStickerButton, ui.Exists(nodewith.NameContaining("Mark the area").Role(role.StaticText)))(ctx); err != nil {
		s.Fatal("Unable to click 'Make a Sticker' button: ", err)
	}

	s.Log("Make a Sticker on process")
	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Fatal("Error while waiting for spinner: ", err)
	}

	canvasBounds, err := ui.ImmediateLocation(ctx, util.ImageCanvas)
	if err != nil {
		s.Fatal("Failed to get canvas location: ", err)
	}

	if err := mouse.Click(tconn, coords.NewPoint(
		canvasBounds.CenterX(),
		canvasBounds.CenterY(),
	), mouse.LeftButton)(ctx); err != nil {
		s.Fatal("Failed to click the mouse: ", err)
	}

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(doneButton)(ctx); err != nil {
		s.Fatal("Failed to click the done button: ", err)
	}

	// Save the result image and close Gallery app.
	saveButton := nodewith.Role(role.Button).Name("Save").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(saveButton)(ctx); err != nil {
		s.Fatal("Failed to click the save button: ", err)
	}
	savedText := nodewith.Role(role.StaticText).Name("Saved").Ancestor(galleryapp.RootFinder)
	if err := ui.WithTimeout(time.Second * 5).WaitUntilExists(savedText)(ctx); err != nil {
		s.Fatal("Failed to save image: ", err)
	}

	gotResult, err := util.FetchImage(downloadsPath + "/" + constant.ImageTestFileName)
	if err != nil {
		s.Fatal("Failed to get image result: ", err)
	}

	expectedResult, err := util.FetchImage(s.DataPath(makeAStickerExpectedResult))
	if err != nil {
		s.Fatal("Failed to get expected result: ", err)
	}

	diffPercentage := util.ImageDiffPercentage(gotResult, expectedResult)
	if diffPercentage > constant.DefaultImageDiffPercentageThreshold {
		s.Fatal("The image difference exceeds the threshold: ", diffPercentage)
	}
}
