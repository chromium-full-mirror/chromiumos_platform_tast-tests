// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// VideoFile is a file to play to use webcam to check.
const VideoFile = "video.mp4"

// PictureFile is a file to play to use webcam to check.
const PictureFile = "red.jpg"

// MyFilesPath is an absolute path on DUT.
const MyFilesPath = "/home/chronos/user/MyFiles/"

var (
	// FilesWindowFinder is the finder of Files app window.
	FilesWindowFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: "Files - My files"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// openFileFinder is the finder used to open the file on the Files app.
	openFileFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
			{Value: &ui.NodeWith_Name{Name: "Open"}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: FilesWindowFinder}},
		},
	}
)

// PushFileToDUT copies the specified fileName from the test's data folder to the DUT's remoteDir.
// Returns the path of the file on the DUT on success.
func PushFileToDUT(ctx context.Context, s *testing.State, dut *dut.DUT, fileName, remoteDir string) (string, error) {
	remotePath := filepath.Join(remoteDir, fileName)
	testing.ContextLog(ctx, "Copy the file to remote data path: ", remotePath)
	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), map[string]string{
		s.DataPath(fileName): remotePath,
	}, linuxssh.DereferenceSymlinks); err != nil {
		return "", errors.Wrapf(err, "failed to send data to remote data path %v", remotePath)
	}

	return remotePath, nil
}

// OpenMediaFileOnFilesapp clicks the file and open it on Filesapp.
func OpenMediaFileOnFilesapp(ctx context.Context, uiautoSvc ui.AutomationServiceClient, videoName string) error {
	filesWindowFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: "Files - My files"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	fileNameFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
			{Value: &ui.NodeWith_Name{Name: videoName}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: filesWindowFinder}},
		},
	}

	filesOpenButtonFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
			{Value: &ui.NodeWith_Name{Name: "Open"}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: filesWindowFinder}},
		},
	}

	galleryWindowName := fmt.Sprintf("Gallery - %s", videoName)
	galleryWindowFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: galleryWindowName}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: filesWindowFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for FilesApp showing on screen")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: fileNameFinder}); err != nil {
		return errors.Wrap(err, "failed to click on the video filename")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: filesOpenButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to click on the open button")
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: galleryWindowFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for Gallery window showing on screen")
	}

	return nil
}

// ClickOnPlayButton clicks button to play the file on Gallery.
func ClickOnPlayButton(ctx context.Context, uiautoSvc ui.AutomationServiceClient) error {
	galleryPlayButtonFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_TOGGLE_BUTTON}},
			{Value: &ui.NodeWith_Name{Name: "Toggle play pause"}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: galleryPlayButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for play button to show")
	}

	info, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: galleryPlayButtonFinder})
	if err != nil {
		return errors.Wrap(err, "failed to get play button info")
	}

	if info.NodeInfo.Checked == ui.Checked_CHECKED_FALSE {
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: galleryPlayButtonFinder}); err != nil {
			return errors.Wrap(err, "failed to click on play button")
		}
	}

	return nil
}

// CloseWindow closes the certain window using keyboard accelerators.
func CloseWindow(ctx context.Context, keyboardSvc inputspb.KeyboardServiceClient, uiautoSvc ui.AutomationServiceClient, windowName string) error {
	windowFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: windowName}},
		},
	}

	//Use keyboard shortcut to close app.
	if _, err := keyboardSvc.Accel(ctx, &inputspb.AccelRequest{Key: "Ctrl+W"}); err != nil {
		return errors.Wrap(err, "failed to type Ctrl+W")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if res, _ := uiautoSvc.IsNodeFound(ctx, &ui.IsNodeFoundRequest{Finder: windowFinder}); res.Found {
			return errors.Errorf("failed to close %s window", windowName)
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Second}); err != nil {
		return err
	}

	return nil
}

// ClickFullScreenButton clicks button to full screen on Gallery.
func ClickFullScreenButton(ctx context.Context, uiautoSvc ui.AutomationServiceClient) error {
	connectButtonNode := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: "fullscreen"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: connectButtonNode}); err != nil {
		return errors.Wrap(err, "failed to wait for fullscreen button to show")
	}

	if _, err := uiautoSvc.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: connectButtonNode}); err != nil {
		return errors.Wrap(err, "failed to click the connect button")
	}

	return nil
}

// OpenMediaFileWithGallery clicks the file on the Filesapp and open it with the Gallery app.
func OpenMediaFileWithGallery(ctx context.Context, uiautoSvc ui.AutomationServiceClient, fileName string) (string, error) {
	fileNameFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
			{Value: &ui.NodeWith_Name{Name: fileName}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: FilesWindowFinder}},
		},
	}

	galleryWindow := fmt.Sprintf("Gallery - %s", fileName)
	galleryWindowFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: galleryWindow}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: FilesWindowFinder}); err != nil {
		return "", errors.Wrap(err, "failed to wait for FilesApp showing on screen")
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: fileNameFinder}); err != nil {
		return "", errors.Wrap(err, "failed to wait for filename showing on screen")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: fileNameFinder}); err != nil {
		return "", errors.Wrap(err, "failed to click on the filename")
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: openFileFinder}); err != nil {
		return "", errors.Wrap(err, "failed to wait for open file button showing on screen")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: openFileFinder}); err != nil {
		return "", errors.Wrap(err, "failed to click on the open button")
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: galleryWindowFinder}); err != nil {
		return "", errors.Wrap(err, "failed to wait for Gallery window showing on screen")
	}

	return galleryWindow, nil
}

// ClickOnMaximizeButton clicks on the maximize button of the given window name.
func ClickOnMaximizeButton(ctx context.Context, uiautoSvc ui.AutomationServiceClient, windowName string) error {
	window := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_Name{Name: windowName}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	maxButton := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Maximize"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
			{Value: &ui.NodeWith_First{First: true}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: window}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: maxButton}); err != nil {
		return errors.Wrap(err, "failed to wait for maximize button from context menu")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: maxButton}); err != nil {
		return errors.Wrap(err, "failed to click on maximize button from context menu")
	}

	return nil
}
