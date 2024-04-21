// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crossdevice is for controlling Cross Device features involving a paired Android phone and Chromebook.
package crossdevice

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/android/mobly"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DCIMPath is the path to the DCIM directory on Android.
const DCIMPath = "/sdcard/DCIM"

// AndroidPhotosPath is the directory where photos taken on the device can be located.
var AndroidPhotosPath = filepath.Join(DCIMPath, "Camera")

// AndroidDevice represents an Android device that's been paired with the Chromebook (i.e. from the "Connected devices" section of OS settings).
// Android control is achieved by making RPCs to the Multidevice Snippet running on the Android device, or by using ADB commands directly.
type AndroidDevice struct {
	Device        *adb.Device
	snippetClient *mobly.SnippetClient
}

// NewAndroidDevice returns an AndroidDevice that can be used to control the Android phone in Cross Device tests.
// Callers should defer Cleanup to ensure the resources used by the AndroidDevice are freed.
func NewAndroidDevice(ctx context.Context, d *adb.Device, apkZipPath string) (*AndroidDevice, error) {
	// Launch the snippet and create a client.
	snippetClient, err := mobly.NewSnippetClient(ctx, d, MultideviceSnippetMoblyPackage, apkZipPath, MultideviceSnippetApkName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start the snippet client for the Multidevice Snippet")
	}
	return &AndroidDevice{Device: d, snippetClient: snippetClient}, nil
}

// ReconnectToSnippet restarts a connection to the Multidevice Snippet on Android device.
func (c *AndroidDevice) ReconnectToSnippet(ctx context.Context) error {
	return c.snippetClient.ReconnectToSnippet(ctx)
}

// Cleanup stops the Multidevice Snippet, removes port forwarding, and closes the TCP connection.
// This should be deferred after calling NewAndroidDevice to ensure the resources used by the AndroidDevice are released at the end of tests.
func (c *AndroidDevice) Cleanup(ctx context.Context) {
	c.snippetClient.Cleanup(ctx)
}

// DumpLogs saves the Android device's logcat output to a file.
func (c *AndroidDevice) DumpLogs(ctx context.Context, outDir, filename string) error {
	filePath := filepath.Join(outDir, filename)
	if err := c.Device.DumpLogcat(ctx, filePath); err != nil {
		testing.ContextLog(ctx, "Failed to dump Android logs: ", err)
		return errors.Wrap(err, "failed to dump Android logs")
	}
	return nil
}

// ClearLogcat clears logcat so each test run can have only relevant logs.
func (c *AndroidDevice) ClearLogcat(ctx context.Context) error {
	if err := c.Device.ClearLogcat(ctx); err != nil {
		return errors.Wrap(err, "failed to clear previous logcat logs")
	}
	return nil
}

// StartScreenRecording starts screen recording on the Android device.
// Defer the returned function to save the recording and clean up on the Android side.
func (c *AndroidDevice) StartScreenRecording(ctx context.Context, filename, outDir string) (func(context.Context, func() bool) error, error) {
	return c.Device.StartScreenRecording(ctx, filename, outDir)
}

// Pair pairs the Android device to nearby Chromebooks signed in with the same GAIA account.
// This will cause the phone to be listed under the "Connected devices" section of OS settings,
// allowing use of Cross Device features (Smart Lock, Phone Hub, etc.) on the Chromebook.
// Calling this function effectively bypasses the crossdevice onboarding flow that is triggered from OOBE or OS settings.
// One notable difference is unlike the normal onboarding flow, not all features in the "Connected devices" page will be
// enabled by default. Some (Phone Hub, WiFi Sync) may need to be toggled on after calling Pair.
func (c *AndroidDevice) Pair(ctx context.Context) error {
	user, err := c.Device.GoogleAccount(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get device user account")
	}
	res, err := c.snippetClient.RPC(ctx, mobly.DefaultRPCResponseTimeout, "enableBetterTogetherHost", user)
	if err != nil {
		return err
	}
	callbackID := res.Callback

	// Wait for the snippet to confirm that it has completed the crossdevice setup operation on the Android side.
	if _, err := c.snippetClient.EventWaitAndGet(ctx, callbackID, "onBeToHostEnableStatus", 30*time.Second); err != nil {
		return errors.Wrap(err, "failed waiting for onBeToHostEnableStatus event to know that crossdevice setup is complete on Android")
	}

	return nil
}

// GenerateMessageNotification creates a message notification on the phone.
// The notification has inline reply enabled, so it can be used to test Phone Hub's notification features.
// The notification title and message text can be specified by the inputs.
// You can create multiple distinct notifications by calling this with different notification IDs.
// The returned function will retrieve the reply to the notification.
func (c *AndroidDevice) GenerateMessageNotification(ctx context.Context, id int, title, text string) (func(context.Context) (string, error), error) {
	res, err := c.snippetClient.RPC(ctx, mobly.DefaultRPCResponseTimeout, "generateMessageNotification", title, text, id)
	if err != nil {
		return nil, err
	}
	callbackID := res.Callback

	// Return a function that will wait for a reply to the message notification and return the replied text.
	return func(ctx context.Context) (string, error) {
		res, err := c.snippetClient.EventWaitAndGet(ctx, callbackID, "replyReceived", 10*time.Second)
		if err != nil {
			return "", errors.Wrap(err, "failed to wait for replyReceived snippet event")
		}
		reply, ok := res.Data["reply"]
		if !ok {
			return "", errors.New("replyReceived event did not contain a reply")
		}
		replyStr, ok := reply.(string)
		if !ok {
			return "", errors.Wrap(err, "reply in replyReceived's response was not a string")
		}
		return replyStr, nil
	}, nil
}

// EnablePhoneHubNotifications sets the flag on Android to allow Phone Hub to receive notification updates.
func (c *AndroidDevice) EnablePhoneHubNotifications(ctx context.Context) error {
	return c.Device.ShellCommand(ctx, "cmd", "notification", "allow_listener", "com.google.android.gms/.auth.proximity.phonehub.PhoneHubNotificationListenerService").Run()
}

// SetPIN sets a screen lock PIN on Android.
func (c *AndroidDevice) SetPIN(ctx context.Context) error {
	if err := c.Device.SetPIN(ctx); err != nil {
		return errors.Wrap(err, "failed to set a screen lock PIN on Android")
	}
	return nil
}

// WaitForPINChallenge waits for the lockscreen PIN prompt.
func (c *AndroidDevice) WaitForPINChallenge(ctx context.Context) error {
	d, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return errors.Wrap(err, "failed initializing UI automator")
	}
	defer d.Close(ctx)

	pinTextField := d.Object(ui.ResourceID("com.android.systemui:id/lockPassword"))
	if err = pinTextField.WaitForExists(ctx, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed to find the PIN verification prompt")
	}

	return nil
}

// EnterPIN enters the default PIN on the challenge screen.
func (c *AndroidDevice) EnterPIN(ctx context.Context) error {
	d, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return errors.Wrap(err, "failed initializing UI automator")
	}
	defer d.Close(ctx)

	// Enter the default code '1234'
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_1))); err != nil {
		return errors.Wrap(err, "failed to enter PIN")
	}
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_2))); err != nil {
		return errors.Wrap(err, "failed to enter PIN")
	}
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_3))); err != nil {
		return errors.Wrap(err, "failed to enter PIN")
	}
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_4))); err != nil {
		return errors.Wrap(err, "failed to enter PIN")
	}
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_ENTER))); err != nil {
		return errors.Wrap(err, "failed to enter PIN")
	}

	return nil
}

// ClearPIN clears a screen lock PIN on Android.
func (c *AndroidDevice) ClearPIN(ctx context.Context) error {
	if err := c.Device.ClearPIN(ctx); err != nil {
		return errors.Wrap(err, "failed to clear  a screen lock PIN on Android")
	}
	return nil
}

// GetAndroidAttributes returns the AndroidAttributes for the device.
func (c *AndroidDevice) GetAndroidAttributes(ctx context.Context) (*AndroidAttributes, error) {
	return GetAndroidAttributes(ctx, c.Device)
}

// ToggleDoNotDisturb toggles the Do Not Disturb setting on the Android device.
func (c *AndroidDevice) ToggleDoNotDisturb(ctx context.Context, enable bool) error {
	status := "off"
	if enable {
		status = "on"
	}
	if err := c.Device.ShellCommand(ctx, "cmd", "notification", "set_dnd", status).Run(); err != nil {
		return errors.Wrapf(err, "failed to set Do Not Disturb to %v", status)
	}
	return nil
}

// DoNotDisturbEnabled returns true if Do Not Disturb is enabled, and false if it is disabled.
func (c *AndroidDevice) DoNotDisturbEnabled(ctx context.Context) (bool, error) {
	res, err := c.Device.ShellCommand(ctx, "sh", "-c", "settings list global | grep zen_mode=").Output(testexec.DumpLogOnError)
	if err != nil {
		return false, errors.Wrap(err, "failed to get Do Not Disturb status")
	}
	if strings.Contains(string(res), "0") {
		return false, nil
	}
	// Any status that's not 0 corresponds to DND being enabled.
	return true, nil
}

// WaitForDoNotDisturb waits for Do Not Disturb to be enabled/disabled.
func (c *AndroidDevice) WaitForDoNotDisturb(ctx context.Context, enabled bool, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if curr, err := c.DoNotDisturbEnabled(ctx); err != nil {
			return err
		} else if curr != enabled {
			return errors.New("current Do Not Disturb status does not match the desired status")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed waiting for desired Do Not Disturb status")
	}
	return nil
}

// ChromeRunning returns true if the Chrome activity is running.
func (c *AndroidDevice) ChromeRunning(ctx context.Context) (bool, error) {
	out, err := c.Device.ShellCommand(ctx, "dumpsys", "activity", "activities").Output(testexec.DumpLogOnError)
	if err != nil {
		return false, err
	}

	r := regexp.MustCompile("ResumedActivity.*" + chromePkg)
	match := r.Find(out)
	if len(match) > 0 {
		return true, nil
	}
	return false, nil
}

// WaitForChromeRunning waits for the Chrome activity to be running.
func (c *AndroidDevice) WaitForChromeRunning(ctx context.Context, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if running, err := c.ChromeRunning(ctx); err != nil {
			return err
		} else if !running {
			return errors.New("Chrome is not running")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed waiting for Chrome to start")
	}
	return nil
}

// FindMyPhoneActive returns true if the "Find my phone" alarm is ringing.
func (c *AndroidDevice) FindMyPhoneActive(ctx context.Context) (bool, error) {
	out, err := c.Device.ShellCommand(ctx, "dumpsys", "audio").Output(testexec.DumpLogOnError)
	if err != nil {
		return false, err
	}

	r := regexp.MustCompile("AudioPlaybackConfiguration.*state:started.*usage=USAGE_ALARM")
	match := r.Find(out)
	if len(match) > 0 {
		return true, nil
	}
	return false, nil
}

// WaitForFindMyPhone waits for "Find my phone" to be on/off.
func (c *AndroidDevice) WaitForFindMyPhone(ctx context.Context, active bool, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if curr, err := c.FindMyPhoneActive(ctx); err != nil {
			return err
		} else if curr != active {
			return errors.New("current 'Find my phone' status does not match the desired status")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed waiting for desired 'Find my phone' status")
	}
	return nil
}

// ToggleScreen turns the screen off and back on again.
// This can also be used to easily disable the loud "Find my phone" ringer in case we fail to do so from the UI.
func (c *AndroidDevice) ToggleScreen(ctx context.Context) error {
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_POWER))); err != nil {
		return errors.Wrap(err, "failed to turn off the screen")
	}
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_WAKEUP))); err != nil {
		return errors.Wrap(err, "failed to wake screen")
	}
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_MENU))); err != nil {
		return errors.Wrap(err, "failed to wake screen")
	}
	return nil
}

// GrantPermissionOnCdmDialog enables the recent photos feature through the phone side set up dialog.
func (c *AndroidDevice) GrantPermissionOnCdmDialog(ctx context.Context) error {
	uiDevice, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the UI Automator server")
	}
	defer uiDevice.Close(ctx)

	turnOnButton := uiDevice.Object(ui.TextMatches("Allow"), ui.Clickable(true))
	if err := turnOnButton.WaitForExists(ctx, 30*time.Second); err != nil {
		return errors.Wrap(err, "setup dialog did not appear")
	}

	if err := turnOnButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to turn on features")
	}
	return nil
}

// TakePhoto takes a photo with Camera app and returns the name of the new photo taken.
func (c *AndroidDevice) TakePhoto(ctx context.Context) (string, error) {
	if err := c.Device.SendIntentCommand(ctx, "android.media.action.STILL_IMAGE_CAMERA", "").Run(); err != nil {
		return "", errors.Wrap(err, "failed to open camera")
	}
	// Close the Camera app by pressing the back button when this function exits.
	defer c.Device.PressKeyCode(ctx, "KEYCODE_BACK")

	mostRecentPhoto, err := c.GetMostRecentPhoto(ctx)
	if err != nil {
		return "", err
	}

	uiDevice, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to the UI Automator server")
	}
	defer uiDevice.Close(ctx)

	shutterButton := uiDevice.Object(ui.ResourceIDMatches(".*/shutter_button"))
	if err := shutterButton.WaitForExists(ctx, 3*time.Second); err != nil {
		shutterButton = uiDevice.Object(ui.DescriptionMatches("Take.*photo"))
		if err := shutterButton.WaitForExists(ctx, 3*time.Second); err != nil {
			return "", errors.Wrap(err, "cannot find the shutter button in the Google Camera app")
		}
	}

	if err := shutterButton.Click(ctx); err != nil {
		return "", errors.Wrap(err, "failed to take a photo")
	}

	// Wait for a new photo to appear.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if photo, err := c.GetMostRecentPhoto(ctx); err != nil {
			return testing.PollBreak(err)
		} else if photo == mostRecentPhoto {
			return errors.Errorf("Still waiting for new photo to appear, current most recent photo is %s", mostRecentPhoto)
		} else {
			mostRecentPhoto = photo
			return nil
		}
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return "", errors.Wrap(err, "no new photo found")
	}

	return mostRecentPhoto, nil
}

// GetMostRecentPhoto returns the file name of the most recent photo taken on the device.
// Returns an empty string if there's no photo on the device.
func (c *AndroidDevice) GetMostRecentPhoto(ctx context.Context) (string, error) {
	// Sometimes the Camera folder may not exist, so check that first.
	contents, err := c.Device.ListContents(ctx, DCIMPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list contents of %s", DCIMPath)
	}

	photosPathExists := false
	for _, f := range contents {
		if filepath.Join(DCIMPath, f) == AndroidPhotosPath {
			photosPathExists = true
		}
	}

	// If the photos path doesn't exist, there is no recent photo. It will be created when a photo is next taken.
	if !photosPathExists {
		return "", nil
	}

	photos, err := c.Device.ListContents(ctx, AndroidPhotosPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list files under %s", AndroidPhotosPath)
	}
	mostRecentPhoto := ""
	if len(photos) > 0 {
		mostRecentPhoto = photos[len(photos)-1]
	}
	return mostRecentPhoto, nil
}

// FileSize returns the size of the specified file in bytes. Returns an error if the file does not exist.
func (c *AndroidDevice) FileSize(ctx context.Context, filename string) (int64, error) {
	return c.Device.FileSize(ctx, filename)
}

// SHA256Sum returns the sha256sum of the specified file as a string.
func (c *AndroidDevice) SHA256Sum(ctx context.Context, filename string) (string, error) {
	return c.Device.SHA256Sum(ctx, filename)
}

// RemoveMediaFile removes the media file specified by filePath from the Android device's storage and media gallery.
func (c *AndroidDevice) RemoveMediaFile(ctx context.Context, filePath string) error {
	return c.Device.RemoveMediaFile(ctx, filePath)
}

// BatteryLevel returns the current battery level.
func (c *AndroidDevice) BatteryLevel(ctx context.Context) (int, error) {
	res, err := c.Device.ShellCommand(ctx, "dumpsys", "battery").Output(testexec.DumpLogOnError)
	if err != nil {
		return -1, errors.Wrap(err, "failed to get battery status")
	}

	r := regexp.MustCompile(`level: (\d+)`)
	m := r.FindStringSubmatch(string(res))
	if len(m) == 0 {
		return -1, errors.Wrap(err, "failed to extract battery percentage from adb response")
	}
	level, err := strconv.Atoi(m[1])
	if err != nil {
		return -1, errors.Wrapf(err, "failed to convert battery level %v to int", m[0])
	}
	return level, nil
}

// Constants for interacting with the Chrome app on Android.
const (
	chromePkg    = "com.android.chrome"
	chromeIntent = chromePkg + "/com.google.android.apps.chrome.Main"
)

// LaunchChrome launches Chrome on the Android device.
func (c *AndroidDevice) LaunchChrome(ctx context.Context) error {
	if err := c.Device.ShellCommand(ctx, "am", "start", "-n", chromeIntent).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to launch Chrome browser on Android")
	}

	return nil
}

// ClearChromeAppDataAndLaunchChrome deletes any local app data associated with Chrome before launching Chrome..
func (c *AndroidDevice) ClearChromeAppDataAndLaunchChrome(ctx context.Context) error {
	var err error = nil

	// Clear Chrome's data.
	if err = c.Device.ShellCommand(ctx, "pm", "clear", chromePkg).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to clear Chrome app data")
	}

	// Try launching Chrome up to three times. The system kills any Chrome processes when it
	// clears out the app data, which can cause this to fail.
	for attempt := 1; attempt <= 3; attempt++ {
		if err = c.LaunchChrome(ctx); err != nil {
			return err
		}
		// GoBigSleepLint: Wait a second in case the process gets killed shortly after launch.
		testing.Sleep(ctx, 1*time.Second)
		if err = c.WaitForChromeRunning(ctx, 2*time.Second); err == nil {
			return nil
		}
		testing.ContextLog(ctx, "Failed waiting for Chrome to launch, retrying")
	}

	return err
}

// LaunchChromeAtURL opens the specified URL in Chrome.
func (c *AndroidDevice) LaunchChromeAtURL(ctx context.Context, url string) error {
	if err := c.Device.ShellCommand(ctx, "am", "start", "-n", chromeIntent, "-d", url).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to launch Chrome browser on Android with URL %v", url)
	}
	return nil
}

// EnableChromeSync turns on Chrome Sync using the UI.
func (c *AndroidDevice) EnableChromeSync(ctx context.Context) error {
	// Clear Chrome's data before launching Chrome so we have a fresh start. This makes it easier to turn on Chrome Sync through the UI.
	if err := c.ClearChromeAppDataAndLaunchChrome(ctx); err != nil {
		return err
	}

	// Set up uiautomator for UI controls.
	d, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return errors.Wrap(err, "failed initializing UI automator")
	}
	defer d.Close(ctx)

	// Since we cleared Chrome's data to force a first-run state, we have to accept the ToS.
	acceptBtn := d.Object(ui.ResourceID("com.android.chrome:id/terms_accept"))
	if err := acceptBtn.WaitForExists(ctx, 3*time.Second); err == nil { // button exists, i.e. first run of Chrome
		if err := acceptBtn.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to accept the ToS on first run of Chrome")
		}
	}

	// Accepting the ToS starts the chrome sync opt-in flow, starting with account selection on more recent builds.
	acceptBtn = d.Object(ui.ResourceID("com.android.chrome:id/signin_fre_continue_button"))
	if err := acceptBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		testing.ContextLog(ctx, "Failed to find account selection page: ", err)
		// Fall through to next opt-in step when account selection button is not found.
	} else if err := acceptBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to select account for Chrome Sync")
	}

	// After selecting the account, confirm turning on sync. Skip if confirmation screen is not found.
	// Recent builds have Sync Enable happening on first time run.
	signInSync := d.Object(ui.ResourceID("com.android.chrome:id/signin_sync_title"))
	confirmBtn := d.Object(ui.ResourceID("com.android.chrome:id/positive_button"))
	if err := signInSync.WaitForExists(ctx, 3*time.Second); err != nil {
		// Fall through to next opt-in step when no sign in sync confirmation screen is found.
		testing.ContextLog(ctx, "Failed to find signin sync confirmation screen: ", err)
	} else if err := confirmBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find the confirmation button for Chrome Sync")
	} else if err := confirmBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click the confirmation button for Chrome Sync")
	} else {
		// If turning on sync is successful on first time run, we can exit this flow early.
		return c.EnableSystemLevelAppSync(ctx, d)
	}

	// Ignore notifications. Skip if no notification permission message shows up.
	notificationDialog := d.Object(ui.ResourceID("com.android.chrome:id/notification_permission_rationale_title"))
	ignoreBtn := d.Object(ui.ResourceID("com.android.chrome:id/negative_button"))
	if err := notificationDialog.WaitForExists(ctx, 3*time.Second); err != nil {
		// Fall through to next opt-in step when no notification dialog is found.
		testing.ContextLog(ctx, "Failed to find notification permissions dialog: ", err)
	} else if err := ignoreBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find the ignore notifications button")
	} else if err := ignoreBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click the ignore notifications button")
	}

	// After selecting the account, go to the Chrome settings page to finish the opt-in flow.
	menuBtn := d.Object(ui.ResourceID("com.android.chrome:id/menu_button"))
	if err := menuBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find Android Chrome menu button")
	}
	if err := menuBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to open Android Chrome menu")
	}

	settingsBtn := d.Object(ui.ResourceID("com.android.chrome:id/menu_item_text"), ui.TextMatches("(?i)settings"))
	if err := settingsBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find Android Chrome settings button")
	}
	if err := settingsBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to open Android Chrome settings")
	}

	continueBtn := d.Object(ui.ResourceID("com.android.chrome:id/signin_promo_signin_button"))
	if err := continueBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find settings menu Chrome Sync opt-in button")
	}
	if err := continueBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click settings menu Chrome Sync opt-in button")
	}

	acceptBtn = d.Object(ui.ResourceID("com.android.chrome:id/positive_button"))
	if err := acceptBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find final opt-in button")
	}
	if err := acceptBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click final opt-in button")
	}

	return c.EnableSystemLevelAppSync(ctx, d)
}

// EnableSystemLevelAppSync gets phones out of the state where Chrome Sync and Android App sync are coupled.
func (c *AndroidDevice) EnableSystemLevelAppSync(ctx context.Context, d *ui.Device) error {
	if err := c.Device.ShellCommand(ctx, "am", "start", "-a", "android.settings.SYNC_SETTINGS").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to open Android account settings")
	}
	syncToggle := d.Object(ui.ResourceID("android:id/switch_widget"))
	if err := syncToggle.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find Android sync toggle")
	}
	syncEnabled, err := syncToggle.IsChecked(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Android sync status")
	}
	if syncEnabled {
		return nil
	}
	if err := syncToggle.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle Android sync")
	}
	okBtn := d.Object(ui.ClassName("android.widget.Button"), ui.TextMatches("(?i)ok"))
	if err := okBtn.WaitForExists(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to find 'OK' button to turn on auto sync")
	}
	if err := okBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click 'OK' button to turn on auto sync")
	}
	return nil
}

// AcceptTetherNotification accepts the notification on the phone that's needed
// to enable Instant Tether on CrOS.
func (c *AndroidDevice) AcceptTetherNotification(ctx context.Context) error {
	uiDevice, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the UI Automator server")
	}
	defer uiDevice.Close(ctx)

	if err := uiDevice.OpenNotification(ctx); err != nil {
		return errors.Wrap(err, "failed to show notifications on Android")
	}

	provideDataBtn := uiDevice.Object(ui.ResourceID("android:id/title"), ui.TextMatches("(?i)provide data connection"))
	if err := provideDataBtn.WaitForExists(ctx, 10*time.Second); err != nil {
		testing.ContextLog(ctx, "notification to provide data did not appear, assuming first-time setup has been completed")
		return nil
	}

	if err := provideDataBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click notification")
	}

	continueBtn := uiDevice.Object(ui.ClassName("android.widget.Button"), ui.ResourceID("android:id/button1"))
	if err := continueBtn.WaitForExists(ctx, 30*time.Second); err != nil {
		return errors.Wrap(err, "continue button did not appear")
	}

	if err := continueBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click continue button")
	}
	return nil
}

// AcceptFastPairHalfsheet automates halfsheet verify and click using the UI.
func (c *AndroidDevice) AcceptFastPairHalfsheet(ctx context.Context) error {
	d, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return errors.Wrap(err, "failed initializing UI automator")
	}
	defer d.Close(ctx)

	// Close the wallpaper editing overlay on the phone. ui.KEYCODE_MENU can cause various tap+hold triggered menus to appear.
	if err := c.Device.PressKeyCode(ctx, strconv.Itoa(int(ui.KEYCODE_BACK))); err != nil {
		return errors.Wrap(err, "failed to close wallpaper overlay on phone")
	}

	// Verify that the Halfsheet showed up on the phone.
	card := d.Object(ui.ResourceID("com.google.android.gms:id/card"))
	if err := card.WaitForExists(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to find a card displaying the Halfsheet")
	}
	pairingPic := d.Object(ui.ResourceID("com.google.android.gms:id/pairing_pic"))
	if err = pairingPic.WaitForExists(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to find a Fast Pair Halfsheet with a pairing pic")
	}

	// Click on the Connect button after verifying the pair.
	connectBtn := d.Object(ui.ResourceID("com.google.android.gms:id/connect_btn"))
	if err = connectBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click on the Connect button for Fast Pair")
	}
	return nil
}

// ExtractQuickStartVerificationPIN finds the Quick Start verification PIN and returns it as a string.
func (c *AndroidDevice) ExtractQuickStartVerificationPIN(ctx context.Context) (string, error) {
	d, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return "", errors.Wrap(err, "failed initializing UI automator")
	}
	defer d.Close(ctx)

	pinElement := d.Object(ui.ResourceID("com.google.android.gms:id/pin"))
	if err := pinElement.WaitForExists(ctx, 3*time.Second); err != nil {
		return "", errors.Wrap(err, "failed to find Quick Start verification PIN on the phone")
	}

	pin, err := pinElement.GetText(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get text from PIN element on the phone")
	}

	return strings.ReplaceAll(pin, " ", ""), nil
}

// TapNext waits for a "Next" button to appear and taps it.
func (c *AndroidDevice) TapNext(ctx context.Context) error {
	d, err := ui.NewDeviceWithRetry(ctx, c.Device)
	if err != nil {
		return errors.Wrap(err, "failed initializing UI automator")
	}
	defer d.Close(ctx)

	nextBtn := d.Object(ui.Text("Next"))
	if err := nextBtn.WaitForExists(ctx, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed to find the Next button")
	}

	if err := nextBtn.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click on the Next button")
	}

	return nil
}
