// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package meetcuj contains meetcuj utility functions for Meet/MeetMultitasking CUJ.
package meetcuj

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// BlurBackgroundFinder is the finder of blurring background effect.
	BlurBackgroundFinder = nodewith.Name("Blur your background").Role(role.ToggleButton).Focusable()
	// TurnOffEffectsFinder is the finder of turning off visual effects.
	TurnOffEffectsFinder = nodewith.Name("Turn off visual effects").Focusable()
	meetRootWebArea      = nodewith.NameContaining("Meet").Role(role.RootWebArea)
)

type zoomLevel struct {
	// percentage is the zoom percentage.
	percentage int
	// presses is the number of Ctrl+Minus keystrokes require to reach the zoom from 100%.
	presses int
}

var zoomLevels = []*zoomLevel{
	{100, 0},
	{90, 1},
	{80, 2},
	{75, 3},
	{67, 4},
	{50, 5},
}

// AddBots adds |numBots| bots to the call.
func AddBots(ctx context.Context, bc *bond.Client, numBots int, testDuration time.Duration, meetingCode string, botsOptions []bond.AddBotsOption) error {
	const (
		// The addBotTimeout allows 3 2-minute BondAPI request retries by the
		// Bond lib.
		addBotTimeout = 6*time.Minute + 10*time.Second
		// addBotRetries is the local retry number for adding bots.
		addBotRetries = 3

		wait = 100 * time.Millisecond
	)

	testing.ContextLogf(ctx, "Adding %d bots to the call", numBots)

	if numBots == 0 {
		return nil
	}

	sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
	defer cancel()

	botsToAdd := numBots
	// Add 30 minutes to the bot duration, to ensure that the bots stay long
	// enough for the test to get info from chrome://webrtc-internals.
	botDuration := testDuration + 30*time.Minute
	// Add bots that requests HD video.
	botsOptions = append(botsOptions, bond.WithHDVideo())
	for i := 0; i < addBotRetries; i++ {
		// GoBigSleepLint: A short sleep before next call to Bond API.
		if err := testing.Sleep(ctx, wait); err != nil {
			return errors.Wrapf(err, "failed to sleep for %v", wait)
		}
		botList, numFailures, err := bc.AddBots(sctx, meetingCode, botsToAdd, botDuration, botsOptions...)
		if err != nil {
			return errors.Wrapf(err, "failed to create %d bots", botsToAdd)
		}
		testing.ContextLogf(ctx, "%d bots started, %d bots failed", len(botList), numFailures)
		botsToAdd -= len(botList)
		if botsToAdd <= 0 {
			break
		}
	}
	if botsToAdd > 0 {
		return errors.Errorf("failed to add all %d bots to the call; %d to be added after %d retries", numBots, botsToAdd, addBotRetries)
	}
	return nil
}

// ResetZoom resets the browser zoom to 100%.
func ResetZoom(ui *uiauto.Context, kw *input.KeyboardEventWriter) uiauto.Action {
	zoomNode := nodewith.HasClass("ZoomBubbleView").Role(role.Window)
	return uiauto.NamedCombine(
		"reset zoom and wait for zoom indicator to be absent",
		ui.LeftClick(meetRootWebArea),
		kw.AccelAction("Ctrl+0"),
		ui.WaitUntilGone(zoomNode),
	)
}

// determineZoomLevel calculates the optimal browser zoom level to ensure
// the window width is not smaller than the expected width.
func determineZoomLevel(ctx context.Context, windowWidth, expectedWidth float64) (*zoomLevel, error) {
	// Calculate the maximum zoom percentage that still satisfies the width threshold.
	targetZoomPercentage := windowWidth * 100 / expectedWidth
	testing.ContextLogf(ctx, "Window width: %.2f, expected width: %.2f, target zoom percentage: %.2f", windowWidth, expectedWidth, targetZoomPercentage)

	for _, level := range zoomLevels {
		if float64(level.percentage) <= targetZoomPercentage {
			return level, nil
		}
	}
	return nil, errors.Errorf("window width %.2f is too small to reach the expected width %.2f", windowWidth, expectedWidth)
}

// setBrowserZoomLevel sets the browser zoom to the specified level
// and verifies the change in the Chrome UI.
func setBrowserZoomLevel(ctx context.Context, kw *input.KeyboardEventWriter, ui *uiauto.Context, zoom *zoomLevel) error {
	if zoom.percentage == 100 {
		return nil
	}
	expectedZoom := fmt.Sprintf("%d%%", zoom.percentage)
	if err := inputsimulations.RepeatKeyPress(ctx, kw, "Ctrl+-", 3*time.Second, zoom.presses); err != nil {
		return errors.Wrapf(err, "failed to repeatedly press Ctrl+Minus to zoom out to %s", expectedZoom)
	}

	browserAppMenuButton := nodewith.Name("Chrome").HasClass("BrowserAppMenuButton").First()
	zoomMenuItem := nodewith.Name("Zoom").Role(role.MenuItem)
	zoomValueNode := nodewith.Role(role.StaticText).Ancestor(zoomMenuItem)
	if err := ui.LeftClickUntil(browserAppMenuButton,
		ui.WithTimeout(3*time.Second).WaitUntilExists(zoomMenuItem),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open browser app menu")
	}

	// Get zoom value text.
	zoomInfo, err := ui.Info(ctx, zoomValueNode)
	if err != nil {
		return errors.Wrap(err, "failed to find the current browser zoom")
	}
	if zoomInfo.Name != expectedZoom {
		return errors.Errorf("unexpected zoom value: got %s; want %s", zoomInfo.Name, expectedZoom)
	}
	if err := ui.LeftClickUntil(browserAppMenuButton,
		ui.WithTimeout(3*time.Second).WaitUntilGone(zoomMenuItem),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close browser app menu")
	}

	testing.ContextLog(ctx, "Zoomed browser window to ", expectedZoom)
	return nil
}

// SetBrowserZoomToFitWidth calculates the optimal zoom level for the given window width
// and target width, and then applies the zoom to the browser.
func SetBrowserZoomToFitWidth(ctx context.Context, kw *input.KeyboardEventWriter, ui *uiauto.Context, windowWidth, expectedWidth float64) error {
	zoomLevel, err := determineZoomLevel(ctx, windowWidth, expectedWidth)
	if err != nil {
		return errors.Wrap(err, "failed to determine the zoom level")
	}
	if err := setBrowserZoomLevel(ctx, kw, ui, zoomLevel); err != nil {
		return errors.Wrap(err, "failed to set the browser zoom level")
	}
	return nil
}

// SetVisualEffects sets the given visual effect.
func SetVisualEffects(ui *uiauto.Context, effect *nodewith.Finder) uiauto.Action {
	uiLongWait := ui.WithTimeout(time.Minute)
	effectsItem := nodewith.Name("Backgrounds and effects").Role(role.MenuItem)
	effectsHeading := nodewith.Name("Backgrounds and effects").Role(role.Heading).Ancestor(meetRootWebArea)
	sidePanel := nodewith.Name("Side panel").Role(role.Complementary).Ancestor(meetRootWebArea)
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(sidePanel).Focusable().First()
	openEffectsPanel := uiauto.NamedCombine("open effects panel",
		// Open the "More options" popup, and wait until we see
		// "Backgrounds and effects".
		doDefaultMoreOptions(ui),
		// Open the visual effects panel.
		ui.WithTimeout(30*time.Second).DoDefault(effectsItem),
		ui.WithTimeout(30*time.Second).WaitUntilExists(effectsHeading),
	)

	return uiauto.NamedCombine(
		fmt.Sprintf("set effect with node %v", effect.Pretty()),
		uiauto.Retry(2, openEffectsPanel),
		toggleEffect(ui, effect),
		// Close the visual effects panel.
		ui.DoDefault(closeButton),
		uiLongWait.WaitUntilGone(effect),
	)
}

func doDefaultMoreOptions(ui *uiauto.Context) uiauto.Action {
	moreOptionsFinder := nodewith.Name("More options").Role(role.PopUpButton)
	callOptionsMenu := nodewith.Name("Call options").Role(role.Menu)
	return func(ctx context.Context) error {
		moreOptionsButtons, err := ui.NodesInfo(ctx, moreOptionsFinder)
		if err != nil || len(moreOptionsButtons) < 1 {
			return errors.Wrap(err, "failed to find more options button")
		}
		// Sometimes, the UI has two identical "More Options" buttons, which requires
		// selecting the last one to be the correct button.
		return ui.DoDefaultUntil(moreOptionsFinder.Nth(len(moreOptionsButtons)-1),
			ui.WithTimeout(5*time.Second).WaitUntilExists(callOptionsMenu),
		)(ctx)
	}
}

func toggleEffect(ui *uiauto.Context, effect *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		if effect == TurnOffEffectsFinder {
			toggleButton := TurnOffEffectsFinder.Role(role.ToggleButton)
			popUpButton := TurnOffEffectsFinder.Role(role.PopUpButton)

			turnOffEffectsButton, err := ui.FindAnyExists(ctx, toggleButton, popUpButton)
			if err != nil {
				return errors.Wrap(err, "failed to find 'Turn off visual effects' button")
			}
			if turnOffEffectsButton == popUpButton {
				nodeInfo, err := ui.Info(ctx, turnOffEffectsButton)
				if err != nil {
					return errors.Wrap(err, "failed to find 'Turn off visual effects' button")
				}
				if nodeInfo.Description == "No effects applied" {
					return nil
				}
				removeAllItem := nodewith.Name("Remove all").Role(role.MenuItem)
				return uiauto.Combine("turn off visual effects",
					ui.LeftClick(popUpButton),
					ui.LeftClick(removeAllItem))(ctx)
			}
		}
		return ui.WithTimeout(time.Minute).DoDefaultUntil(effect,
			ui.WithTimeout(5*time.Second).WaitUntilCheckedState(effect, true))(ctx)
	}
}

// WaitForParticipantInfoLoaded waits for the participant info to be loaded.
func WaitForParticipantInfoLoaded(ui *uiauto.Context) uiauto.Action {
	gotItButton := nodewith.NameContaining("Got it").Role(role.Button)
	meetRootWebArea := nodewith.NameContaining("Meet").Role(role.RootWebArea)
	peopleButton := nodewith.Name("People").Role(role.Button).Ancestor(meetRootWebArea)
	participantRegex := regexp.MustCompile(`People\s*-\s*(\d+)\s*joined`)
	participantButton := nodewith.NameRegex(participantRegex).Role(role.Button).Ancestor(meetRootWebArea)
	return uiauto.NamedCombine("wait for the number of participants to be loaded",
		// Some DUT models have poor performance. When joining a large conference
		// (over 15 participants), it would take much time to render DOM elements.
		// Set a longer timer here.
		ui.WithTimeout(time.Minute).WaitUntilAnyExists(gotItButton, participantButton, peopleButton),
		uiauto.IfSuccessThen(ui.Exists(gotItButton), ui.DoDefault(gotItButton)),
		ui.WithTimeout(time.Minute).WaitUntilAnyExists(participantButton, peopleButton),
	)
}

// GrantPermissionsInMeet grants all the potential permissions prompt in Meet.
func GrantPermissionsInMeet(tconn *chrome.TestConn) uiauto.Action {
	return prompts.ClearPotentialPrompts(
		tconn,
		time.Minute,
		prompts.ShowNotificationsPrompt,
		prompts.AllowAVPermissionPrompt,
		prompts.AllowMicrophoneAndCameraPermissionPrompt,
		prompts.OthersSeeDiffPrompt,
	)
}

// SetupFakeCameraHAL sets up the fake camera HAL.
func SetupFakeCameraHAL(ctx context.Context, videoFilePath string, supportedFormats []*testutil.FakeCameraFormatsConfig) (cleanup func(context.Context), retErr error) {
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Configure CrOS to use only fake HAL camera.
	if err := testutil.SetupTestConfig(ctx, testutil.UseFakeHALCamera); err != nil {
		return nil, errors.Wrap(err, "failed to set up camera test config")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			testutil.RemoveTestConfig(ctx)
		}
	}(closeCtx)

	// Copy the fake camera video to where the camera module can access.
	dutFakeHALPath, err := testutil.CopyFakeHALFrameImage(videoFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to copy fake camera input")
	}
	defer func() {
		if retErr != nil {
			os.Remove(dutFakeHALPath)
		}
	}()

	// Write the fake HAL config to the system.
	fakeCameraConfig := testutil.FakeCameraConfig{
		ID:        1,
		Connected: true,
		Frames: &testutil.FakeCameraImageConfig{
			Path: dutFakeHALPath,
		},
		SupportedFormats: supportedFormats,
	}
	fakeHALConfig := testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{fakeCameraConfig},
	}
	if err := testutil.WriteFakeHALConfig(ctx, fakeHALConfig); err != nil {
		return nil, errors.Wrap(err, "failed to configure HAL camera")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			testutil.RemoveFakeHALConfig(ctx)
		}
	}(closeCtx)

	const cameraService = "cros-camera"
	if err := upstart.RestartJob(ctx, cameraService); err != nil {
		return nil, errors.Wrapf(err, "failed to restart %s after camera setup", cameraService)
	}

	return func(ctx context.Context) {
		testutil.RemoveFakeHALConfig(ctx)
		os.Remove(dutFakeHALPath)
		testutil.RemoveTestConfig(ctx)
		upstart.RestartJob(ctx, cameraService)
	}, nil
}

// GetDisabledExperiments gets the list of partially rolled out experiments,
// that should be disabled in Meet tests.
func GetDisabledExperiments(ctx context.Context, cloudStorage *testing.CloudStorage) ([]string, error) {
	reader, err := cloudStorage.Open(ctx, "gs://chromeos-test-assets-partner-shared/tast/crosint/meet-experiments/partial-rollout-experiments.txt")
	if err != nil {
		return nil, errors.Wrap(err, "failed to download experiment list")
	}
	defer reader.Close()

	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read experiments list")
	}

	return strings.Split(strings.TrimSpace(string(b)), "\n"), nil
}
