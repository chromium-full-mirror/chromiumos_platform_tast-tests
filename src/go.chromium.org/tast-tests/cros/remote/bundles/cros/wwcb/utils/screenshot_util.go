// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
)

// DownloadsPath is an absolute path on DUT.
const DownloadsPath = "/home/chronos/user/MyFiles/Downloads"

var (
	// screenshotPattern is the filename format of screenhsot taken by screen capture.
	screenshotPattern = regexp.MustCompile(`Screenshot (\d{4})-(\d{2})-(\d{2}) (\d{1,2}).(\d{2}).(\d{2}) (AM|PM).png`)
)

var (
	// BuildinDisplayFinder is the finder of build-in display.
	BuildinDisplayFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Built-in display"}},
			{Value: &ui.NodeWith_HasClass{HasClass: "RootWindow-0"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// statusAreaFinder is the finder to find the Quick Settings area in the UI.
	statusAreaFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "UnifiedSystemTray"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// screenCaptureFinder is the finder to open screen capture tool.
	screenCaptureFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Screen capture"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// takeWindowScreenshotFinder is the finder to open function to take window screenshot.
	takeWindowScreenshotFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Take window screenshot"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_TOGGLE_BUTTON}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// takeFullScreenshotFinder is the finder to open function to take full screen screenshot.
	takeFullScreenshotFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Take full screen screenshot"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_TOGGLE_BUTTON}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
)

// TakeWindowScreenshot returns a file info by taking the window screenshot.
func TakeWindowScreenshot(ctx context.Context, uiautoSvc ui.AutomationServiceClient, fs *dutfs.Client, window *ui.Finder) (os.FileInfo, error) {
	screenshotStartTime := time.Now()
	for _, test := range []struct {
		finder *ui.Finder
	}{
		{statusAreaFinder},
		{screenCaptureFinder},
		{takeWindowScreenshotFinder},
		{window},
	} {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: test.finder}); err != nil {
			return nil, errors.Wrapf(err, "failed to wait for the %q finder from context menu", test.finder)
		}
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: test.finder}); err != nil {
			return nil, errors.Wrapf(err, "failed to click on the %q status area from context menu", test.finder)
		}
	}

	screenshotInfo, err := waitForFileSaved(ctx, fs, DownloadsPath, screenshotPattern, screenshotStartTime, 5*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for screenshot file saved")
	}

	return screenshotInfo, nil
}

// TakeFullScreenshot returns a file info by taking full screen screenshot.
func TakeFullScreenshot(ctx context.Context, uiautoSvc ui.AutomationServiceClient, fs *dutfs.Client) (os.FileInfo, error) {
	screenshotStartTime := time.Now()
	for _, test := range []struct {
		finder *ui.Finder
	}{
		{statusAreaFinder},
		{screenCaptureFinder},
		{takeFullScreenshotFinder},
		{BuildinDisplayFinder},
	} {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: test.finder}); err != nil {
			return nil, errors.Wrapf(err, "failed to wait for the %q finder from context menu", test.finder)
		}
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: test.finder}); err != nil {
			return nil, errors.Wrapf(err, "failed to click on the %q status area from context menu", test.finder)
		}
	}

	screenshotInfo, err := waitForFileSaved(ctx, fs, DownloadsPath, screenshotPattern, screenshotStartTime, 5*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for screenshot file saved")
	}

	return screenshotInfo, nil
}
