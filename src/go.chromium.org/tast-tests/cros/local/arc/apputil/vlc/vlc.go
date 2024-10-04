// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vlc contains local Tast tests that exercise vlc.
package vlc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// AppName is the name of ARC app.
	AppName = "VLC"
	// PackageName is the package name of ARC app.
	PackageName = "org.videolan.vlc"
	version     = "3.4.2"
	idPrefix    = "org.videolan.vlc:id/"

	titleID      = idPrefix + "title"
	navDirID     = idPrefix + "nav_directories"
	doneBtnID    = idPrefix + "doneButton"
	nextBtnID    = idPrefix + "nextButton"
	playerRootID = idPrefix + "player_root"
	closeTipsID  = idPrefix + "close"

	shortTimeout   = 5 * time.Second
	defaultTimeout = 15 * time.Second
	longTimeout    = 2 * time.Minute
)

// MediaType represents the type of media to be played by the vlc player.
type MediaType int

// Media types supported by Vlc player.
const (
	Video MediaType = 0
	Audio MediaType = 1
)

// Vlc holds resources of ARC app VLC player.
type Vlc struct {
	app *apputil.App
}

// MediaInfo holds information of a media file.
type MediaInfo struct {
	FileName string    // Name of the media file.
	FileType MediaType // Type of the media file
}

// NewVLCPlayer returns VLC instance.
func NewVLCPlayer(ctx context.Context, cr *chrome.Chrome, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device) (*Vlc, error) {
	app, err := apputil.NewApp(ctx, kb, tconn, a, d, AppName, PackageName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create arc resource")
	}
	vlcPlayer := &Vlc{app}
	if err := vlcPlayer.Install(ctx, cr); err != nil {
		return nil, errors.Wrap(err, "failed to install VLC app")
	}
	return vlcPlayer, nil
}

// Install downloads and installs the Vlc player APK from
// "https://get.videolan.org/vlc-android", because the version installed
// from the play store will be inconsistent under different accounts.
// If the wrong version is installed, it will reinstall.
func (vlc *Vlc) Install(ctx context.Context, cr *chrome.Chrome) error {
	isInstalled, err := vlc.app.ARC.PackageInstalled(ctx, vlc.app.PkgName)
	if err != nil {
		return errors.Wrap(err, "failed to find if package is installed")
	}
	if isInstalled {
		ver, err := vlc.app.GetVersion(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get installed version")
		}
		if ver == version {
			return nil
		}
		testing.ContextLogf(ctx, "Version %s has been installed, reinstall version %s", ver, version)
		if err := vlc.app.ARC.Uninstall(ctx, vlc.app.PkgName); err != nil {
			return errors.Wrapf(err, "failed to uninstall the wrong version %s", ver)
		}
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to retrieve users Downloads path")
	}
	apkName, err := vlc.getApkName(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get apk name")
	}

	// Downloading the APK by "curl" rather than UI (e.g. Chrome browser) so that
	// this package doesn't need the lacros variant.
	// The apk download link in "https://get.videolan.org/vlc-android" is redirected.
	// To address this, the -LO argument needs to be specified to ensure the download
	// of the complete original file along with the filename.
	cmd := testexec.CommandContext(ctx, "curl", "-LO", fmt.Sprintf("https://get.videolan.org/vlc-android/%s/%s", version, apkName))
	// The curl doesn't have an option to specify the destination path, so
	// change work dir to user's Downloads directory to download result to it
	cmd.Dir = downloadsPath

	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to download apk file")
	}

	apkPath := filepath.Join(downloadsPath, apkName)
	// Defer call remove apk file in advance to make sure the apk will be deleted.
	defer os.Remove(apkPath)

	return vlc.app.ARC.Install(ctx, apkPath, adb.InstallOptionGrantPermissions)
}

// getApkName gets the name of the APK file to install on the DUT.
func (vlc *Vlc) getApkName(ctx context.Context) (string, error) {
	out, err := vlc.app.ARC.Command(ctx, "getprop", "ro.product.cpu.abi").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get abi: %s", string(out))
	}
	arch := "x86_64"
	if strings.HasPrefix(string(out), "arm64-v8a") {
		arch = "arm64-v8a"
	}
	return fmt.Sprintf("VLC-Android-%s-%s.apk", version, arch), nil
}

// Launch launches ARC app VLC.
func (vlc *Vlc) Launch(ctx context.Context) error {
	testing.ContextLogf(ctx, "Openning app: %q", AppName)
	if _, err := vlc.app.Launch(ctx); err != nil {
		return errors.Wrap(err, "failed to launch App")
	}

	return vlc.clearStartupPrompt(ctx)
}

// Close closes ARC app VLC player and cleanup resources.
func (vlc *Vlc) Close(ctx context.Context, cr *chrome.Chrome, hasError func() bool, outDir string) error {
	return vlc.app.Close(ctx, cr, hasError, outDir)
}

// EnterAudioFolder enters into audio folder.
func (vlc *Vlc) EnterAudioFolder(ctx context.Context) error {
	testing.ContextLog(ctx, "Navigate to vlc audio folder")
	return uiauto.Combine("navigate to vlc audio folder",
		apputil.FindAndClick(vlc.app.Device.Object(ui.ID(navDirID)), shortTimeout),
		apputil.FindAndClick(vlc.app.Device.Object(ui.ID(titleID), ui.Text("Download")), shortTimeout),
		apputil.FindAndClick(vlc.app.Device.Object(ui.ID(titleID), ui.Text("audios")), shortTimeout),
	)(ctx)
}

// EnterDownloadFolder enters into download folder.
func (vlc *Vlc) EnterDownloadFolder(ctx context.Context) error {
	return uiauto.NamedCombine("Navigate to vlc download folder",
		apputil.FindAndClick(vlc.app.Device.Object(ui.ID(navDirID)), longTimeout),
		apputil.FindAndClick(vlc.app.Device.Object(ui.ID(titleID), ui.Text("Download")), longTimeout),
	)(ctx)
}

// Play plays the media file.
func (vlc *Vlc) Play(ctx context.Context, mediaInfo *MediaInfo) error {
	testing.ContextLog(ctx, "Click on file")
	fileUIObject := vlc.app.Device.Object(ui.TextContains(mediaInfo.FileName))
	if err := apputil.FindAndClick(fileUIObject, defaultTimeout)(ctx); err != nil {
		return errors.Wrapf(err, "failed to find the target file: %s", mediaInfo.FileName)
	}

	switch mediaInfo.FileType {
	case Video:
		if err := apputil.ClickIfExist(vlc.app.Device.Object(ui.ID(playerRootID)), shortTimeout)(ctx); err != nil {
			return errors.Wrap(err, "failed to show control panel")
		}

		testing.ContextLog(ctx, "Close tips")
		closeTips := vlc.app.Device.Object(ui.ID(closeTipsID))
		if err := apputil.ClickIfExist(closeTips, shortTimeout)(ctx); err != nil {
			return errors.Wrap(err, "failed to close tips")
		}
	case Audio:
		if err := vlc.clearPromptAfterPlay(ctx); err != nil {
			return errors.Wrap(err, "failed to clear prompt after play")
		}

		testing.ContextLog(ctx, "Verify playing filename")
		playingFilename := vlc.app.Device.Object(ui.ID(titleID), ui.TextContains(mediaInfo.FileName))
		if err := playingFilename.WaitForExists(ctx, defaultTimeout); err != nil {
			return errors.Wrap(err, "the VLC player is not playing")
		}

		testing.ContextLog(ctx, "Wait for pause button")
		playPauseID := idPrefix + "header_play_pause"
		pauseButton := vlc.app.Device.Object(ui.ID(playPauseID), ui.Description("Pause"))
		if err := pauseButton.WaitForExists(ctx, defaultTimeout); err != nil {
			return errors.Wrap(err, "the VLC player is not playing")
		}
	}

	testing.ContextLogf(ctx, "Start playing media file: %s", mediaInfo.FileName)

	return nil
}

func (vlc *Vlc) clearAllYesButton(ctx context.Context) error {
	yesButton := vlc.app.Device.Object(ui.Text("YES"))
	if err := apputil.WaitForExists(yesButton, shortTimeout)(ctx); err != nil {
		return nil
	}
	if err := apputil.FindAndClick(yesButton, shortTimeout)(ctx); err != nil {
		return err
	}

	return vlc.clearAllYesButton(ctx)
}

func (vlc *Vlc) clearStartupPrompt(ctx context.Context) error {
	// If app messages appear, click it.
	testing.ContextLog(ctx, "Clear start up prompt")
	startBtn := vlc.app.Device.Object(ui.ID(idPrefix + "startButton"))
	permissionBtn := vlc.app.Device.Object(ui.ID(idPrefix + "grantPermissionButton"))

	return uiauto.IfSuccessThen(
		apputil.WaitForExists(startBtn, defaultTimeout),
		uiauto.Combine("clear start up prompt",
			apputil.ClickIfExist(startBtn, shortTimeout),
			apputil.ClickIfExist(permissionBtn, shortTimeout),
			apputil.ClickIfExist(vlc.app.Device.Object(ui.Text("ALLOW")), shortTimeout),
			apputil.ClickIfExist(vlc.app.Device.Object(ui.ID(nextBtnID)), shortTimeout),
			apputil.ClickIfExist(vlc.app.Device.Object(ui.ID(doneBtnID)), shortTimeout),
			vlc.clearAllYesButton,
		),
	)(ctx)
}

func (vlc *Vlc) clearPromptAfterPlay(ctx context.Context) error {
	testing.ContextLog(ctx, "Clear instruction prompt")
	nextButton := vlc.app.Device.Object(ui.ID(nextBtnID))

	// The multi-step prompt has the same button object. Use for loop to reduce code.
	for i := 0; i < 3; i++ {
		if err := apputil.ClickIfExist(nextButton, defaultTimeout)(ctx); err != nil {
			return err
		}
	}
	return nil
}

// IsPaused check if the player is paused.
func (vlc *Vlc) IsPaused(ctx context.Context) error {
	playPauseID := idPrefix + "header_play_pause"
	playButton := vlc.app.Device.Object(ui.ID(playPauseID), ui.Description("Play"))
	if err := playButton.Exists(ctx); err != nil {
		errors.Wrap(err, "play button not found, player is not paused")
	}
	return nil
}

// IsPlaying check if the player is playing.
func (vlc *Vlc) IsPlaying(ctx context.Context) error {
	playPauseID := idPrefix + "header_play_pause"
	playButton := vlc.app.Device.Object(ui.ID(playPauseID), ui.Description("Pause"))
	if err := playButton.Exists(ctx); err != nil {
		errors.Wrap(err, "pause button not found, player is not playing")
	}
	return nil
}
