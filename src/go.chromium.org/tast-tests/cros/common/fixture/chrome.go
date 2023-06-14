// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtures defined in go.chromium.org/tast-tests/cros/local/chrome/fixture.go
const (
	// Logged into a user session.
	ChromeLoggedIn = "chromeLoggedIn"
	// Logged into a user session with --disable-sync flag.
	ChromeLoggedInDisableSync = "chromeLoggedInDisableSync"
	// Logged into a user session with --disable-sync flag and firmware updates disabled.
	ChromeLoggedInDisableSyncNoFwUpdate = "chromeLoggedInDisableSyncNoFwUpdate"
	// Logged into a guest user session
	ChromeLoggedInGuest = "chromeLoggedInGuest"
	// Logged into a user session with 100 fake apps.
	ChromeLoggedInWith100FakeApps = "chromeLoggedInWith100FakeApps"
	// Logged into a user session with 100 fake apps and battery saver enabled.
	ChromeLoggedInWith100FakeAppsWithBatterySaver = "chromeLoggedInWith100FakeAppsWithBatterySaver"
	// Logged into a user session with 100 fake apps and the passthrough command decoder enabled.
	ChromeLoggedInWith100FakeAppsPassthroughCmdDecoder = "chromeLoggedInWith100FakeAppsPassthroughCmdDecoder"
	// Logged into a session with Gaia user where CalendarView is enabled.
	ChromeLoggedInWithCalendarView = "chromeLoggedInWithCalendarView"
	// Logged into a session with Gaia user where there are events set up to join Hangout meetings.
	ChromeLoggedInWithCalendarEvents = "chromeLoggedInWithCalendarEvents"
	// Logged into a session with Gaia user where there are upcoming events.
	ChromeLoggedInWithUpcomingCalendarEvents = "chromeLoggedInWithUpcomingCalendarEvents"
	// Logged into a session with feature QsRevamp enabled.
	// TODO(b/252870625): Delete this after M-117 branch when QsRevamp launches
	// and we're sure the feature won't be rolled back in chrome.
	ChromeLoggedInQsRevampEnabled = "chromeLoggedInQsRevampEnabled"
	// Logged into a session with feature QsRevamp disabled.
	// TODO(b/252870625): Delete this after all tests are ported to support QsRevamp.
	ChromeLoggedInQsRevampDisabled = "chromeLoggedInQsRevampDisabled"
	// Logged into a session with Gaia user.
	ChromeLoggedInWithGaia = "chromeLoggedInWithGaia"
	// Logged into a user session to support thunderbolt devices.
	ChromeLoggedInThunderbolt = "chromeLoggedInThunderbolt"
	// Logged into a user session with OS Feedback enabled.
	ChromeLoggedInWithOsFeedback = "chromeLoggedInWithOsFeedback"
	// Logged into a user session with ShortcutCustomizationApp enabled.
	chromeLoggedInWithShortcutCustomizationApp = "chromeLoggedInWithShortcutCustomizationApp"
	// Logged into a user session that has continue section in the launcher enabled.
	ChromeLoggedInWithLauncherContinueSection = "chromeLoggedInWithLauncherContinueSection"
	// Log in and proceed with the post-login OOBE flow.
	ChromeLoggedInWithOobe = "chromeLoggedInWithOobe"
	// Logged into a user session with OS Feedback and OsFeedbackSaveReportToLocalForE2ETesting enabled.
	ChromeLoggedInWithOsFeedbackSaveReportToLocalForE2ETesting = "chromeLoggedInWithOsFeedbackSaveReportToLocalForE2ETesting"
	// Log in and proceed with the post-login OOBE flow with the accessibility button enabled on the marketing opt-in screen.
	ChromeLoggedInWithOobeAndAccessibilityButtonEnabled = "chromeLoggedInWithOobeAndAccessibilityButtonEnabled"
	// Logged into a user session; stack-sampled metrics on turned on.
	ChromeLoggedInWithStackSampledMetrics = "chromeLoggedInWithStackSampledMetrics"
	// Logged into a user session with FirmwareUpdaterApp disabled.
	ChromeLoggedInExtendedAutocomplete = "chromeLoggedInExtendedAutocomplete"
	// Logged into a user session with searchFeedbackEnabled flag enabled.
	ChromeLoggedInWithOsSettingsSearchFeedback = "chromeLoggedInWithOsSettingsSearchFeedback"
	// Logged into a guest user session with searchFeedbackEnabled flag enabled.
	ChromeLoggedInGuestWithOsSettingsSearchFeedback = "chromeLoggedInGuestWithOsSettingsSearchFeedback"
	// Ownership cleaned, logged into a user session with flags to enable verbose logging about consent.
	ChromeLoggedInVerboseConsentLogs = "chromeLoggedInVerboseConsentLogs"
	// Logged into a user session with PasspointARCSupport flag enabled.
	ChromeLoggedInWithPasspoint = "chromeLoggedInWithPasspoint"
	// Logged into a user session with InputDeviceSettingsSplit enabled.
	ChromeLoggedInWithInputDeviceSettingsSplit = "chromeLoggedInWithInputDeviceSettingsSplit"
	// Logged into a user session with VM display marked as external, allowing display mode change.
	ChromeLoggedInWithForceVMDisplayExternal = "chromeLoggedInWithForceVMDisplayExternal"
	// Logged into a user session with Jelly enabled.
	ChromeLoggedInWithJelly = "chromeLoggedInWithJelly"
)
