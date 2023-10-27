// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package phonehub

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/crossdevicesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	setupDialogURL   = "chrome://os-settings/multidevice/features?showPhonePermissionSetupDialog"
	phoneHubToggleJS = crossdevicesettings.MultideviceSubpageJS +
		`.shadowRoot.getElementById("phoneHubItem")` +
		`.shadowRoot.querySelector("settings-multidevice-feature-toggle")` +
		`.shadowRoot.getElementById("toggle")`
	recentPhotosToggleJS = crossdevicesettings.MultideviceSubpageJS +
		`.shadowRoot.getElementById("phoneHubCameraRollItem")` +
		`.shadowRoot.querySelector("settings-multidevice-feature-toggle")` +
		`.shadowRoot.getElementById("toggle")`
	notificationToggleJS = crossdevicesettings.MultideviceSubpageJS +
		`.shadowRoot.getElementById("phoneHubNotificationsItem")` +
		`.shadowRoot.querySelector("settings-multidevice-feature-toggle")` +
		`.shadowRoot.getElementById("toggle")`
	setupDialogNextButtonJS = crossdevicesettings.MultidevicePageJS +
		`.shadowRoot.querySelector("settings-multidevice-permissions-setup-dialog")` +
		`.shadowRoot.getElementById("getStartedButton")`
	featureCheckedJS = `.checked`
)

// Enable enables Phone Hub from OS Settings using JS. Assumes a connected device has already been paired.
// Hide should be called afterwards to close the Phone Hub tray. It is left open here so callers can capture the UI state upon error if needed.
func Enable(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	settingsConn, err := crossdevicesettings.OSSettingsWithShadowPiercer(ctx, tconn, cr, crossdevicesettings.ConnectedDevicesSettingsURL /*existingConn=*/, false)
	if err != nil {
		return err
	}
	defer settingsConn.Close()

	// Toggle Phone Hub on with JS.
	if err := settingsConn.WaitForExpr(ctx, phoneHubToggleJS); err != nil {
		return errors.Wrap(err, "failed to find the Phone Hub toggle")
	}
	var enabled bool
	if err := settingsConn.Eval(ctx, phoneHubToggleJS+featureCheckedJS, &enabled); err != nil {
		return errors.Wrap(err, "failed to get Phone Hub toggle status")
	}
	if !enabled {
		if err := settingsConn.Eval(ctx, phoneHubToggleJS+`.click()`, nil); err != nil {
			return errors.Wrap(err, "failed to enable Phone Hub")
		}
	}
	// Check that the toggle was correctly enabled.
	if err := settingsConn.WaitForExpr(ctx, phoneHubToggleJS+featureCheckedJS+`===true`); err != nil {
		return errors.Wrap(err, "failed to toggle Phone Hub on using JS")
	}
	// Phone Hub is still not immediately ready to use after toggling it on from OS Settings,
	// since it takes a short amount of time for it to connect to the phone and display anything.
	// Wait for it to become usable by checking for the existence of a settings pod. Retry a few times before we actually fail this step.
	if err := uiauto.Retry(3, func(ctx context.Context) error {
		ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
		if err := Show(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to show Phone Hub")
		}
		if err := ui.WaitUntilExists(phoneHubSettingPod.First())(ctx); err != nil {
			// Hide the Phone Hub bubble before retry if not successfully connected this time.
			if err := Hide(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to hide Phone Hub buble before retry")
			}
			return errors.Wrap(err, "failed to find a Phone Hub setting pod")
		}
		return nil
	})(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to phone after retries")
	}

	return nil
}

// PhoneHubTray is the finder for the Phone Hub tray UI.
var PhoneHubTray = nodewith.Name("Phone Hub").ClassName("Widget")

// PhoneHubShelfIcon is the finder for the Phone Hub shelf icon.
var PhoneHubShelfIcon = nodewith.Name("Phone Hub").Role(role.Button).ClassName("PhoneHubTray")

// phoneHubSettingPod is the base UI finder for the individual setting pods.
var phoneHubSettingPod = nodewith.Ancestor(PhoneHubTray).Role(role.ToggleButton)

// SilencePhonePod is the finder for Phone Hub's Silence Phone pod.
var SilencePhonePod = phoneHubSettingPod.NameContaining("Toggle Silence phone")

// LocatePhonePod is the finder for Phone Hub's "Locate phone" pod.
var LocatePhonePod = phoneHubSettingPod.NameContaining("Toggle Locate phone")

// Show opens Phone Hub if it's not already open.
func Show(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	if err := ui.Exists(PhoneHubTray)(ctx); err == nil { // Phone Hub already open
		return nil
	}
	if err := uiauto.Combine("click Phone Hub shelf icon and wait for it to open",
		ui.LeftClick(PhoneHubShelfIcon),
		ui.WaitUntilExists(PhoneHubTray),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open Phone Hub")
	}
	return nil
}

// Hide hides Phone Hub if it's not already hidden.
func Hide(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	if err := ui.Exists(PhoneHubTray)(ctx); err != nil { // Phone Hub already hidden
		return nil
	}
	if err := uiauto.Combine("click Phone Hub shelf icon and wait for it to close",
		ui.LeftClick(PhoneHubShelfIcon),
		ui.WaitUntilGone(PhoneHubTray),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close Phone Hub")
	}
	return nil
}

// podToggled returns true if the given setting pod is active, and false otherwise.
func podToggled(ctx context.Context, tconn *chrome.TestConn, pod *nodewith.Finder) (bool, error) {
	ui := uiauto.New(tconn)
	info, err := ui.Info(ctx, pod)
	if err != nil {
		return false, errors.Wrap(err, "failed to get node info for the setting pod")
	}
	if info.Checked == checked.True {
		return true, nil
	}
	return false, nil
}

// waitForPodToggled waits for the given pod to be toggled on/off.
func waitForPodToggled(ctx context.Context, tconn *chrome.TestConn, pod *nodewith.Finder, enabled bool, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if curr, err := podToggled(ctx, tconn, pod); err != nil {
			return err
		} else if curr != enabled {
			return errors.New("current status does not match the desired status")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		wanted := "off"
		if enabled {
			wanted = "on"
		}
		return errors.Wrapf(err, "failed waiting for Silence Phone to be turned %v", wanted)
	}
	return nil
}

// togglePod toggles the given Phone Hub pod on/off.
func togglePod(ctx context.Context, tconn *chrome.TestConn, pod *nodewith.Finder, enable bool) error {
	ui := uiauto.New(tconn)
	curr, err := podToggled(ctx, tconn, pod)
	if err != nil {
		return errors.Wrap(err, "failed to check current pod setting")
	}
	if curr == enable {
		return nil
	}
	if err := ui.LeftClick(pod)(ctx); err != nil {
		return errors.Wrap(err, "failed to click pod")
	}
	if err := waitForPodToggled(ctx, tconn, pod, enable, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed waiting for pod to be toggled after clicking")
	}
	return nil
}

// PhoneSilenced returns true if the "Silence phone" pod is active, and false otherwise.
func PhoneSilenced(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	return podToggled(ctx, tconn, SilencePhonePod)
}

// WaitForPhoneSilenced waits for the "Phone Silenced" pod to be toggled on/off, since its state can be changed from the Android side.
func WaitForPhoneSilenced(ctx context.Context, tconn *chrome.TestConn, silenced bool, timeout time.Duration) error {
	return waitForPodToggled(ctx, tconn, SilencePhonePod, silenced, timeout)
}

// LocatePhoneEnabled returns true if the "Locate phone" pod is active, and false otherwise.
func LocatePhoneEnabled(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	return podToggled(ctx, tconn, LocatePhonePod)
}

// WaitForLocatePhoneEnabled waits for the "Locate phone" pod to be toggled on/off.
func WaitForLocatePhoneEnabled(ctx context.Context, tconn *chrome.TestConn, silenced bool, timeout time.Duration) error {
	return waitForPodToggled(ctx, tconn, LocatePhonePod, silenced, timeout)
}

// ToggleSilencePhonePod toggles Phone Hub's "Silence Phone" pod on/off.
func ToggleSilencePhonePod(ctx context.Context, tconn *chrome.TestConn, silence bool) error {
	return togglePod(ctx, tconn, SilencePhonePod, silence)
}

// ToggleLocatePhonePod toggles Phone Hub's "Locate phone" pod on/off.
func ToggleLocatePhonePod(ctx context.Context, tconn *chrome.TestConn, enable bool) error {
	return togglePod(ctx, tconn, LocatePhonePod, enable)
}

// FindSubFeaturesSetupButton returns a finder which locates the Set up button for the Recent Photos feature.
func FindSubFeaturesSetupButton() *nodewith.Finder {
	return nodewith.Ancestor(nodewith.ClassName("MultideviceFeatureOptInView")).NameContaining("Set up").Role(role.Button)
}

// OptInSubFeatures enables the Phone Hub sub-features by clicking on the Set up button displayed in the Phone Hub bubble and run the set up flow right before user consent on the phone.
func OptInSubFeatures(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	ui := uiauto.New(tconn)
	subFeaturesButton := FindSubFeaturesSetupButton()
	// If the setup button is not visible in Phone Hub UI, try to enable them in settings page.
	if subFeaturesButton == nil {
		if err := ToggleRecentPhotosSetting(ctx, tconn, cr, true); err != nil {
			return errors.Wrap(err, "no setup button found in Phone Hub bubble and failed to toggle the Camera Roll feature in settings page")
		}
		if err := ToggleNotificationSetting(ctx, tconn, cr, true); err != nil {
			return errors.Wrap(err, "no setup button found in Phone Hub bubble and failed to toggle the Notification feature in settings page")
		}
	} else {
		if err := ui.LeftClick(subFeaturesButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to click on the sub-features opt-in button")
		}

		setupDialogConn, err := crossdevicesettings.OSSettingsWithShadowPiercer(ctx, tconn, cr, setupDialogURL, true)
		if err != nil {
			return errors.Wrap(err, "permissions set up dialog did not launch")
		}
		defer setupDialogConn.Close()
		if err := setupDialogConn.WaitForExpr(ctx, setupDialogNextButtonJS); err != nil {
			return errors.Wrap(err, "failed to wait for Permissions Setup Dialog to be visible")
		}
		if err := setupDialogConn.Eval(ctx, setupDialogNextButtonJS+`.click()`, nil); err != nil {
			return errors.Wrap(err, "failed to click Next on Permissions Setup Dialog intro screen")
		}
	}

	return nil
}

// DownloadMostRecentPhoto downloads the first photo shown in Phone Hub's Recent Photos section to Tote.
func DownloadMostRecentPhoto(ctx context.Context, tconn *chrome.TestConn) error {
	mostRecentPhotoThumbnail := nodewith.Ancestor(PhoneHubTray).ClassName("CameraRollThumbnail").First()
	// It might take some time to receive and display the recent photo thumbnails.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := ui.LeftClick(mostRecentPhotoThumbnail)(ctx); err != nil {
		return errors.Wrap(err, "failed to download recent photo")
	}
	return nil
}

// ToggleRecentPhotosSetting toggles the Camera Roll feature in the settings page.
func ToggleRecentPhotosSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, enable bool) error {
	return ToggleSubFeatureSetting(ctx, tconn, cr, true, recentPhotosToggleJS)
}

// ToggleNotificationSetting toggles the Notification feature in the settings page.
func ToggleNotificationSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, enable bool) error {
	return ToggleSubFeatureSetting(ctx, tconn, cr, true, notificationToggleJS)
}

// ToggleSubFeatureSetting toggles the sub features (Camera roll and notification) setting using JS. This assumes that a connected device has already been paired.
func ToggleSubFeatureSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, enable bool, featureToggleJS string) error {
	settingsConn, err := crossdevicesettings.OSSettingsWithShadowPiercer(ctx, tconn, cr /*url=*/, "" /*existingConn=*/, false)
	if err != nil {
		return err
	}
	defer settingsConn.Close()

	settingsConn, err = crossdevicesettings.OSSettingsWithShadowPiercer(ctx, tconn, cr, crossdevicesettings.ConnectedDevicesSettingsURL /*existingConn=*/, false)
	if err != nil {
		return errors.Wrap(err, "failed to re-launch OS Settings to the multidevice feature page")
	}

	if err := settingsConn.WaitForExpr(ctx, featureToggleJS); err != nil {
		return errors.Wrap(err, "failed to find the feature's toggle")
	}
	var isEnabled bool
	if err := settingsConn.Eval(ctx, featureToggleJS+featureCheckedJS, &isEnabled); err != nil {
		return errors.Wrap(err, "failed to get feature's toggle status")
	}
	if isEnabled != enable {
		if err := settingsConn.Eval(ctx, featureToggleJS+`.click()`, nil); err != nil {
			return errors.Wrap(err, "failed to click on feature;s toggle")
		}
	}
	if err := settingsConn.WaitForExpr(ctx, featureToggleJS+featureCheckedJS+`===`+strconv.FormatBool(enable)); err != nil {
		return errors.Wrapf(err, "failed to toggle feature to %v using JS", strconv.FormatBool(enable))
	}

	return nil
}

// BatteryLevel returns the battery level displayed in Phone Hub.
func BatteryLevel(ctx context.Context, tconn *chrome.TestConn) (int, error) {
	ui := uiauto.New(tconn)
	battery, err := ui.Info(ctx, nodewith.Role(role.StaticText).NameContaining("Phone battery"))
	if err != nil {
		return -1, errors.Wrap(err, "failed to find phone battery display")
	}

	r := regexp.MustCompile(`Phone battery (\d+)%`)
	m := r.FindStringSubmatch(string(battery.Name))
	if len(m) == 0 {
		return -1, errors.Wrap(err, "failed to extract battery percentage from the UI")
	}
	level, err := strconv.Atoi(m[1])
	if err != nil {
		return -1, errors.Wrapf(err, "failed to convert battery level %v to int", m[0])
	}
	return level, nil
}

// RecentTabChipFinder is the finder for Phone Hub "Recent Chrome tab" chips.
var RecentTabChipFinder = nodewith.Role(role.Button).HasClass("ContinueBrowsingChip")

// RecentTabChip represents one of Phone Hub's "Recent Chrome tab" chips.
type RecentTabChip struct {
	URL    string
	Finder *nodewith.Finder
}

// RecentTabChips returns all of the "Recent Chrome tab" chips currently displayed in Phone Hub.
func RecentTabChips(ctx context.Context, tconn *chrome.TestConn) ([]RecentTabChip, error) {
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(RecentTabChipFinder.First())(ctx); err != nil {
		return nil, errors.Wrap(err, "no recent tab chips found")
	}
	nodes, err := ui.NodesInfo(ctx, RecentTabChipFinder)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get UI node info about recent tab chips")
	}

	var chips []RecentTabChip
	for _, node := range nodes {
		// Extract the URL from the Name attribute, which looks like:
		//   name=Browser tab 1 of 1. Blah, https://www.blah.org/
		name := node.Name
		parts := strings.Split(name, " ")
		url := parts[len(parts)-1]
		chips = append(chips, RecentTabChip{URL: url, Finder: RecentTabChipFinder.Name(name)})
	}
	return chips, nil
}
