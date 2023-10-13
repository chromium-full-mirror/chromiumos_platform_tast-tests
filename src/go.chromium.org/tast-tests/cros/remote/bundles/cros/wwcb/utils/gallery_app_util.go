// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"

	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"

	"go.chromium.org/tast/core/errors"
)

var (
	// GalleryWindowFinder is the finder of Gallery app window.
	GalleryWindowFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: "Gallery"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// openFileButtonFinder is the finder used to trigger "select a file to open" window on the Gallery app.
	openFileButtonFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
			{Value: &ui.NodeWith_Name{Name: "Open file"}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: GalleryWindowFinder}},
		},
	}

	// selectFileWindow is the finder of select a file window.
	selectFileWindow = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: "Select a file to open"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// openButtonFinder is the finder used to open the file on the select a file window.
	openButtonFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
			{Value: &ui.NodeWith_Name{Name: "Open"}},
			{Value: &ui.NodeWith_Focusable{Focusable: true}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: selectFileWindow}},
		},
	}
)

// OpenFileInGallery opens media file in the Gallery app and returns the window title.
func OpenFileInGallery(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, fileName string) (string, error) {
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Gallery", TimeoutSecs: 60}); err != nil {
		return "", errors.Wrap(err, "failed to launch Gallery")
	}

	fileNameFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
			{Value: &ui.NodeWith_Name{Name: fileName}},
		},
	}

	for _, finder := range []*ui.Finder{
		openFileButtonFinder,
		fileNameFinder,
		openButtonFinder,
	} {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: finder}); err != nil {
			return "", errors.Wrapf(err, "failed to wait until the %q exists", finder)
		}
		if _, err := uiautoSvc.DoDefault(ctx, &ui.DoDefaultRequest{Finder: finder}); err != nil {
			return "", errors.Wrapf(err, "failed to click on the %q", finder)
		}
	}

	galleryTitle := fmt.Sprintf("Gallery - %s", fileName)
	galleryWindow := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: galleryTitle}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: galleryWindow}); err != nil {
		return "", errors.Wrap(err, "failed to wait until Gallery window exist")
	}
	return galleryTitle, nil
}
