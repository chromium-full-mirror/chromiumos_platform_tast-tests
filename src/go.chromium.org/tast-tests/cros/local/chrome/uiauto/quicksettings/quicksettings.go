// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package quicksettings is for controlling the Quick Settings directly from the UI.
package quicksettings

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const uiTimeout = 10 * time.Second

// Whether these helper functions assume the new "QsRevamp" UI is enabled.
var qsRevampEnabled = false

// findStatusArea finds the status area UI node.
func findStatusArea(ctx context.Context, tconn *chrome.TestConn) (*nodewith.Finder, error) {
	ui := uiauto.New(tconn)
	statusArea := nodewith.HasClass("UnifiedSystemTray").First()
	return statusArea, ui.WithTimeout(uiTimeout).WaitUntilExists(statusArea)(ctx)
}

// clickAndWaitForAnimation clicks the node found with the provided finder and
// waits until the Quick Settings is no longer animating. The node provided is
// expected to be, but not enforced to be, either the expand or collapse
// button.
func clickAndWaitForAnimation(ctx context.Context, tconn *chrome.TestConn, node *nodewith.Finder) error {
	initialBounds, err := Rect(ctx, tconn)

	if err != nil {
		return err
	}

	previousBounds := initialBounds
	checkIfAnimating := func(ctx context.Context) error {
		if currentBounds, err := Rect(ctx, tconn); err != nil {
			return testing.PollBreak(err)
		} else if currentBounds != previousBounds {
			previousBounds = currentBounds
			return errors.New("the Quick Settings is still animating")
		}
		return nil
	}

	if err := uiauto.New(tconn).LeftClick(node)(ctx); err != nil {
		errors.Wrap(err, "failed to click the node")
	}

	if err := testing.Poll(ctx, checkIfAnimating, &testing.PollOptions{Interval: 500 * time.Millisecond, Timeout: uiTimeout}); err != nil {
		return errors.Wrap(err, "the Quick Settings did not stop animating")
	}
	return nil
}

// Init configures the package with the correct value of feature QsRevamp,
// depending on its state in chrome. The state might vary because the feature
// flag default was flipped in a chrome uprev, or because a test explicitly
// opted-in or opted-out of the feature. Returns a cleanup function which
// callers should defer.
func Init(ctx context.Context, tconn *chrome.TestConn) (func(), error) {
	var qs bool
	if err := tconn.Call(ctx, &qs,
		"tast.promisify(chrome.autotestPrivate.isFeatureEnabled)", "QsRevamp"); err != nil {
		return nil, errors.Wrap(err, "failed to get QsRevamp feature state")
	}
	qsRevampEnabled = qs
	return func() { qsRevampEnabled = false }, nil
}

// SetQsRevampEnabled configures the package to assume the feature QsRevamp is
// either enabled or disabled. Returns a cleanup function which callers should
// defer.
func SetQsRevampEnabled(enabled bool) func() {
	qsRevampEnabled = enabled
	return func() { qsRevampEnabled = false }
}

// GetRootFinder returns the finder for the root quick settings view.
// The finder is different based on whether QsRevamp is enabled or not.
func GetRootFinder() *nodewith.Finder {
	if qsRevampEnabled {
		return QsRootFinder
	}
	return LegacyRootFinder
}

// Rect returns a coords.Rect struct for the Quick Settings area, which contains
// coordinate information about the rectangular region it occupies on the screen.
// As clients of this function generally expect the bounds of the window, not the
// "UnifiedSystemTrayView" view itself, this finds a not that has
// UnifiedSystemTrayView as a child.
func Rect(ctx context.Context, tconn *chrome.TestConn) (coords.Rect, error) {
	ui := uiauto.New(tconn)
	bubbleFrameView := nodewith.HasClass("BubbleFrameView")
	results, err := ui.NodesInfo(ctx, bubbleFrameView)
	if err != nil {
		return coords.Rect{}, errors.Wrap(err, "failed to find quick settings")
	}

	for i := range results {
		if err := ui.Exists(GetRootFinder().Ancestor(bubbleFrameView.Nth(i)))(ctx); err == nil {
			return results[i].Location, nil
		}
	}
	return coords.Rect{}, errors.Wrap(err, "failed to find quick settings")
}

// ClickStatusArea clicks the status area, which is the area on the shelf where info
// such as time and battery level are shown.
func ClickStatusArea(ctx context.Context, tconn *chrome.TestConn) error {
	statusArea, err := findStatusArea(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find status area widget")
	}
	ui := uiauto.New(tconn)
	if err := ui.LeftClick(statusArea)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the status area")
	}
	return nil
}

// Shown checks if Quick Settings exists in the UI.
func Shown(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	return uiauto.New(tconn).IsNodeFound(ctx, GetRootFinder())
}

// Show will click the status area to show Quick Settings and wait for it to appear.
// If Quick Settings is already open, it does nothing. Quick Settings will remain
// open between tests if it's not closed explicitly, so this should be accompanied
// by a deferred call to Hide to clean up the UI before starting other tests.
func Show(ctx context.Context, tconn *chrome.TestConn) error {
	if shown, err := Shown(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed initial quick settings visibility check")
	} else if shown {
		return nil
	}

	if err := ClickStatusArea(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to click the status area")
	}

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(GetRootFinder())(ctx); err != nil {
		return errors.Wrap(err, "failed waiting for quick settings to appear")
	}
	return nil
}

// Hide will click the status area to hide Quick Settings if it's currently shown.
// It will then wait for it to be hidden for the duration specified by timeout.
func Hide(ctx context.Context, tconn *chrome.TestConn) error {
	if shown, err := Shown(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed initial quick settings visibility check")
	} else if !shown {
		return nil
	}

	if err := ClickStatusArea(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to click the status area")
	}

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(uiTimeout).WaitUntilGone(GetRootFinder())(ctx); err != nil {
		return errors.Wrap(err, "failed waiting for quick settings to be hidden")
	}
	return nil
}

// Collapse will result in the Quick Settings being opened and in a collapsed
// state. This is safe to call even when Quick Settings is already open.
// TODO(b/252870625): Remove this function when all tests work with QsRevamp
// and replace call sites with Show().
func Collapse(ctx context.Context, tconn *chrome.TestConn) error {
	if err := Hide(ctx, tconn); err != nil {
		return err
	}

	if err := ShowWithRetry(ctx, tconn, 5*time.Second); err != nil {
		return err
	}

	if qsRevampEnabled {
		// Quick settings with QsRevamp cannot be collapsed, so there is nothing to do.
		return nil
	}

	exist, err := uiauto.New(tconn).IsNodeFound(ctx, ExpandButton)
	if err != nil {
		return errors.Wrap(err, "failed to check if the expand button already exists")
	}
	if exist {
		return nil
	}

	if err := clickAndWaitForAnimation(ctx, tconn, CollapseButton); err != nil {
		return errors.Wrap(err, "the Quick Settings did not collapse")
	}
	return nil
}

// Expand will result in the Quick Settings being opened and in an expanded
// state. This is safe to call even when Quick Settings is already open.
// TODO(b/252870625): Remove this function when all tests work with QsRevamp
// and replace call sites with Show().
func Expand(ctx context.Context, tconn *chrome.TestConn) error {
	if err := Hide(ctx, tconn); err != nil {
		return err
	}

	if err := ShowWithRetry(ctx, tconn, 5*time.Second); err != nil {
		return err
	}

	if qsRevampEnabled {
		// Quick settings with QsRevamp cannot be collapsed, so there is nothing to expand.
		return nil
	}

	exist, err := uiauto.New(tconn).IsNodeFound(ctx, CollapseButton)
	if err != nil {
		return errors.Wrap(err, "failed to check if the collapse button already exists")
	}
	if exist {
		return nil
	}

	if err := clickAndWaitForAnimation(ctx, tconn, ExpandButton); err != nil {
		return errors.Wrap(err, "the Quick Settings did not expand")
	}
	return nil
}

// ShowWithRetry will continuously click the status area until Quick Settings is shown,
// for the duration specified by timeout. Quick Settings sometimes does not open if the status area
// is clicked very early in the test, so this function can be used to ensure it will be opened.
// Callers should also defer a call to Hide to ensure Quick Settings is closed between tests.
// TODO(crbug/1099502): remove this once there's a better indicator for when the status area
// is ready to receive clicks.
func ShowWithRetry(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) error {
	statusArea, err := findStatusArea(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find the status area widget")
	}

	ui := uiauto.New(tconn)
	if err := ui.WithPollOpts(testing.PollOptions{Timeout: timeout, Interval: time.Second}).
		LeftClickUntil(statusArea, ui.Exists(GetRootFinder()))(ctx); err != nil {
		return errors.Wrap(err, "quick settings not shown")
	}
	return nil
}

// PodIconButton generates nodewith.Finder for the specified quick setting feature pod icon button.
// This function is not supported with QsRevamp. Use the constants quicksettings.FeatureTile*
// instead.
func PodIconButton(setting SettingPod) *nodewith.Finder {
	if qsRevampEnabled {
		// Not supported with QsRevamp, see function docs.
		return nil
	}

	// The network pod cannot be easily found by its Name attribute in both logged-in and lock screen states.
	// Instead, find it by its unique ClassName.
	if setting == SettingPodNetwork {
		return nodewith.HasClass("NetworkFeaturePodButton")
	}

	// The pod icon names change based on their state, but a substring containing the setting name stays
	// the same regardless of state, so we can match that in the name attribute.
	return nodewith.HasClass("FeaturePodIconButton").NameContaining(string(setting))
}

// PodLabelButton generates nodewith.Finder to enter the panel of the specified quick setting pod.
// This function is not supported with QsRevamp. Use the constants quicksettings.FeatureTile*
// instead.
func PodLabelButton(setting SettingPod) *nodewith.Finder {
	if qsRevampEnabled {
		// Not supported with QsRevamp, see function docs.
		return nil
	}

	if setting == SettingPodDoNotDisturb {
		return nodewith.HasClass("FeaturePodLabelButton").NameContaining("notification")
	}

	return nodewith.HasClass("FeaturePodLabelButton").NameContaining(string(setting))
}

// ensureVisible ensures that Quick Settings is shown. If it's not visible, this function will
// show Quick Settings and return a cleanup function to hide it. If it is already visible,
// this function will do nothing and the returned function will do nothing, since no cleanup is required.
func ensureVisible(ctx context.Context, tconn *chrome.TestConn) (func(ctx context.Context) error, error) {
	shown, err := Shown(ctx, tconn)
	if err != nil {
		return nil, err
	}

	if !shown {
		if err := Show(ctx, tconn); err != nil {
			return nil, err
		}
		return func(ctx context.Context) error {
			return Hide(ctx, tconn)
		}, nil
	}

	return func(ctx context.Context) error {
		return nil
	}, nil
}

// BluetoothEnabled returns whether the bluetooth feature tile is toggled in
// quick settings. In order to check the setting, quick settings will be shown
// if it's not already, but the original state will be restored once the check
// is complete.
func BluetoothEnabled(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	if !qsRevampEnabled {
		return false, errors.New("BluetoothEnabled() requires QsRevamp enabled")
	}

	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return false, err
	}
	defer cleanup(ctx)

	ui := uiauto.New(tconn)
	// FeatureTiles do not expose an explicit toggled state to the UI node tree
	// so the best we can do is search for the tooltip string.
	return ui.IsNodeFound(ctx, FeatureTileBluetooth.NameContaining("Bluetooth is on"))
}

// SettingEnabled checks if the specified quick setting is on or off.
// In order to check the setting, Quick Settings will be shown if it's not already,
// but the original state will be restored once the check is complete.
func SettingEnabled(ctx context.Context, tconn *chrome.TestConn, setting SettingPod) (bool, error) {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return false, err
	}
	defer cleanup(ctx)

	pod := PodIconButton(setting)
	ui := uiauto.New(tconn)
	info, err := ui.Info(ctx, pod)
	if err != nil {
		return false, errors.Wrap(err, "failed to get the pod icon button info")
	}
	switch status := info.Checked; status {
	case checked.True:
		return true, nil
	case checked.False:
		return false, nil
	default:
		return false, errors.New("invalid checked state for pod icon button; quick setting may not be toggleable")
	}
}

// ToggleSetting toggles a quick setting by clicking the corresponding pod icon.
// If Quick Settings is not already shown, it will be opened and then closed once the setting is toggled.
func ToggleSetting(ctx context.Context, tconn *chrome.TestConn, setting SettingPod, enable bool) error {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return err
	}
	defer cleanup(ctx)

	if currentState, err := SettingEnabled(ctx, tconn, setting); err != nil {
		return errors.Wrap(err, "failed to get initial setting state")
	} else if currentState == enable {
		return nil
	}

	pod := PodIconButton(setting)
	ui := uiauto.New(tconn)
	if err := ui.LeftClick(pod)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the pod icon button")
	}
	return nil
}

// IsToggleOptionEnabled checks if the specified toggle option is on or off.
func IsToggleOptionEnabled(ctx context.Context, tconn *chrome.TestConn, toggleButton *nodewith.Finder) (bool, error) {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return false, err
	}
	defer cleanup(ctx)

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(toggleButton)(ctx); err != nil {
		return false, errors.Wrap(err, "failed to fine the toggle option")
	}
	info, err := ui.Info(ctx, toggleButton)
	if err != nil {
		return false, errors.Wrap(err, "failed to get the toggle option info")
	}
	switch status := info.Checked; status {
	case checked.True:
		return true, nil
	case checked.False:
		return false, nil
	default:
		return false, errors.New("invalid checked state for toggle option; quick setting may not be toggleable")
	}
}

// ToggleOption toggles a toggle option by clicking the corresponding toggle button.
func ToggleOption(ctx context.Context, tconn *chrome.TestConn, toggleButton *nodewith.Finder, enable bool) error {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return err
	}
	defer cleanup(ctx)

	if currentState, err := IsToggleOptionEnabled(ctx, tconn, toggleButton); err != nil {
		return errors.Wrap(err, "failed to get initial setting state")
	} else if currentState == enable {
		return nil
	}

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(toggleButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the pod icon button")
	}
	return nil
}

// TileRestricted checks if a feature tile is restricted and unable to be used
// on the lock screen.
func TileRestricted(ctx context.Context, tconn *chrome.TestConn, tile *nodewith.Finder) (bool, error) {
	if !qsRevampEnabled {
		return false, errors.New("TileRestricted() requires QsRevamp enabled")
	}

	// It's slow to open and close quick settings each time a caller wants to
	// verify a single tile is restricted. Instead require the caller to show
	// settings once.
	if shown, err := Shown(ctx, tconn); err != nil {
		return false, errors.Wrap(err, "failed to check Shown()")
	} else if !shown {
		return false, errors.Wrap(err, "quick settings must be visible to call TileRestricted()")
	}

	ui := uiauto.New(tconn)
	info, err := ui.Info(ctx, tile)
	if err != nil {
		return false, errors.Wrap(err, "failed to get the feature tile info")
	}
	return info.Restriction == restriction.Disabled, nil
}

// OpenSettingsApp will launch the Settings app by clicking on the Settings icon and wait
// for its icon to appear in the shelf. Quick Settings will be opened if not already shown.
func OpenSettingsApp(ctx context.Context, tconn *chrome.TestConn) error {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return err
	}
	defer cleanup(ctx)

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(SettingsButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find settings top shortcut button")
	}

	// Try clicking the Settings button until it goes away, indicating the click was received.
	// todo(crbug/1099502): determine when this is clickable, and just click it once.
	opts := testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond}
	if err := ui.WithPollOpts(opts).LeftClickUntil(SettingsButton, ui.Gone(SettingsButton))(ctx); err != nil {
		return errors.Wrap(err, "settings button still present after clicking")
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		return errors.Wrapf(err, "settings app did not open within %v seconds", uiTimeout)
	}

	return nil
}

// LockScreen locks the screen.
func LockScreen(ctx context.Context, tconn *chrome.TestConn) error {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return err
	}
	defer cleanup(ctx)

	ui := uiauto.New(tconn)
	if qsRevampEnabled {
		if err := ui.WithTimeout(uiTimeout).LeftClick(PowerMenuButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to find and click power menu button")
		}
		if err := ui.WithTimeout(uiTimeout).LeftClick(PowerMenuLockItem)(ctx); err != nil {
			return errors.Wrap(err, "failed to find and click power menu lock item")
		}
	} else {
		if err := ui.WithTimeout(uiTimeout).LeftClick(LockButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to find and click lock button")
		}
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, uiTimeout); err != nil {
		return errors.Wrapf(err, "waiting for screen to be locked failed (last status %+v)", st)
	}

	return nil

}

// ShowNotificationCenter opens the notification center. The notification
// center button does not show unless a notification exists, so this function
// first waits for the button to appear. This function is useful if a test
// wants to show a notification view without it timing out.
func ShowNotificationCenter(ctx context.Context, tconn *chrome.TestConn) error {
	if !qsRevampEnabled {
		return errors.New("ShowNotificationCenter() requires QsRevamp enabled")
	}
	ui := uiauto.New(tconn)
	notificationCenterIcon := nodewith.HasClass("NotificationCenterTray")
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(notificationCenterIcon)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the notification center icon to exist, perhaps there are no notifications?")
	}
	if err := ui.WithTimeout(uiTimeout).LeftClick(notificationCenterIcon)(ctx); err != nil {
		return errors.Wrap(err, "failed to click notification center icon")
	}
	return nil
}

// NotificationsHidden checks that the 'Notifications are hidden' notification
// appears and that no other notifications are visible.
func NotificationsHidden(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	if !qsRevampEnabled {
		return false, errors.New("NotificationsHidden() requires QsRevamp enabled")
	}
	// Open the notification center.
	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(uiTimeout).LeftClick(nodewith.HasClass("NotificationCenterTray"))(ctx); err != nil {
		return false, errors.Wrap(err, "failed to open notification center")
	}

	// Wait for the 'Notifications are hidden' notification to appear.
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(nodewith.Role(role.StaticText).NameStartingWith("Notifications are hidden"))(ctx); err != nil {
		return false, errors.Wrap(err, "failed to find notifications hidden text")
	}

	// Also check that no other notifications are shown in the UI. Must use
	// role.GenericContainer because the notification title has class
	// "AshNotificationView::NotificationTitleRow" and nodewith.HasClass() does
	// a substring match.
	nodes, err := ui.NodesInfo(ctx, nodewith.HasClass("AshNotificationView").Role(role.GenericContainer))
	if err != nil {
		return false, errors.Wrap(err, "failed getting notification node info")
	}
	success := len(nodes) == 1

	return success, nil
}

// findSlider finds the UI node for the specified slider. Callers should defer releasing the returned node.
func findSlider(ctx context.Context, tconn *chrome.TestConn, slider SliderType) (*nodewith.Finder, error) {
	// The mic gain slider is on the audio settings page of Quick Settings, so we need to navigate there first.
	if slider == SliderTypeMicGain {
		if err := OpenAudioSettings(ctx, tconn); err != nil {
			return nil, err
		}
	}

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(SliderParamMap[slider])(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed finding the %v slider", slider)
	}
	return SliderParamMap[slider], nil
}

// OpenAudioSettings opens Quick Settings' audio settings page. It does nothing if the page is already open.
func OpenAudioSettings(ctx context.Context, tconn *chrome.TestConn) error {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return err
	}
	defer cleanup(ctx)

	audioSettingsBtn := nodewith.Role(role.Button).Name("Audio settings").Onscreen()
	audioDetailedView := nodewith.HasClass("AudioDetailedView").Onscreen()

	ui := uiauto.New(tconn)
	// If audio settings view is open, just return.
	if err := ui.EnsureExistsFor(audioDetailedView, time.Second)(ctx); err == nil {
		return nil
	} else if !nodewith.IsNodeNotFoundErr(err) {
		return errors.Wrap(err, "failed to check the existence of the audio detailed view")
	}

	// Expand the Quick Settings if it is collapsed.
	if err := Expand(ctx, tconn); err != nil {
		return err
	}

	// It worth noting that LeftClickUntil will check the condition before doing the first
	// left click. This actually gives time for the UI to be stable before clicking.
	if err := ui.WithTimeout(uiTimeout).LeftClickUntil(audioSettingsBtn, ui.Exists(audioDetailedView))(ctx); err != nil {
		return errors.Wrap(err, "failed to click audio settings button to show audio detailed view")
	}

	return nil
}

// SliderValue returns the slider value as an integer.
// The slider node's value taken directly is a string expressing a percentage, like "50%".
func SliderValue(ctx context.Context, tconn *chrome.TestConn, slider SliderType) (int, error) {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return 0, err
	}
	defer cleanup(ctx)

	s, err := findSlider(ctx, tconn, slider)
	if err != nil {
		return 0, err
	}

	ui := uiauto.New(tconn)
	info, err := ui.Info(ctx, s)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get the slider info")
	}
	percent := strings.Replace(info.Value, "%", "", 1)
	level, err := strconv.Atoi(percent)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert %v to int", percent)
	}
	return level, nil
}

// focusSlider puts the keyboard focus on the slider. The keyboard can then be used to change the slider level.
// TODO(crbug/1123231): use better slider automation controls if possible, instead of keyboard controls.
func focusSlider(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, slider SliderType) error {
	s, err := findSlider(ctx, tconn, slider)
	if err != nil {
		return err
	}

	ui := uiauto.New(tconn)
	info, err := ui.Info(ctx, s)
	if err != nil {
		return errors.Wrap(err, "failed to get the slider info")
	}
	// Return if already focused.
	if info.State[state.Focused] {
		return nil
	}

	// Press tab to ensure keyboard focus is already in Quick Settings, otherwise it may not receive the focus.
	if err := kb.Accel(ctx, "Tab"); err != nil {
		return errors.Wrap(err, "failed to press tab key")
	}

	if err := ui.WithTimeout(uiTimeout).EnsureFocused(s)(ctx); err != nil {
		return errors.Wrapf(err, "failed to focus the %v slider", slider)
	}
	return nil
}

// changeSlider increments or decrements the slider using the keyboard.
// TODO(crbug/1123231): use better slider automation controls if possible, instead of keyboard controls.
func changeSlider(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, slider SliderType, increase bool) error {
	key := "up"
	if !increase {
		key = "down"
	}

	if err := focusSlider(ctx, tconn, kb, slider); err != nil {
		return err
	}

	initial, err := SliderValue(ctx, tconn, slider)
	if err != nil {
		return err
	}

	if err := kb.Accel(ctx, key); err != nil {
		return errors.Wrapf(err, "failed to press %v arrow key", key)
	}

	// The slider shouldn't move at all if we try to increase/decrease past the min/max value.
	// The polling below will time out in these cases, so just return immediately after pressing the key.
	if (initial == 0 && !increase) || (initial == 100 && increase) {
		return nil
	}

	// GoBigSleepLint: testing.Poll() is flaky for quicksettings.Brightness/VolumeSlider that sometimes it cannot get the correct value.
	// Sleeps for 10 seconds before entering the polling to ensure SliderValue() returns the updated value.
	testing.Sleep(ctx, time.Second*10)

	// The value changes smoothly as the slider animates, so wait for it to finish before returning the final value.
	previous := initial
	slidingDone := func(ctx context.Context) error {
		if current, err := SliderValue(ctx, tconn, slider); err != nil {
			return testing.PollBreak(err)
		} else if current == initial {
			return errors.New("slider hasn't started moving yet")
		} else if current == previous {
			return nil
		} else if (increase && current < previous) || (!increase && current > previous) {
			return testing.PollBreak(errors.Errorf("slider moved opposite of the expected direction from %v to %v", previous, current))
		} else {
			previous = current
			return errors.New("slider still sliding")
		}
	}

	if err := testing.Poll(ctx, slidingDone, &testing.PollOptions{Interval: 500 * time.Millisecond, Timeout: uiTimeout}); err != nil {
		return errors.Wrap(err, "failed waiting for slider animation to complete")
	}
	return nil
}

// IncreaseSlider increments the slider positively using the keyboard and returns the new level.
func IncreaseSlider(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, slider SliderType) (int, error) {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return 0, err
	}
	defer cleanup(ctx)

	if err := changeSlider(ctx, tconn, kb, slider, true); err != nil {
		return 0, err
	}

	return SliderValue(ctx, tconn, slider)
}

// DecreaseSlider increments the slider positively using the keyboard and returns the new level.
func DecreaseSlider(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, slider SliderType) (int, error) {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return 0, err
	}
	defer cleanup(ctx)

	if err := changeSlider(ctx, tconn, kb, slider, false); err != nil {
		return 0, err
	}

	return SliderValue(ctx, tconn, slider)
}

// MicEnabled checks if the microphone is enabled (unmuted).
func MicEnabled(ctx context.Context, tconn *chrome.TestConn) (bool, error) {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return false, err
	}
	defer cleanup(ctx)

	if err := OpenAudioSettings(ctx, tconn); err != nil {
		return false, err
	}
	ui := uiauto.New(tconn)
	// Scroll the mic toggle into view.
	kb, err := input.Keyboard(ctx)
	defer kb.Close(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to setup keyboard")
	}
	if err := kb.Accel(ctx, "Tab"); err != nil {
		return false, errors.Wrap(err, "failed to press Tab to bring focus into Quick Settings")
	}
	if err := ui.EnsureFocused(MicToggle)(ctx); err != nil {
		return false, errors.Wrap(err, "failed to scroll mic toggle into view")
	}
	info, err := ui.Info(ctx, MicToggle)
	if err != nil {
		return false, errors.Wrap(err, "failed to get the pod icon button info")
	}
	return info.Checked == checked.True, nil
}

// ToggleMic toggles the microphone's enabled state by clicking the microphone icon adjacent to the slider.
// If the microphone is already in the desired state, this will do nothing.
func ToggleMic(ctx context.Context, tconn *chrome.TestConn, enable bool) error {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return err
	}
	defer cleanup(ctx)

	if err := OpenAudioSettings(ctx, tconn); err != nil {
		return err
	}
	if current, err := MicEnabled(ctx, tconn); err != nil {
		return err
	} else if current != enable {
		ui := uiauto.New(tconn)
		if err := ui.DoDefault(MicToggle)(ctx); err != nil {
			return errors.Wrap(err, "failed to click mic toggle button")
		}
	}
	return nil
}

// SelectAudioOption selects the audio input or output device with the given name from the audio settings page.
func SelectAudioOption(ctx context.Context, tconn *chrome.TestConn, device string) error {
	cleanup, err := ensureVisible(ctx, tconn)
	if err != nil {
		return err
	}
	defer cleanup(ctx)

	if err := OpenAudioSettings(ctx, tconn); err != nil {
		return err
	}
	ui := uiauto.New(tconn)
	option := nodewith.Role(role.CheckBox).NameStartingWith(device)

	// If there are several audio options available, the target option may be out of view.
	// Furthermore, chrome.automation occasionally reports the wrong location of the audio option after focusing it into view.
	// Using DoDefault here is more reliable since we cannot rely on a stable location from the a11y tree.
	if err := ui.DoDefault(option)(ctx); err != nil {
		return errors.Wrapf(err, "failed to click %v audio option", device)
	}

	return nil
}

// RestrictedFeatureTiles returns a map from a descriptive name to a
// nodewith.Finder for the feature tiles that are restricted when Quick Settings
// is opened while a user is not signed in.
func RestrictedFeatureTiles(ctx context.Context) (map[string]*nodewith.Finder, error) {
	tiles := map[string]*nodewith.Finder{
		"Network tile": FeatureTileNetwork,
	}

	// First check for the bluetooth pod on devices with at least 1 bluetooth adapter.
	// If bluetooth adapters exist, add the bluetooth tile to the tiles map.
	adapters, err := bluez.Adapters(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get Bluetooth adapters")
	}
	if len(adapters) > 0 {
		tiles["Bluetooth tile"] = FeatureTileBluetooth
	}

	return tiles, nil
}

// CommonElements returns a map that contains ui.FindParams for Quick Settings UI elements that are present in all sign-in states (signed in, signed out, screen locked).
// The keys of the map are descriptive names for the UI elements.
func CommonElements(ctx context.Context, tconn *chrome.TestConn, hasBattery, isLockedScreen bool) (map[string]*nodewith.Finder, error) {
	if !qsRevampEnabled {
		return nil, errors.New("CommonElements() requires QsRevamp enabled")
	}

	// Associate the params with a descriptive name for better error reporting.
	getNodes := map[string]*nodewith.Finder{
		"Volume slider":     VolumeSlider,
		"Brightness slider": BrightnessSlider,
		"Power menu":        PowerMenuButton,
	}

	if hasBattery {
		getNodes["Battery display"] = BatteryView
	}

	if isLockedScreen {
		// Check that the expected accessibility UI element is shown in Quick Settings.
		getNodes["Accessibility tile"] = FeatureTileAccessibility
	} else {
		// Get the restricted feaure tiles.
		tileMap, err := RestrictedFeatureTiles(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get the restricted pod param")
		}
		// Merge the maps.
		for k, v := range tileMap {
			getNodes[k] = v
		}

		// Add the accessibility and keyboard tiles.
		getNodes["Accessibility tile"] = FeatureTileAccessibility
		getNodes["Keyboard tile"] = FeatureTileKeyboard
	}
	return getNodes, nil
}

// SignOut signouts by clicking signout button in Uber tray.
func SignOut(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	if err := Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to open Uber tray")
	}

	if !qsRevampEnabled {
		// In legacy quick settings the sign out button is at the top level.
		buttonFound, err := ui.IsNodeFound(ctx, SignoutButton)
		if err != nil {
			return errors.Wrap(err, "failed to find the sign out button")
		}
		if !buttonFound {
			return errors.New("signout button was not found")
		}

		// We ignore errors here because when we click on "Sign out" button
		// Chrome shuts down and the connection is closed. So we will always get an
		// error.
		ui.LeftClick(SignoutButton)(ctx)
		return nil
	}

	// Sign out is in the power menu.
	if err := ui.WithTimeout(uiTimeout).LeftClick(PowerMenuButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find and click power menu button")
	}
	// Ignore errors here because clicking the "Sign out" item causes Chrome to
	// shut down, which closes the connection and always generates an error.
	ui.WithTimeout(uiTimeout).LeftClick(PowerMenuSignOutItem)(ctx)
	return nil
}

// TriggerAddingVPNDialog clicks VPN setting button in quick settings page
// then clicking "+" button to trigger ADD dialog.
// Note: VPN setting button is not shown in quick settings if no VPN added in OS setting.
func TriggerAddingVPNDialog(tconn *chrome.TestConn) uiauto.Action {
	ui := uiauto.New(tconn)

	return func(ctx context.Context) error {
		if err := Show(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to open Uber tray")
		}
		return uiauto.Combine("trigger adding VPN",
			ui.LeftClick(PodIconButton(SettingPodVPN)),
			ui.LeftClick(nodewith.Name("Add connection")),
		)(ctx)
	}
}

// StartCast casts by clicking Cast button in Uber tray.
func StartCast(tconn *chrome.TestConn) uiauto.Action {
	ui := uiauto.New(tconn)

	return func(ctx context.Context) error {
		if err := Expand(ctx, tconn); err != nil {
			return err
		}
		if err := ui.LeftClick(PodIconButton(SettingPodCast))(ctx); err != nil {
			return errors.Wrap(err, "failed to click cast button")
		}
		return nil
	}
}

// StopCast casts by clicking Stop button in notification.
func StopCast(tconn *chrome.TestConn) uiauto.Action {
	ui := uiauto.New(tconn)

	castingNotification := nodewith.NameContaining("Casting").HasClass("AshNotificationView")
	stopButton := nodewith.Name("Stop").Role(role.Button).Ancestor(castingNotification)
	return func(ctx context.Context) error {
		if err := Expand(ctx, tconn); err != nil {
			return err
		}
		if err := ui.LeftClick(stopButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to click stop cast button")
		}
		return nil
	}
}
