// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtures defined in go.chromium.org/tast-tests/cros/local/chrome/fixture.go
const (
	// Logged into a user session.
	ChromeLoggedIn = "chromeLoggedIn"
	// Logged into a user session with multi display enabled.
	ChromeLoggedInMultiDisplay = "chromeLoggedInMultiDisplay"
	// Logged into a user session with --disable-sync flag.
	ChromeLoggedInDisableSync = "chromeLoggedInDisableSync"
	// Logged into a user session with --disable-sync flag and turn on battery saver.
	ChromeLoggedInDisableSyncWithBatterySaver = "chromeLoggedInDisableSyncWithBatterySaver"
	// Logged into a user session with --disable-sync flag and firmware updates disabled.
	ChromeLoggedInDisableSyncNoFwUpdate = "chromeLoggedInDisableSyncNoFwUpdate"
	// Logged into a guest user session
	ChromeLoggedInGuest = "chromeLoggedInGuest"
	// Start Chrome without logging in.
	ChromeNotLoggedIn = "chromeNotLoggedIn"
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
	// Logged into a user session with printer setup assistance and jelly flags enabled.
	ChromeLoggedInWithPrinterSetupAssistance = "chromeLoggedInWithPrinterSetupAssistance"
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
	// Logged into a user session with InputDeviceSettingsSplit enabled.
	ChromeLoggedInWithInputDeviceSettingsSplit = "chromeLoggedInWithInputDeviceSettingsSplit"
	// Logged into a user session with VM display marked as external, allowing display mode change.
	ChromeLoggedInWithForceVMDisplayExternal = "chromeLoggedInWithForceVMDisplayExternal"
	// Logged in to a user session with FieldTrialConfigEnable.
	ChromeLoggedInWithFieldTrialConfigEnable = "chromeLoggedInWithFieldTrialConfigEnable"
	// Logged in to a user session with FieldTrialConfigDisable.
	ChromeLoggedInWithFieldTrialConfigDisable = "chromeLoggedInWithFieldTrialConfigDisable"
	// Logged in to a user session with FieldTrialConfigEnable *and* verbose consent flags.
	ChromeLoggedInWithFieldTrialConfigEnableAndVerboseConsent = "chromeLoggedInWithFieldTrialConfigEnableAndVerboseConsent"
	// Logged in to a user session with FieldTrialConfigDisable *and* verbose consent flags.
	ChromeLoggedInWithFieldTrialConfigDisableAndVerboseConsent = "chromeLoggedInWithFieldTrialConfigDisableAndVerboseConsent"
	// Logged in to a user session with SchedQoSOnResourcedForChrome feature enabled.
	ChromeLoggedInWithSchedQoS = "chromeLoggedInWithSchedQoS"
	// Logged in to a user session with oak feature enabled.
	ChromeLoggedInWithOak = "chromeLoggedInWithOak"
	// Logged into a user session with FaceGaze enabled.
	ChromeLoggedInDisableSyncWithFaceGaze = "chromeLoggedInDisableSyncWithFaceGaze"
)
