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
	"time"
)

type fileTypeSupportTestParameters struct {
	fileName    string
	isSupported bool
}

const (
	pngFileName  = constant.ImageTestFileName
	jpgFileName  = "jpg_image_20250117.jpg"
	jpegFileName = "jpeg_image_20250117.jpeg"
	webpFileName = "webp_image_20250117.webp"
	gifFileName  = "gif_image_20250117.gif"
	pdfFileName  = "pdf_file_20250117.pdf"
	webmFileName = "webm_video_20250117.webm"
	mp3FileName  = "mp3_audio_20250117.mp3"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FileTypeSupport,
		Desc: "Verify file type supported by mantis",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.DefaultTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc", "gaia"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{pngFileName, jpgFileName, jpegFileName, webpFileName, pdfFileName, webmFileName, mp3FileName, gifFileName},
		Attr:         []string{"group:mainline", "group:cbx", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
		Params: []testing.Param{
			{
				Name: "png",
				Val: fileTypeSupportTestParameters{
					fileName:    pngFileName,
					isSupported: true,
				},
			},
			{
				Name: "jpg",
				Val: fileTypeSupportTestParameters{
					fileName:    jpgFileName,
					isSupported: true,
				},
			},
			{
				Name: "jpeg",
				Val: fileTypeSupportTestParameters{
					fileName:    jpegFileName,
					isSupported: true,
				},
			},
			{
				Name: "webp",
				Val: fileTypeSupportTestParameters{
					fileName:    webpFileName,
					isSupported: true,
				},
			},
			{
				Name: "gif",
				Val: fileTypeSupportTestParameters{
					fileName:    gifFileName,
					isSupported: false,
				},
			},
			{
				Name: "pdf",
				Val: fileTypeSupportTestParameters{
					fileName:    pdfFileName,
					isSupported: false,
				},
			},
			{
				Name: "webm",
				Val: fileTypeSupportTestParameters{
					fileName:    webmFileName,
					isSupported: false,
				},
			},
			{
				Name: "mp3",
				Val: fileTypeSupportTestParameters{
					fileName:    mp3FileName,
					isSupported: false,
				},
			},
		},
	})
}

func FileTypeSupport(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	params := s.Param().(fileTypeSupportTestParameters)
	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(params.fileName), params.fileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	editWithAIButton := nodewith.Role(role.ToggleButton).Name("Edit with AI").Ancestor(galleryapp.RootFinder)
	isEditWithAIButtonExist := ui.WithTimeout(time.Second).Exists(editWithAIButton)(ctx) == nil

	if params.isSupported && !isEditWithAIButtonExist {
		s.Fatal("The 'Edit with AI' button is not displayed for a supported file type")
	}

	if !params.isSupported && isEditWithAIButtonExist {
		s.Fatal("The 'Edit with AI' button is unexpectedly displayed for an unsupported file type")
	}
}
