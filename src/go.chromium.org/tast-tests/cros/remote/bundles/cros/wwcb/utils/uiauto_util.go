// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"google.golang.org/grpc"

	"go.chromium.org/tast/core/ctxutil"
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
	FilesWindowFinder = ui.Node().Role(ui.Role_ROLE_WINDOW).Name("Files - My files").Nth(0).Finder()

	// openFileFinder is the finder used to open the file on the Files app.
	openFileFinder = ui.Node().Role(ui.Role_ROLE_BUTTON).Name("Open").Ancestor(FilesWindowFinder).Finder()
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
	filesWindowFinder := ui.Node().Role(ui.Role_ROLE_WINDOW).Name("Files - My files").Nth(0).Finder()

	fileNameFinder := ui.Node().Role(ui.Role_ROLE_STATIC_TEXT).Name(videoName).Ancestor(filesWindowFinder).Finder()

	filesOpenButtonFinder := ui.Node().Role(ui.Role_ROLE_BUTTON).Name("Open").Ancestor(filesWindowFinder).Finder()

	galleryWindowName := fmt.Sprintf("Gallery - %s", videoName)
	galleryWindowFinder := ui.Node().Role(ui.Role_ROLE_WINDOW).Name(galleryWindowName).Finder()

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
	gallerFullScreenButtonFinder := ui.Node().Name("Toggle fullscreen").Role(ui.Role_ROLE_BUTTON).Finder()

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: gallerFullScreenButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for full screen button to show")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: gallerFullScreenButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to click on full screen button")
	}

	galleryPlayButtonFinder := ui.Node().Name("Toggle play pause").Role(ui.Role_ROLE_BUTTON).Finder()

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: galleryPlayButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for play button to show")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: galleryPlayButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to click on play button")
	}

	return nil
}

// CloseWindow closes the certain window using keyboard accelerators.
func CloseWindow(ctx context.Context, keyboardSvc inputspb.KeyboardServiceClient, uiautoSvc ui.AutomationServiceClient, windowName string) error {
	windowFinder := ui.Node().Role(ui.Role_ROLE_WINDOW).Name(windowName).Finder()

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
	connectButtonNode := ui.Node().NameContaining("fullscreen").Role(ui.Role_ROLE_BUTTON).Finder()

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
	fileNameFinder := ui.Node().Role(ui.Role_ROLE_STATIC_TEXT).Name(fileName).Ancestor(FilesWindowFinder).Finder()

	galleryWindow := fmt.Sprintf("Gallery - %s", fileName)
	galleryWindowFinder := ui.Node().Role(ui.Role_ROLE_WINDOW).Name(galleryWindow).Finder()

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
	window := ui.Node().Role(ui.Role_ROLE_WINDOW).Name(windowName).Nth(0).Finder()
	maxButton := ui.Node().Name("Maximize").Role(ui.Role_ROLE_BUTTON).Nth(0).Ancestor(window).Finder()

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: maxButton}); err != nil {
		return errors.Wrap(err, "failed to wait for maximize button from context menu")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: maxButton}); err != nil {
		return errors.Wrap(err, "failed to click on maximize button from context menu")
	}

	return nil
}

// ClickPowerSourceOption clicks the option after open power source combobox.
func ClickPowerSourceOption(ctx context.Context, uiautoSvc ui.AutomationServiceClient, comboBoxFinder, optionFinder *ui.Finder) error {
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: comboBoxFinder}); err != nil {
		return errors.Wrap(err, "wait power source exists")
	}
	if _, err := uiautoSvc.DoDefault(ctx, &ui.DoDefaultRequest{Finder: comboBoxFinder}); err != nil {
		return errors.Wrap(err, "click power source combobox")
	}
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: optionFinder}); err != nil {
		return errors.Wrap(err, "click power source option")
	}
	return nil
}

// OpenPowerSettings opens the power settings page.
func OpenPowerSettings(ctx context.Context, cs ui.ChromeServiceClient, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient) error {
	launchAppTimeout := 60

	deviceFinder := ui.Node().Name("Device").Nth(0).Finder()
	powerFinder := ui.Node().Name("Power").Nth(0).Finder()

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Settings", TimeoutSecs: int32(launchAppTimeout)}); err != nil {
		return errors.Wrap(err, "launch settings app")
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: deviceFinder}); err != nil {
		return errors.Wrap(err, "wait device button")
	}
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: deviceFinder}); err != nil {
		return errors.Wrap(err, "click device button")
	}
	// GoBigSleepLint. Sleep is 2 second time, ensure power button position is correct.
	testing.Sleep(ctx, 2*time.Second)
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: powerFinder}); err != nil {
		return errors.Wrap(err, "wait power button")
	}
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: powerFinder}); err != nil {
		return errors.Wrap(err, "click power button")
	}
	return nil
}

// WaitUntilBatteryStatus wait untils the battery status based on ui name.
func WaitUntilBatteryStatus(ctx context.Context, uiautoSvc ui.AutomationServiceClient, isCharging bool) error {
	const (
		CheckTimeout, CheckInterval    = 10 * time.Second, 200 * time.Millisecond
		batteryCharging, batteryIsFull = "charging", "Battery is full"
	)
	batteryInfo := ui.Node().HasClass("PowerTrayView").Nth(0).Finder()

	return testing.Poll(ctx, func(ctx context.Context) error {
		batteryInformation, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: batteryInfo})
		if err != nil {
			return errors.Wrap(err, "could not found battery info")
		}
		infoName := batteryInformation.NodeInfo.Name
		statusCharging := strings.Contains(infoName, batteryCharging)
		statusFull := strings.Contains(infoName, batteryIsFull)
		statusChargingOrFull := statusCharging || statusFull

		if isCharging && !statusChargingOrFull {
			return errors.New("battery not charging")
		} else if !isCharging && statusChargingOrFull {
			return errors.New("battery is charging")
		}
		return nil
	}, &testing.PollOptions{Timeout: CheckTimeout, Interval: CheckInterval})
}

// OpenDeviceSettings opens the device settings sub page.
func OpenDeviceSettings(ctx context.Context, subItem string, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient) error {
	launchAppTimeout := 60

	deviceFinder := ui.Node().Name("Device").Nth(0).Finder()
	subItemFinder := ui.Node().Name(subItem).Nth(0).Finder()

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Settings", TimeoutSecs: int32(launchAppTimeout)}); err != nil {
		return errors.Wrap(err, "launch settings app")
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: deviceFinder}); err != nil {
		return errors.Wrap(err, "wait device button")
	}
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: deviceFinder}); err != nil {
		return errors.Wrap(err, "click device button")
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: subItemFinder}); err != nil {
		return errors.Wrapf(err, "wait %s button", subItem)
	}
	if _, err := uiautoSvc.DoDefault(ctx, &ui.DoDefaultRequest{Finder: subItemFinder}); err != nil {
		return errors.Wrapf(err, "click %s button", subItem)
	}
	return nil
}

// OpenFileInDownloads opens the file on downloads folder.
func OpenFileInDownloads(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, fileName, windowTitle string) error {
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		return errors.Wrap(err, "failed to launch files app")
	}

	fileNameFinder := ui.Node().Name(fileName).Role(ui.Role_ROLE_STATIC_TEXT).Nth(0).Finder()
	downloadsFinder := ui.Node().Name("Downloads").Nth(0).Finder()

	for _, finder := range []*ui.Finder{
		downloadsFinder,
		fileNameFinder,
	} {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: finder}); err != nil {
			return errors.Wrapf(err, "failed to wait until the %q exists", finder)
		}
		if _, err := uiautoSvc.DoubleClick(ctx, &ui.DoubleClickRequest{Finder: finder}); err != nil {
			return errors.Wrapf(err, "failed to click on the %q", finder)
		}
	}

	Window := ui.Node().Name(windowTitle).Role(ui.Role_ROLE_WINDOW).Nth(0).Finder()
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: Window}); err != nil {
		return errors.Wrap(err, "failed to wait until html window exist")
	}
	return nil
}

// VerifyGuestLearnMore Verify that the guest help page is shown for Learn More.
func VerifyGuestLearnMore(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Chrome"}); err != nil {
		return errors.Wrap(err, "failed to launch chrome")
	}
	defer appsSvc.CloseApp(cleanupCtx, &pb.CloseAppRequest{AppName: "Chrome"})
	learnMoreFinder := ui.Node().Name("Learn more").Nth(0).Finder()
	helpPageFinder := ui.Node().NameContaining("Use a Chromebook as a guest").Role(ui.Role_ROLE_STATIC_TEXT).Nth(0).Finder()
	for _, finder := range []*ui.Finder{
		learnMoreFinder,
		helpPageFinder,
	} {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: finder}); err != nil {
			return errors.Wrap(err, "failed to wait for exists")
		}
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: finder}); err != nil {
			return errors.Wrap(err, "failed to click")
		}
	}
	return nil
}

// DumpUITreeWithScreenshotToFile dumps the UI tree and screenshot at the time of error to faillog
func DumpUITreeWithScreenshotToFile(ctx context.Context, conn *grpc.ClientConn, filePrefix string) error {
	svc := ui.NewChromeUIServiceClient(conn)
	if _, err := svc.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{FilePrefix: filePrefix}); err != nil {
		return err
	}
	return nil
}
