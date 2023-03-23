// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uidetection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/coords"
	"chromiumos/tast/local/personalization"
	"chromiumos/tast/local/uidetection"
	"chromiumos/tast/testing"
)

type serverType int

const (
	prodServer serverType = iota
	stagingServer
	whiteWallpaperFileName string = "basic_detections_white_wallpaper.jpg"
	chromeIconFileName     string = "basic_detections_logo_chrome.png"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BasicDetections,
		Desc:         "Confirm that the image-based uidetection library works as intended",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts:     []string{"chromeos-engprod-sydney@google.com", "alvinjia@google.com", "mattlui@google.com"},
		BugComponent: "b:1103568", // ChromeOS -> EngProd -> Apps
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      12 * time.Minute,
		Data:         []string{chromeIconFileName, whiteWallpaperFileName},
		VarDeps:      uidetection.UIDetectionVars,
		Params: []testing.Param{
			{
				Val: prodServer,
			},
			{
				Name: "staging_server",
				Val:  stagingServer,
			},
		},
	})
}

func setSolidWhiteWallpaper(ctx context.Context, tconn *chrome.TestConn, s *testing.State) error {
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Fetch the wallpaper from a Tast data file to minimise external
	// dependencies.
	if err := tconn.Call(ctx, nil, `(url) => tast.promisify(chrome.wallpaper.setWallpaper)({
              url: url,
              layout: 'STRETCH',
              filename: 'test_wallpaper'
            })`, server.URL+"/"+whiteWallpaperFileName); err != nil {
		return err
	}
	return nil
}

func setLightMode(ui *uiauto.Context) uiauto.Action {
	return uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui),
		personalization.ClosePersonalizationHub(ui),
	)
}

func setAutoThemeMode(ui *uiauto.Context) uiauto.Action {
	return uiauto.Combine("Enable auto theme mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleAutoMode(ui),
		personalization.ClosePersonalizationHub(ui),
	)
}

func BasicDetections(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	server := s.Param().(serverType)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 15*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	if err := ash.SetTabletModeEnabled(ctx, tconn, false); err != nil {
		s.Fatal("Failed to enter clamshell mode: ", err)
	}

	var ud *uidetection.Context
	if server == prodServer {
		ud = uidetection.NewDefault(tconn)
	} else {
		keyType := s.RequiredVar(uidetection.KeyType)
		key := s.RequiredVar(uidetection.Key)
		serverAddr := "staging-chromeosuidetection.sandbox.googleapis.com:443"
		ud = uidetection.New(tconn, keyType, key, serverAddr)
	}

	ui := uiauto.New(tconn)

	chromeIcon := uidetection.CustomIcon(s.DataPath(chromeIconFileName))
	addShortcut := uidetection.TextBlock([]string{"Add", "shortcut"})
	bottomBar := nodewith.ClassName("ShelfView")
	notificationArea := nodewith.ClassName("StatusAreaWidget")
	chromeWindow := nodewith.Role(role.Window).Name("Chrome - New Tab")

	verifyChromeIsMinimized := uiauto.NamedAction("verify that chrome is minimized",
		ui.WaitUntilExists(chromeWindow.Invisible()))
	verifyChromeIsShown := uiauto.NamedAction("verify that chrome is shown",
		ui.WaitUntilExists(chromeWindow.Visible()))

	expectError := func(f *uidetection.Finder, expectation string) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			if loc, err := ud.Location(ctx, f); err == nil {
				return errors.Errorf("expected error: %s, actually successful with value %+v", expectation, loc)
			} else if !strings.Contains(err.Error(), expectation) {
				return errors.Errorf("expected error: %s, actual error: %s", expectation, err)
			}
			return nil
		}
	}

	maximizeButton := nodewith.Role(role.Button).ClassName("FrameSizeButton").Name("Maximize")

	// Use light mode and the default wallpaper to minimise icon detection flakiness.
	if err := setLightMode(ui)(ctx); err != nil {
		s.Fatal("Failed to set light mode: ", err)
	}
	defer func(cleanupCtx context.Context) {
		// If Chrome is open, minimise it to make the desktop visible.
		if err := uiauto.IfSuccessThen(verifyChromeIsShown, ud.LeftClick(chromeIcon))(cleanupCtx); err != nil {
			s.Fatal("Failed to minimise Chrome: ", err)
		}
		if err := setAutoThemeMode(ui)(cleanupCtx); err != nil {
			s.Fatal("Failed to set auto theme mode: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "basic_detections")

	if err := setSolidWhiteWallpaper(ctx, tconn, s); err != nil {
		s.Fatal("Failed to switch to the default wallpaper: ", err)
	}

	// Perform UI interaction to click Chrome logo to open Chrome,
	// click "Add shortcut", and click "cancel".
	// This covers all three UI detections: custom icon detection,
	// textblock detection and word detection.
	if err := uiauto.Combine("verify detections",
		uiauto.Combine("verify that basic matchers work",
			ud.LeftClick(chromeIcon),
			verifyChromeIsShown,
			ud.WithScreenshotStrategy(uidetection.ImmediateScreenshot).LeftClick(uidetection.TextBlock([]string{"Customize", "Chrome"})),
			ud.LeftClick(uidetection.Word("Cancel")),
			ud.WaitUntilGone(uidetection.Word("Cancel")),
			// Check the negative cases.
			expectError(uidetection.Word("Google"), uidetection.ErrMultipleMatch),
			expectError(uidetection.Word("Google").Nth(10), uidetection.ErrNthNotFound),
			expectError(uidetection.TextBlock([]string{"Add", "shortcut"}).Nth(2), uidetection.ErrNthNotFound),
			// Maximize chrome to ensure that the left and the right of the chrome
			// are empty bounding boxes later on. Do this early so that we don't
			// have a race condition later.
			uiauto.IfSuccessThen(ui.Exists(maximizeButton), ui.LeftClick(maximizeButton))),

		uiauto.Combine("verify that within successfully matches",
			uiauto.Combine("for px",
				expectError(chromeIcon.WithinPx(coords.NewRect(0, 0, 50, 50)), uidetection.ErrNotFound),
				ud.LeftClick(chromeIcon.WithinPx(coords.NewRect(50, 50, 9999, 9999))),
				verifyChromeIsMinimized),
			uiauto.Combine("for dp",
				expectError(chromeIcon.WithinDp(coords.NewRect(0, 0, 50, 50)), uidetection.ErrNotFound),
				ud.LeftClick(chromeIcon.WithinDp(coords.NewRect(50, 50, 9999, 9999))),
				verifyChromeIsShown),
			uiauto.Combine("for ui detection",
				expectError(chromeIcon.Within(addShortcut), uidetection.ErrNotFound),
				ud.LeftClick(chromeIcon.Within(chromeIcon)),
				verifyChromeIsMinimized),
			uiauto.Combine("for a11y nodes",
				expectError(chromeIcon.WithinA11yNode(notificationArea), uidetection.ErrNotFound),
				ud.LeftClick(chromeIcon.WithinA11yNode(bottomBar)),
				verifyChromeIsShown)),

		uiauto.Combine("verify that relative and pixel bounds successfully match",
			uiauto.Combine("for a11y nodes",
				expectError(chromeIcon.RightOfA11yNode(chromeWindow), uidetection.ErrEmptyBoundingBox),
				expectError(chromeIcon.LeftOfA11yNode(chromeWindow), uidetection.ErrEmptyBoundingBox),
				ud.LeftClick(chromeIcon.LeftOfA11yNode(notificationArea)),
				verifyChromeIsMinimized),
			uiauto.Combine("for dp",
				expectError(chromeIcon.LeftOfDp(50), uidetection.ErrNotFound),
				ud.LeftClick(chromeIcon.RightOfDp(50)),
				verifyChromeIsShown),
			uiauto.Combine("for ui detection",
				expectError(chromeIcon.Above(addShortcut), uidetection.ErrNotFound),
				ud.LeftClick(chromeIcon.Below(addShortcut)),
				verifyChromeIsMinimized),
			uiauto.Combine("for px",
				expectError(chromeIcon.AbovePx(100), uidetection.ErrNotFound),
				ud.LeftClick(chromeIcon.BelowPx(100)),
				verifyChromeIsShown)),

		uiauto.Combine("verify that resized images successfully match",
			ud.WithScreenshotResizing().WaitUntilExists(addShortcut),
			ud.WithScreenshotResizing().WaitUntilExists(uidetection.Word("Tab")),
		))(ctx); err != nil {
		s.Fatal("Failed to perform image-based UI interactions: ", err)
	}
}
