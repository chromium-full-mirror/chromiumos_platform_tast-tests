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
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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

	if err := util.OpenEditWithAIPanel(ctx, ui); err != nil {
		s.Fatal("Failed to open edit with AI panel: ", err)
	}

	if err := util.WaitForDLCPreparation(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for DLC preparation: ", err)
	}

	var errorMessageShown bool
	errorMessage := nodewith.Role(role.StaticText).NameContaining("Can’t edit this image. Try another image.").Ancestor(galleryapp.RootFinder)
	if err := ui.WithTimeout(constant.DefaultUITimeout).WaitUntilExists(errorMessage)(ctx); err != nil {
		errorMessageShown = false
	} else {
		errorMessageShown = true
	}

	if params.isSafe {
		if errorMessageShown {
			s.Fatal("Unexpected error message on a safe image")
		}

		tools := []string{"Expand Background", "Remove Background", "Make a Sticker", "Reimagine", "Erase"}
		for _, tool := range tools {
			toolButton := nodewith.Role(role.Button).Name(tool).Ancestor(galleryapp.RootFinder).First()
			if err := ui.WithTimeout(constant.DefaultUITimeout).WaitUntilExists(toolButton)(ctx); err != nil {
				s.Fatalf("%q tool button should be shown: %v", tool, err)
			}
		}
	}

	if !params.isSafe && !errorMessageShown {
		s.Fatal("Error message isn't shown on an unsafe image")
	}
}
