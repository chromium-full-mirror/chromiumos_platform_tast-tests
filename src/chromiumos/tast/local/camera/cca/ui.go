// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cca provides utilities to interact with Chrome Camera App.
package cca

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// UIComponent represents a CCA UI component.
type UIComponent struct {
	Name      string
	Selectors []string
}

var (
	// ModeSelector is selection bar for different capture modes.
	ModeSelector = UIComponent{"mode selector", []string{"#modes-group"}}
	// SettingsButton is button for opening primary setting menu.
	SettingsButton = UIComponent{"settings", []string{"#open-settings"}}
	// SwitchDeviceButton is button for switching camera device.
	SwitchDeviceButton = UIComponent{"switch device button", []string{"#switch-device"}}
	// VideoSnapshotButton is button for taking video snapshot during recording.
	VideoSnapshotButton = UIComponent{"video snapshot button", []string{"#video-snapshot"}}
	// VideoPauseResumeButton is button for pausing or resuming video recording.
	VideoPauseResumeButton = UIComponent{"video pause/resume button", []string{"#pause-recordvideo"}}

	// PhotoResolutionSettingButton is button for opening photo resolution setting menu.
	PhotoResolutionSettingButton = UIComponent{"photo resolution setting button", []string{"#settings-photo-resolution"}}
	// PhotoAspectRatioSettingButton is button for opening photo aspect ratio setting menu.
	PhotoAspectRatioSettingButton = UIComponent{"photo aspect ratio setting button", []string{"#settings-photo-aspect-ratio"}}
	// VideoResolutionSettingButton is button for opening video resolution setting menu.
	VideoResolutionSettingButton = UIComponent{"video resolution setting button", []string{"#settings-video-resolution"}}

	// ExpertModeButton is button used for opening expert mode setting menu.
	ExpertModeButton = UIComponent{"expert mode button", []string{"#settings-expert"}}
	// FeedbackButton is the feedback button showing in the settings menu.
	FeedbackButton = UIComponent{"feedback button", []string{"#settings-feedback"}}
	// HelpButton is the help button showing in the settings menu.
	HelpButton = UIComponent{"help button", []string{"#settings-help"}}

	// BarcodeChipURL is chip for url detected from barcode.
	BarcodeChipURL = UIComponent{"barcode chip url", []string{".barcode-chip-url a"}}
	// BarcodeChipText is chip for text detected from barcode.
	BarcodeChipText = UIComponent{"barcode chip text", []string{".barcode-chip-text"}}
	// BarcodeCopyURLButton is button to copy url detected from barcode.
	BarcodeCopyURLButton = UIComponent{"barcode copy url button",
		[]string{"#barcode-chip-url-container .barcode-copy-button"}}
	// BarcodeCopyTextButton is button to copy text detected from barcode.
	BarcodeCopyTextButton = UIComponent{"barcode copy text button",
		[]string{"#barcode-chip-text-container .barcode-copy-button"}}

	// OpenMirrorPanelButton is the button which is used for opening the mirror state settings panel.
	OpenMirrorPanelButton = UIComponent{"mirror state option button", []string{"#open-mirror-panel"}}
	// OpenGridPanelButton is the button which is used for opening the grid type settings panel.
	OpenGridPanelButton = UIComponent{"grid type option button", []string{"#open-grid-panel"}}
	// OpenTimerPanelButton is the button which is used for opening the timer type settings panel.
	OpenTimerPanelButton = UIComponent{"timer type option button", []string{"#open-timer-panel"}}

	// ScanBarcodeOption is the option button to switch to QR code detection mode in scan mode.
	ScanBarcodeOption = UIComponent{"scan barcode option", []string{"#scan-barcode"}}
	// ReviewView is the review view after taking a photo under document mode.
	ReviewView = UIComponent{"document review view", []string{"#view-review"}}

	// GifRecordingOption is the radio button to toggle gif recording option.
	GifRecordingOption = UIComponent{"gif recording button", []string{
		"input[type=radio][data-state=record-type-gif]"}}
	// GifReviewSaveButton is the save button in gif review page.
	GifReviewSaveButton = UIComponent{"save gif button", []string{
		"#view-review button[i18n-text=label_save]"}}
	// GifReviewRetakeButton is the retake button in gif review page.
	GifReviewRetakeButton = UIComponent{"retake gif button", []string{"#review-retake"}}

	// LowStorageDialog is the dialog displayed when there's an unexpected behavior during recording due to low storage.
	LowStorageDialog = UIComponent{"low storage dialog", []string{"#view-low-storage-dialog"}}
	// LowStorageDialogOKButton is the button labeled "OK" in LowStorageDialog, used to acknowledge and close the dialog.
	LowStorageDialogOKButton = UIComponent{"low storage dialog OK button", []string{"#view-low-storage-dialog button.dialog-positive-button"}}
	// LowStorageDialogManageButton is the button in LowStorageDialog that navigates users to "Manage storage" page in system settings.
	LowStorageDialogManageButton = UIComponent{"low storage manage storage button", []string{"#view-low-storage-dialog button.dialog-negative-button"}}
	// LowStorageWarning is the warning nudge displayed while recording on device with low storage.
	LowStorageWarning = UIComponent{"low storage warning nudge", []string{"#nudge"}}
)

var (
	// A11yRootNode represents the root node of Camera app in A11y tree.
	A11yRootNode = nodewith.Name("Camera").Role(role.RootWebArea)
	// A11yCanvasNode represents the canvas node in A11y tree.
	A11yCanvasNode = nodewith.Role(role.Canvas).Ancestor(A11yRootNode)
)

// Option is the option for toggling state.
type Option struct {
	// ui is the |UIComponent| to toggle the option.
	ui UIComponent
	// state is state toggle by this option.
	state string
}

func newOption(state, selector string) Option {
	name := fmt.Sprintf("option to toggle %v state", state)
	selectors := []string{selector}
	return Option{ui: UIComponent{Name: name, Selectors: selectors}, state: state}
}

var (
	// CustomVideoParametersOption is the option to enable custom video parameters.
	CustomVideoParametersOption = newOption("custom-video-parameters", "#custom-video-parameters")
	// ExpertModeOption is the option to enable expert mode.
	ExpertModeOption = newOption("expert", "#expert-enable-expert-mode")
	// SaveMetadataOption is the option to save metadata of capture result.
	SaveMetadataOption = newOption("save-metadata", "#expert-save-metadata")
	// ShowMetadataOption is the option to show preview metadata.
	ShowMetadataOption = newOption("show-metadata", "#expert-show-metadata")
	// EnableMultistreamRecordingOption is the option to enable document scanning on all cameras.
	EnableMultistreamRecordingOption = newOption("enable-multistream-recording", "#expert-enable-multistream-recording")
)

// The following types are defined corresponding to definitions in
// /js/test/cca_type.ts in CCA side.

// UIComponentName represents a name of UI component.
type UIComponentName string

// List of UI components used in CCA for testing.
const (
	// BackAspectRatioOptions are the buttons of aspect ratio options for the back camera.
	BackAspectRatioOptions UIComponentName = "backAspectRatioOptions"
	// BackPhotoResolutionOptions are the buttons of photo resolution options for the back camera.
	BackPhotoResolutionOptions UIComponentName = "backPhotoResolutionOptions"
	// BackVideoResolutionOptions are the buttons of video resolution options for the back camera.
	BackVideoResolutionOptions UIComponentName = "backVideoResolutionOptions"
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
	// GridOptionGoldenRatio is an option to enable grid of type golden ratio.
	GridOptionGoldenRatio UIComponentName = "gridOptionGoldenRatio"
	// MirrorOptionOff is an option to disable mirror preview.
	MirrorOptionOff UIComponentName = "mirrorOptionOff"
	// MirrorOptionOff is an option to enable mirror preview.
	MirrorOptionOn UIComponentName = "mirrorOptionOn"
	// OpenPTZPanelButton is the button for opening PTZ panel.
	OpenPTZPanelButton UIComponentName = "openPTZPanelButton"
	// PanLeftButton is the button for panning left preview.
	PanLeftButton UIComponentName = "panLeftButton"
	// PanRightButton is the button for panning right preview.
	PanRightButton UIComponentName = "panRightButton"
	// PTZResetAllButton is the button for reset PTZ to default value.
	PTZResetAllButton UIComponentName = "ptzResetAllButton"
	// PreviewViewport is the container of the preview video.
	PreviewViewport UIComponentName = "previewViewport"
	// ScanDocumentModeOption is the document mode option of scan mode.
	ScanDocumentModeOption UIComponentName = "scanDocumentModeOption"
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
	// VideoProfileSelect is select-options for selecting video profile.
	VideoProfileSelect UIComponentName = "videoProfileSelect"
	// ZoomInButton is the button for zoom in preview.
	ZoomInButton UIComponentName = "zoomInButton"
	// ZoomOutButton is the button for zoom out preview.
	ZoomOutButton UIComponentName = "zoomOutButton"
)

type errorUINotExist struct {
	ui *UIComponent
}

func (err errorUINotExist) Error() string {
	return fmt.Sprintf("failed to resolved ui %v to its correct selector", err.ui.Name)
}

// resolveUISelector resolves ui to its correct selector.
func (a *App) resolveUISelector(ctx context.Context, ui UIComponent) (string, error) {
	for _, s := range ui.Selectors {
		if exist, err := a.selectorExist(ctx, s); err != nil {
			return "", err
		} else if exist {
			return s, nil
		}
	}
	return "", errorUINotExist{ui: &ui}
}

// Style returns the value of an CSS attribute of an UI component.
func (a *App) Style(ctx context.Context, ui UIComponent, attribute string) (string, error) {
	selector, err := a.resolveUISelector(ctx, ui)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get the selector of UI: %v", ui.Name)
	}
	var style string
	if err := a.conn.Call(ctx, &style, "Tast.getStyle", selector, attribute); err != nil {
		return "", errors.Wrapf(err, "failed to get the style of attribute: %v of UI: %v", attribute, ui.Name)
	}
	return style, nil
}

// Visible returns whether a UIComponent{Name} is visible on the screen.
// TODO(b/242800694): Replace this function with |VisibleUIComponentName| once the refactor is completed.
func (a *App) Visible(ctx context.Context, ui interface{}) (bool, error) {
	switch t := ui.(type) {
	case UIComponentName:
		return a.VisibleUIComponentName(ctx, ui.(UIComponentName))
	case UIComponent:
		return a.VisibleLegacy(ctx, ui.(UIComponent))
	default:
		return false, errors.Errorf("failed to click: invalid type %v", t)
	}
}

// VisibleLegacy returns whether a UI component is visible on the screen.
func (a *App) VisibleLegacy(ctx context.Context, ui UIComponent) (bool, error) {
	wrapError := func(err error) error {
		return errors.Wrapf(err, "failed to check visibility state of %v", ui.Name)
	}
	selector, err := a.resolveUISelector(ctx, ui)
	if err != nil {
		return false, wrapError(err)
	}
	var visible bool
	if err := a.conn.Call(ctx, &visible, "Tast.isVisible", selector); err != nil {
		return false, wrapError(err)
	}
	return visible, nil
}

// VisibleUIComponentName returns whether a UI component is visible on the screen.
func (a *App) VisibleUIComponentName(ctx context.Context, ui UIComponentName) (bool, error) {
	var visible bool
	if err := a.conn.Call(ctx, &visible, "CCATest.isVisible", ui); err != nil {
		return false, errors.Wrapf(err, "failed to check the visibility of %v", ui)
	}
	return visible, nil
}

// CheckVisible returns an error if visibility state of ui is not expected.
func (a *App) CheckVisible(ctx context.Context, ui interface{}, expected bool) error {
	if visible, err := a.Visible(ctx, ui); err != nil {
		return err
	} else if visible != expected {
		// TODO(b/242800694): Fix the comment back once changing interface{} to |UIComponentName|
		return errors.Errorf("unexpected visibility state: got %v, want %v", visible, expected)
	}
	return nil
}

// WaitForVisibleState calls WaitForVisibleStateFor with 5 second timeout.
func (a *App) WaitForVisibleState(ctx context.Context, ui interface{}, expected bool) error {
	return a.WaitForVisibleStateFor(ctx, ui, expected, 5*time.Second)
}

// WaitForVisibleStateFor waits until the visibility of ui becomes expected for specified time.
func (a *App) WaitForVisibleStateFor(ctx context.Context, ui interface{}, expected bool, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		visible, err := a.Visible(ctx, ui)
		if err != nil {
			return testing.PollBreak(err)
		}
		if visible != expected {
			// TODO(b/242800694): Fix the comment back once changing interface{} to |UIComponentName|
			return errors.Errorf("failed to wait visibility state: got %v, want %v", visible, expected)
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
// TODO(b/242800694): Replace this function with |ClickUIComponentName| once the refactor is completed.
func (a *App) Click(ctx context.Context, ui interface{}) error {
	switch t := ui.(type) {
	case UIComponentName:
		return a.ClickUIComponentName(ctx, ui.(UIComponentName))
	case UIComponent:
		return a.ClickLegacy(ctx, ui.(UIComponent))
	default:
		return errors.Errorf("failed to click: invalid type %v", t)
	}
}

// ClickLegacy clicks on ui.
func (a *App) ClickLegacy(ctx context.Context, ui UIComponent) error {
	wrapError := func(err error) error {
		return errors.Wrapf(err, "failed to click on %v", ui.Name)
	}
	selector, err := a.resolveUISelector(ctx, ui)
	if err != nil {
		return wrapError(err)
	}
	if err := a.ClickWithSelector(ctx, selector); err != nil {
		return wrapError(err)
	}
	return nil
}

// ClickUIComponentName clicks on ui.
func (a *App) ClickUIComponentName(ctx context.Context, ui UIComponentName) error {
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
