// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	arcui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	permissionControllerPackage  = "com.android.permissioncontroller"
	allowRadioButtonID           = permissionControllerPackage + ":id/allow_radio_button"
	allowAlwaysRadioButtonID     = permissionControllerPackage + ":id/allow_always_radio_button"
	allowForegroundRadioButtonID = permissionControllerPackage + ":id/allow_foreground_only_radio_button"
	askRadioButtonID             = permissionControllerPackage + ":id/ask_radio_button"
	denyRadioButtonID            = permissionControllerPackage + ":id/deny_radio_button"
)

type permissionState struct {
	name  string
	state string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppManagement,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies integration of ARC apps into the OS Settings App Management UI",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"tsergeant@chromium.org",
		},
		BugComponent: "b:1203766",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		// Read-only permissions is currently only enabled on ARC-T.
		SoftwareDeps: []string{"chrome", "android_vm_t"},
		Fixture:      "arcBooted",
		Timeout:      5 * time.Minute,
	})
}

// AppManagement tests integration of ARC apps into the OS Settings App
// Management UI.
func AppManagement(ctx context.Context, s *testing.State) {
	const (
		testAppID   = "jdmlckamajahcocmdkmmnppjbnlloidd"
		testAppName = "ArcAppManagementTest"
	)

	cr := s.FixtValue().(*arc.PreData).Chrome
	arcDevice := s.FixtValue().(*arc.PreData).ARC
	uiAutomator := s.FixtValue().(*arc.PreData).UIDevice

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := arcDevice.Install(ctx, arc.APKPath("ArcAppManagementTest.apk")); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	appHeader := nodewith.Name(testAppName).Role(role.Heading).Ancestor(ossettings.WindowFinder)

	osSettings, err := ossettings.LaunchAtAppMgmtPage(ctx, tconn, cr, testAppID, ui.Exists(appHeader))
	if err != nil {
		s.Fatal("Failed to open OS Settings: ", err)
	}
	defer osSettings.Close(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := osSettings.LeftClick(nodewith.Name("Manage permissions"))(ctx); err != nil {
		s.Fatal("Failed to open ARC settings: ", err)
	}

	s.Run(ctx, "Verify default state", func(ctx context.Context, s *testing.State) {
		if err := verifyPermissionState(ctx, osSettings, defaultPermissionState()); err != nil {
			s.Fatal("Failed to verify default state: ", err)
		}
	})

	s.Run(ctx, "Verify storage behavior", func(ctx context.Context, s *testing.State) {
		if err := changeAndVerifyStoragePermissions(ctx, osSettings, uiAutomator); err != nil {
			s.Fatal("Failed to verify storage behavior: ", err)
		}
	})

	s.Run(ctx, "Verify non-storage behavior", func(ctx context.Context, s *testing.State) {
		if err := changeAndVerifyOtherPermissions(ctx, osSettings, uiAutomator); err != nil {
			s.Fatal("Failed to verify behavior of non-storage permissions: ", err)
		}
	})
}

func changeAndVerifyStoragePermissions(ctx context.Context, osSettings *ossettings.OSSettings, d *arcui.Device) error {
	// We will update the Storage state directly for different test cases.
	expectedStates := defaultPermissionState()

	if err := setArcPermission(ctx, d, "Photos and videos", allowRadioButtonID); err != nil {
		return err
	}
	expectedStates[4].state = "Allowed – Photos and Videos"
	if err := verifyPermissionState(ctx, osSettings, expectedStates); err != nil {
		return errors.Wrap(err, "failed to verify storage with photos & videos enabled")
	}

	if err := setArcPermission(ctx, d, "Music and audio", allowRadioButtonID); err != nil {
		return err
	}
	expectedStates[4].state = "Allowed – Audio, Photos, and Videos"
	if err := verifyPermissionState(ctx, osSettings, expectedStates); err != nil {
		return errors.Wrap(err, "failed to verify storage with all media types")
	}

	if err := setArcPermission(ctx, d, "Photos and videos", denyRadioButtonID); err != nil {
		return err
	}
	expectedStates[4].state = "Allowed – Audio"
	if err := verifyPermissionState(ctx, osSettings, expectedStates); err != nil {
		return errors.Wrap(err, "failed to verify storage with only audio enabled")
	}

	// Reset state back to the default before the next subtest.
	return setArcPermission(ctx, d, "Music and audio", denyRadioButtonID)
}

func changeAndVerifyOtherPermissions(ctx context.Context, osSettings *ossettings.OSSettings, d *arcui.Device) error {
	if err := setArcPermission(ctx, d, "Camera", askRadioButtonID); err != nil {
		return err
	}
	if err := setArcPermission(ctx, d, "Contacts", allowRadioButtonID); err != nil {
		return err
	}
	if err := setArcPermission(ctx, d, "Location", allowAlwaysRadioButtonID); err != nil {
		return err
	}
	if err := setArcPermission(ctx, d, "Microphone", allowForegroundRadioButtonID); err != nil {
		return err
	}

	expectedStates := []permissionState{
		{
			name:  "Location",
			state: "Allowed – Always",
		}, {
			name:  "Camera",
			state: "Ask every time",
		}, {
			name:  "Microphone",
			state: "Allowed – While in use",
		}, {
			name:  "Contacts",
			state: "Allowed",
		}, {
			name:  "Storage",
			state: "Denied",
		},
	}
	return verifyPermissionState(ctx, osSettings, expectedStates)
}

// verifyPermissionState checks that read-only permissions are shown on the App
// Management page in the given state. Assumes that App Management is open with
// a permission list visible.
func verifyPermissionState(ctx context.Context, osSettings *ossettings.OSSettings, permissions []permissionState) error {
	for i, permission := range permissions {
		// Search for the nth permission-name or permission-state and find a child with the right name.
		nameFinder := nodewith.Name(permission.name).Role(role.StaticText).Ancestor(nodewith.HasClass("permission-name").Nth(i))
		stateFinder := nodewith.Name(permission.state).Role(role.StaticText).Ancestor(nodewith.HasClass("permission-state").Nth(i))

		if err := uiauto.Combine("check permission state", osSettings.Exists(nameFinder), osSettings.Exists(stateFinder))(ctx); err != nil {
			return err
		}
	}

	return nil
}

// setArcPermission changes the state of a permission in the ARC permission
// controller app. Assumes that the "App permissions" settings page is open.
func setArcPermission(ctx context.Context, d *arcui.Device, name, setting string) error {
	scrollLayout := d.Object(arcui.ClassName("android.widget.ScrollView"), arcui.Scrollable(true))
	permissionView := d.Object(arcui.Text(name))

	if err := scrollLayout.ScrollTo(ctx, permissionView); err != nil {
		return errors.Wrapf(err, "failed to set %s permission", name)
	}
	if err := permissionView.Click(ctx); err != nil {
		return errors.Wrapf(err, "failed to set %s permission", name)
	}
	if err := d.Object(arcui.ID(setting)).Click(ctx); err != nil {
		return errors.Wrapf(err, "failed to set %s permission", name)
	}
	if err := d.PressKeyCode(ctx, arcui.KEYCODE_BACK, 0); err != nil {
		return errors.Wrapf(err, "failed to set %s permission", name)
	}
	return nil
}

func defaultPermissionState() []permissionState {
	return []permissionState{
		{
			name:  "Location",
			state: "Denied",
		}, {
			name:  "Camera",
			state: "Denied",
		}, {
			name:  "Microphone",
			state: "Denied",
		}, {
			name:  "Contacts",
			state: "Denied",
		}, {
			name:  "Storage",
			state: "Denied",
		},
	}
}
