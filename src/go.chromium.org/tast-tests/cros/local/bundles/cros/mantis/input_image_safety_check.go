// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/constant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"time"
)

type inputImageSafetyCheckTestParameters struct {
	fileName string
	isSafe   bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: InputImageSafetyCheck,
		Desc: "Verify input image safety check",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.DefaultTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{constant.ImageTestFileName, constant.UnsafeImageTestFileName},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
		Params: []testing.Param{
			{
				Name: "safe",
				Val: inputImageSafetyCheckTestParameters{
					fileName: constant.ImageTestFileName,
					isSafe:   true,
				},
			},
			{
				Name: "unsafe",
				Val: inputImageSafetyCheckTestParameters{
					fileName: constant.UnsafeImageTestFileName,
					isSafe:   false,
				},
			},
		},
	})
}

func InputImageSafetyCheck(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	params := s.Param().(inputImageSafetyCheckTestParameters)
	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(params.fileName), params.fileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	editWithAIButton := nodewith.Role(role.ToggleButton).Name("Edit with AI").Ancestor(galleryapp.RootFinder)
	if err := util.LeftClickButton(ctx, ui, editWithAIButton); err != nil {
		s.Fatal("Unable to click 'Edit with AI' button: ", err)
	}

	if err := util.WaitForProgressBar(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for progress bar: ", err)
	}

	// Hover to each tool buttons and then check whether the error tooltip is shown.
	tools := []string{"Expand Background", "Remove Background", "Make a Sticker", "Reimagine"}
	for _, tool := range tools {
		toolButton := nodewith.Role(role.Button).Name(tool).Ancestor(galleryapp.RootFinder).First()
		if err := uiauto.Combine("Hover mouse to tool button",
			ui.WithTimeout(constant.DefaultUITimeout).WaitForLocation(toolButton),
			ui.MouseMoveTo(toolButton, time.Second),
		)(ctx); err != nil {
			s.Fatal("Failed to hover to tool button: ", err)
		}

		var errorTooltipExist bool

		ud := uidetection.NewDefault(tconn)

		errorTooltip := uidetection.TextBlock([]string{"Can't", "edit", "this", "image"})
		if err := ud.WithTimeout(constant.DefaultUITimeout).WaitUntilExists(errorTooltip)(ctx); err != nil {
			errorTooltipExist = false
		} else {
			errorTooltipExist = true
		}

		if params.isSafe && errorTooltipExist {
			s.Fatal("Unexpected tooltip on a safe image")
		}

		if !params.isSafe && !errorTooltipExist {
			s.Fatal("Tooltip isn't shown on an unsafe image")
		}
	}
}
