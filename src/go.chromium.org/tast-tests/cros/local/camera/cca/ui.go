// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cca provides utilities to interact with Chrome Camera App.
package cca

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// A11yRootNode represents the root node of Camera app in A11y tree.
	A11yRootNode = nodewith.Name("Camera").Role(role.RootWebArea)
	// A11yCanvasNode represents the canvas node in A11y tree.
	A11yCanvasNode = nodewith.Role(role.Canvas).Ancestor(A11yRootNode)
)

// The following types are defined corresponding to definitions in
// /js/test/cca_type.ts in CCA side.

// UIComponentName represents a name of UI component.
type UIComponentName string

// SettingMenu is the setting menu in CCA.
type SettingMenu string

// Option is the option for toggling state.
type Option string

// List of UI components used in CCA for testing.
const (
	// BackAspectRatioOptions are the buttons of aspect ratio options for the back camera.
	BackAspectRatioOptions UIComponentName = "backAspectRatioOptions"
	// BackPhotoResolutionOptions are the buttons of photo resolution options for the back camera.
	BackPhotoResolutionOptions UIComponentName = "backPhotoResolutionOptions"
	// BackVideoResolutionOptions are the buttons of video resolution options for the back camera.
	BackVideoResolutionOptions UIComponentName = "backVideoResolutionOptions"
	// BarcodeChipText is chip for text detected from barcode.
	BarcodeChipText UIComponentName = "barcodeChipText"
	// BarcodeChipURL is chip for url detected from barcode.
	BarcodeChipURL UIComponentName = "barcodeChipURL"
	// BarcodeChipURL is chip for url detected from barcode.
	BarcodeChipWifi UIComponentName = "barcodeChipWifi"
	// BarcodeCopyTextButton is button to copy text detected from barcode.
	BarcodeCopyTextButton UIComponentName = "barcodeCopyTextButton"
	// BarcodeCopyURLButton is button to copy url detected from barcode.
	BarcodeCopyURLButton UIComponentName = "barcodeCopyURLButton"
	// BitrateMultiplierRangeInput is range input for selecting bitrate multiplier.
	BitrateMultiplierRangeInput UIComponentName = "bitrateMultiplierRangeInput"
	// CancelResultButton is button for canceling intent review result.
	CancelResultButton UIComponentName = "cancelResultButton"
	// ConfirmResultButton is button for confirming intent review result.
	ConfirmResultButton UIComponentName = "confirmResultButton"
	// DocumentAddPageButton is the button to close the review UI of multi-page document mode temporarily for adding new pages.
	DocumentAddPageButton UIComponentName = "documentAddPageButton"
	// DocumentBackButton is the resume button to show review UI of multi-page document mode when there're pending pages for reviewing.
	DocumentBackButton UIComponentName = "documentBackButton"
	// DocumentCancelButton is the cancel button in multi-page document mode.
	DocumentCancelButton UIComponentName = "documentCancelButton"
	// DocumentCorner are corners drawn around the document boundary corner.
	DocumentCorner UIComponentName = "documentCorner"
	// DocumentDoneFixButton is the exit button of fix mode in multi-page document mode.
	DocumentDoneFixButton UIComponentName = "documentDoneFixButton"
	// DocumentFixButton is the entry button of fix mode in multi-page document mode.
	DocumentFixButton UIComponentName = "documentFixButton"
	// DocumentFixModeCorner is the crop area dragging point in fix mode in multi-page document mode.
	DocumentFixModeCorner UIComponentName = "documentFixModeCorner"
	// DocumentFixModeImage is the preview image of fix mode in multi-page document mode.
	DocumentFixModeImage UIComponentName = "documentFixModeImage"
	// DocumentPreviewModeImage is the preview image of preview mode in multi-page document mode.
	DocumentPreviewModeImage UIComponentName = "documentPreviewModeImage"
	// DocumentReview is the review view for multi-page document mode.
	DocumentReview UIComponentName = "documentReview"
	// DocumentSaveAsPdfButton is the button save as a PDF file in multi-page document mode.
	DocumentSaveAsPdfButton UIComponentName = "documentSaveAsPdfButton"
	// DocumentSaveAsPhotoButton is the button to save as a photo in multi-page document mode.
	DocumentSaveAsPhotoButton UIComponentName = "documentSaveAsPhotoButton"
	// FeedbackButton is the feedback button showing in the settings menu.
	FeedbackButton UIComponentName = "feedbackButton"
	// FPS60Buttons are the 60 FPS buttons in the video resolution menu.
	FPS60Buttons UIComponentName = "fps60Buttons"
	// FrontAspectRatioOptions are the buttons of aspect ratio options for the front camera.
	FrontAspectRatioOptions UIComponentName = "frontAspectRatioOptions"
	// FrontPhotoResolutionOptions are the buttons of photo resolution options for the front camera.
	FrontPhotoResolutionOptions UIComponentName = "frontPhotoResolutionOptions"
	// FrontVideoResolutionOptions are the buttons of video resolution options for the front camera.
	FrontVideoResolutionOptions UIComponentName = "frontVideoResolutionOptions"
	// GalleryButton is button for entering the Backlight app as a gallery for captured files.
	GalleryButton UIComponentName = "galleryButton"
	// GalleryButtonCover is cover photo of gallery button.
	GalleryButtonCover UIComponentName = "galleryButtonCover"
	// GifRecordingOption is the radio button to toggle gif recording option.
	GifRecordingOption UIComponentName = "gifRecordingOption"
	// GifReviewRetakeButton is the retake button in gif review page.
	GifReviewRetakeButton UIComponentName = "gifReviewRetakeButton"
	// GifReviewSaveButton is the save button in gif review page.
	GifReviewSaveButton UIComponentName = "gifReviewSaveButton"
	// GridOptionGoldenRatio is an option to enable grid of type golden ratio.
	GridOptionGoldenRatio UIComponentName = "gridOptionGoldenRatio"
	// HelpButton is the help button showing in the settings menu.
	HelpButton UIComponentName = "helpButton"
	// LowStorageDialog is the dialog displayed when there's an unexpected behavior during recording due to low storage.
	LowStorageDialog UIComponentName = "lowStorageDialog"
	// LowStorageDialogManageButton is the button in LowStorageDialog that navigates users to "Manage storage" page in system settings.
	LowStorageDialogManageButton UIComponentName = "lowStorageDialogManageButton"
	// LowStorageDialogOKButton is the button labeled "OK" in LowStorageDialog, used to acknowledge and close the dialog.
	LowStorageDialogOKButton UIComponentName = "lowStorageDialogOKButton"
	// LowStorageWarning is the warning nudge displayed while recording on device with low storage.
	LowStorageWarning UIComponentName = "lowStorageWarning"
	// MirrorOptionOff is an option to disable mirror preview.
	MirrorOptionOff UIComponentName = "mirrorOptionOff"
	// MirrorOptionOff is an option to enable mirror preview.
	MirrorOptionOn UIComponentName = "mirrorOptionOn"
	// ModeSelector is the selection bar for different capture modes.
	ModeSelector UIComponentName = "modeSelector"
	// OpenGridPanelButton is the button which is used for opening the grid type settings panel.
	OpenGridPanelButton UIComponentName = "openGridPanelButton"
	// OpenMirrorPanelButton is the button which is used for opening the mirror state settings panel.
	OpenMirrorPanelButton UIComponentName = "openMirrorPanelButton"
	// OpenPTZPanelButton is the button for opening PTZ panel.
	OpenPTZPanelButton UIComponentName = "openPTZPanelButton"
	// OpenTimerPanelButton is the button which is used for opening the timer type settings panel.
	OpenTimerPanelButton UIComponentName = "openTimerPanelButton"
	// PanLeftButton is the button for panning left preview.
	PanLeftButton UIComponentName = "panLeftButton"
	// PanRightButton is the button for panning right preview.
	PanRightButton UIComponentName = "panRightButton"
	// PTZResetAllButton is the button for reset PTZ to default value.
	PTZResetAllButton UIComponentName = "ptzResetAllButton"
	// PreviewExposureTime is the exposure data displayed when the expert option to show metadata is enabled.
	PreviewExposureTime UIComponentName = "previewExposureTime"
	// PreviewResolution is the resolution displayed when the expert option to show metadata is enabled.
	PreviewResolution UIComponentName = "previewResolution"
	// PreviewViewport is the container of the preview video.
	PreviewViewport UIComponentName = "previewViewport"
	// ReviewView is the review view after taking a photo under document mode.
	ReviewView UIComponentName = "reviewView"
	// ScanBarcodeOption is the option button to switch to QR code detection mode in scan mode.
	ScanBarcodeOption UIComponentName = "scanBarcodeOption"
	// ScanDocumentModeOption is the document mode option of scan mode.
	ScanDocumentModeOption UIComponentName = "scanDocumentModeOption"
	// SettingsButton is the button for opening primary setting menu.
	SettingsButton UIComponentName = "settingsButton"
	// SettingsHeader is the header on the settings view.
	SettingsHeader UIComponentName = "settingsHeader"
	// Shutter is a shutter button used to take a picture and start/stop a video.
	Shutter UIComponentName = "shutter"
	// Snackbar is a snackbar showing a label.
	Snackbar UIComponentName = "snackbar"
	// SwitchDeviceButton is the button for switching camera device.
	SwitchDeviceButton UIComponentName = "switchDeviceButton"
	// TiltDownButton is the button for tilting down preview.
	TiltDownButton UIComponentName = "tiltDownButton"
	// TimerOption10Seconds is an option to turn on 10-seconds timer.
	TimerOption10Seconds UIComponentName = "timerOption10Seconds"
	// TimerOption3Seconds is an option to turn on 3-seconds timer.
	TimerOption3Seconds UIComponentName = "timerOption3Seconds"
	// TimerOptionOff is an option to turn off the timer.
	TimerOptionOff UIComponentName = "timerOptionOff"
	// TiltUpButton is the button for tilting up preview.
	TiltUpButton UIComponentName = "tiltUpButton"
	// TimeLapseRecordingOption is the radio button to toggle time-lapse recording option.
	TimeLapseRecordingOption UIComponentName = "timeLapseRecordingOption"
	// ToggleMicButton is the button to toggle microphone option.
	ToggleMicButton UIComponentName = "toggleMicButton"
	// VideoPauseResumeButton is the button for pausing or resuming video recording.
	VideoPauseResumeButton UIComponentName = "videoPauseResumeButton"
	// VideoProfileSelect is select-options for selecting video profile.
	VideoProfileSelect UIComponentName = "videoProfileSelect"
	// VideoSnapshotButton is the button for taking video snapshot during recording.
	VideoSnapshotButton UIComponentName = "videoSnapshotButton"
	// ZoomInButton is the button for zoom in preview.
	ZoomInButton UIComponentName = "zoomInButton"
	// ZoomOutButton is the button for zoom out preview.
	ZoomOutButton UIComponentName = "zoomOutButton"
)

// List of setting menus in CCA.
const (
	// ExpertMenu is the expert settings menu.
	ExpertMenu SettingMenu = "expertMenu"
	// MainMenu is the main setting menu.
	MainMenu SettingMenu = "mainMenu"
	// PhotoAspectRatioMenu is the photo aspect ratio settings menu.
	PhotoAspectRatioMenu SettingMenu = "photoAspectRatioMenu"
	// PhotoResolutionMenu is the photo resolution settings menu.
	PhotoResolutionMenu SettingMenu = "photoResolutionMenu"
	// VideoResolutionMenu is the video resolution settings menu.
	VideoResolutionMenu SettingMenu = "videoResolutionMenu"
)

// List of options in CCA.
const (
	// CustomVideoParametersOption is the option to enable custom video parameters.
	CustomVideoParametersOption Option = "customVideoParametersOption"
	// ExpertModeOption is the option to enable expert mode.
	ExpertModeOption Option = "expertModeOption"
	// MultistreamRecordingOption is the option to enable multistream video recording.
	MultistreamRecordingOption Option = "multiStreamRecordingOption"
	// MultistreamRecordingChromeOption is the option to enable multistream video recording (scale by Chrome version).
	MultistreamRecordingChromeOption Option = "multiStreamRecordingChromeOption"
	// SaveMetadataOption is the option to save metadata of capture result.
	SaveMetadataOption Option = "saveMetadataOption"
	// ShowMetadataOption is the option to show preview metadata.
	ShowMetadataOption Option = "showMetadataOption"
)

// Visible returns whether a UIComponent{Name} is visible on the screen.
func (a *App) Visible(ctx context.Context, ui UIComponentName) (bool, error) {
	var visible bool
	if err := a.conn.Call(ctx, &visible, "CCATest.isVisible", ui); err != nil {
		return false, errors.Wrapf(err, "failed to check the visibility of %v", ui)
	}
	return visible, nil
}

// CheckVisible returns an error if visibility state of ui is not expected.
func (a *App) CheckVisible(ctx context.Context, ui UIComponentName, expected bool) error {
	if visible, err := a.Visible(ctx, ui); err != nil {
		return err
	} else if visible != expected {
		return errors.Errorf("unexpected %v visibility state: got %v, want %v", ui, visible, expected)
	}
	return nil
}

// WaitForVisibleState calls WaitForVisibleStateFor with 5 second timeout.
func (a *App) WaitForVisibleState(ctx context.Context, ui UIComponentName, expected bool) error {
	return a.WaitForVisibleStateFor(ctx, ui, expected, 5*time.Second)
}

// WaitForVisibleStateFor waits until the visibility of ui becomes expected for specified time.
func (a *App) WaitForVisibleStateFor(ctx context.Context, ui UIComponentName, expected bool, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		visible, err := a.Visible(ctx, ui)
		if err != nil {
			return testing.PollBreak(err)
		}
		if visible != expected {
			return errors.Errorf("failed to wait visibility state for %v: got %v, want %v", ui, visible, expected)
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

// Disabled returns disabled attribute of HTMLElement of |ui|.
func (a *App) Disabled(ctx context.Context, ui UIComponentName) (bool, error) {
	var disabled bool
	if err := a.conn.Call(ctx, &disabled, "CCATest.isDisabled", ui); err != nil {
		return false, errors.Wrapf(err, "failed to get disabled state of %v", ui)
	}
	return disabled, nil
}

// WaitForDisabled waits until the disabled state of ui becomes |expected|.
func (a *App) WaitForDisabled(ctx context.Context, ui UIComponentName, expected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		disabled, err := a.Disabled(ctx, ui)
		if err != nil {
			return testing.PollBreak(errors.Wrapf(err, "failed to wait disabled state of %v to be %v", ui, expected))
		}
		if disabled != expected {
			return errors.Errorf("failed to wait disabled state for %v: got %v, want %v", ui, disabled, expected)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
}

// CountUI returns number of ui elements.
func (a *App) CountUI(ctx context.Context, ui UIComponentName) (int, error) {
	var number int
	if err := a.conn.Call(ctx, &number, "CCATest.countUI", ui); err != nil {
		return 0, errors.Wrapf(err, "failed to count number of %v", ui)
	}
	return number, nil
}

// AttributeWithIndex returns the attr attribute of the index th ui.
func (a *App) AttributeWithIndex(ctx context.Context, ui UIComponentName, index int, attr string) (string, error) {
	var value string
	if err := a.conn.Call(ctx, &value, "CCATest.getAttribute", ui, attr, index); err != nil {
		return "", errors.Wrapf(err, "failed to get %v attribute of %v th %v", attr, index, ui)
	}
	return value, nil
}

// ScreenXYWithIndex returns the screen coordinates of the left-top corner of the |index|'th |ui|.
func (a *App) ScreenXYWithIndex(ctx context.Context, ui UIComponentName, index int) (*coords.Point, error) {
	var pt coords.Point
	if err := a.conn.Call(ctx, &pt, "CCATest.getScreenXY", ui, index); err != nil {
		return nil, errors.Wrapf(err, "failed to get sceen coordinates of %v'th %v", index, ui)
	}
	return &pt, nil
}

// Size returns size of the |ui|.
func (a *App) Size(ctx context.Context, ui UIComponentName) (*Resolution, error) {
	var size Resolution
	if err := a.conn.Call(ctx, &size, "CCATest.getSize", ui); err != nil {
		return nil, errors.Wrapf(err, "failed to get size of %v", ui)
	}
	return &size, nil
}

// Click clicks on UIComponent{Name}.
func (a *App) Click(ctx context.Context, ui UIComponentName) error {
	if err := a.conn.Call(ctx, nil, "CCATest.click", ui); err != nil {
		return errors.Wrapf(err, "failed to click on %v", ui)
	}
	return nil
}

// ClickWithIndex clicks nth ui.
func (a *App) ClickWithIndex(ctx context.Context, ui UIComponentName, index int) error {
	if err := a.conn.Call(ctx, nil, "CCATest.click", ui, index); err != nil {
		return errors.Wrapf(err, "failed to click on %v", ui)
	}
	return nil
}

// Hold holds on |ui| by sending pointerdown and pointerup for |d| duration.
func (a *App) Hold(ctx context.Context, ui UIComponentName, d time.Duration) error {
	if err := a.conn.Call(ctx, nil, "CCATest.hold", ui, d.Milliseconds()); err != nil {
		return errors.Wrapf(err, "failed to hold %v", ui)
	}
	return nil
}

// ClickPTZButton clicks on PTZ Button.
func (a *App) ClickPTZButton(ctx context.Context, ui UIComponentName) error {
	// Hold for 0ms to trigger PTZ minimal step movement.
	return a.Hold(ctx, ui, 0)
}

// IsCheckedWithIndex gets checked state of nth ui.
func (a *App) IsCheckedWithIndex(ctx context.Context, ui UIComponentName, index int) (bool, error) {
	var checked bool
	if err := a.conn.Call(ctx, &checked, "CCATest.isChecked", ui, index); err != nil {
		return false, errors.Wrapf(err, "failed to get checked state on %v(th) %v", index, ui)
	}
	return checked, nil
}

// SelectOption selects the target option in HTMLSelectElement.
func (a *App) SelectOption(ctx context.Context, ui UIComponentName, value string) error {
	if err := a.WaitForVisibleState(ctx, ui, true); err != nil {
		return err
	}
	if err := a.conn.Call(ctx, nil, "CCATest.selectOption", ui, value); err != nil {
		return errors.Wrapf(err, "failed to select option of %v", ui)
	}
	return nil
}

// InputRange returns the range of valid value for range type input element.
func (a *App) InputRange(ctx context.Context, ui UIComponentName) (*Range, error) {
	if err := a.WaitForVisibleState(ctx, ui, true); err != nil {
		return nil, err
	}
	var r Range
	if err := a.conn.Call(ctx, &r, "CCATest.getInputRange", ui); err != nil {
		return nil, errors.Wrapf(err, "failed to get input range of %v", ui)
	}
	return &r, nil
}

// SetRangeInput sets value of range input.
func (a *App) SetRangeInput(ctx context.Context, ui UIComponentName, value int) error {
	if err := a.WaitForVisibleState(ctx, ui, true); err != nil {
		return err
	}
	if err := a.conn.Call(ctx, nil, "CCATest.setRangeInputValue", ui, value); err != nil {
		return errors.Wrapf(err, "failed to set range input %v to %v", ui, value)
	}
	return nil
}

// WaitForSettingMenuState waits until setting menu state become as expected.
func (a *App) WaitForSettingMenuState(ctx context.Context, menu SettingMenu, expected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		var actual bool
		if err := a.conn.Call(ctx, &actual, "CCATest.isSettingMenuOpened", menu); err != nil {
			return testing.PollBreak(errors.Wrapf(err, "failed to get the state of setting menu %v", menu))
		}
		if actual != expected {
			return errors.Errorf("failed to wait setting menu state for %v: got %v, want %v", menu, actual, expected)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
}

// OpenSettingMenu opens the setting menu and waits for it to be opened.
func (a *App) OpenSettingMenu(ctx context.Context, menu SettingMenu) error {
	if err := a.conn.Call(ctx, nil, "CCATest.openSettingMenu", menu); err != nil {
		return errors.Wrapf(err, "failed to click to open the setting menu %v", menu)
	}
	return a.WaitForSettingMenuState(ctx, menu, true)
}

// CloseSettingMenu closes the setting menu.
func (a *App) CloseSettingMenu(ctx context.Context, menu SettingMenu) error {
	if err := a.conn.Call(ctx, nil, "CCATest.closeSettingMenu", menu); err != nil {
		return errors.Wrapf(err, "failed to close the setting menu %v", menu)
	}
	return a.WaitForSettingMenuState(ctx, menu, false)
}

// CloseSettingMenuIfVisible closes the setting menu if it is opened and visible.
func (a *App) CloseSettingMenuIfVisible(ctx context.Context, menu SettingMenu) error {
	if visible, err := a.Visible(ctx, SettingsHeader); err != nil {
		return errors.Wrap(err, "failed to detect if the setting menu is opened")
	} else if visible {
		return a.CloseSettingMenu(ctx, menu)
	}
	return nil
}

// OptionChecked returns the checked state of the state associated to |option|.
func (a *App) OptionChecked(ctx context.Context, option Option) (bool, error) {
	var result bool
	if err := a.conn.Call(ctx, &result, "CCATest.getOptionState", option); err != nil {
		return false, errors.Wrapf(err, "failed to get the state of %v", option)
	}
	return result, nil
}

// SetOptionChecked sets the checked state of |option| to |enabled|.
func (a *App) SetOptionChecked(ctx context.Context, option Option, enabled bool) error {
	prev, err := a.OptionChecked(ctx, option)
	if err != nil {
		return errors.Wrapf(err, "failed to get option state %v", option)
	}
	if prev == enabled {
		return nil
	}

	if err := a.conn.Call(ctx, nil, "CCATest.toggleOption", option); err != nil {
		return errors.Wrapf(err, "failed to toggle option %v", option)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if state, err := a.OptionChecked(ctx, option); err != nil {
			return testing.PollBreak(errors.Wrapf(err, "failed to get the state of option %v", option))
		} else if state != enabled {
			return errors.Errorf("failed to wait for state change of option %v", option)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}
	return nil
}
