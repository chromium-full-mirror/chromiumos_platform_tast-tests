// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/files"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/restrictionlevel"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type fileUSBCopyTestParams struct {
	restriction restrictionlevel.RestrictionLevel
	browserType browser.Type
}

// filesUSBCopyBlockPolicy returns a DLP policy that blocks copying file to USB.
func filesUSBCopyBlockPolicy() []policy.Policy {
	return []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Disable copying of confidential file to USB",
				Description: "User should not be able to copy confidential file to USB drive",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						"*",
					},
				},
				Destinations: &policy.DataLeakPreventionRulesListValueDestinations{
					Components: []string{
						"USB",
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "FILES",
						Level: "BLOCK",
					},
				},
			},
		},
	},
	}
}

// filesUSBCopyWarnPolicy returns a DLP policy that warns when copying file to USB.
func filesUSBCopyWarnPolicy() []policy.Policy {
	return []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Warn before copying of confidential file to USB",
				Description: "User should be warned before copy confidential file to USB drive",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						"*",
					},
				},
				Destinations: &policy.DataLeakPreventionRulesListValueDestinations{
					Components: []string{
						"USB",
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "FILES",
						Level: "WARN",
					},
				},
			},
		},
	},
	}
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListFilesUSB,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with file copy to USB restriction",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"poromov@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > DLP (Data Loss Prevention)
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Block users from sharing confidential information (usb): COM_DATPROT_CUJ3_TASK1_WF1.
				Value: "screenplay-8372a021-869b-466b-a031-0199b899874c",
			}, {
				Key: "feature_id",
				// Block users from sharing confidential information within company (usb): COM_DATPROT_CUJ4_TASK1_WF1.
				Value: "screenplay-b6dab588-8e02-4914-a8d5-608395e75d4b",
			}, {
				Key: "feature_id",
				// Warn users from sharing confidential information (usb): COM_DATPROT_CUJ3_TASK2_WF1.
				Value: "screenplay-6d903887-d562-4238-9a8c-99bef2351d72",
			}, {
				Key: "feature_id",
				// Warn users from sharing confidential information within company (usb): COM_DATPROT_CUJ4_TASK2_WF1.
				Value: "screenplay-387a9ca9-a622-43df-b0a1-8f95fa1c78c8",
			}},
		Params: []testing.Param{
			{
				Name: "ash_allowed",
				ExtraAttr: []string{
					"group:golden_tier",
					"group:medium_low_tier",
					"group:hardware",
					"group:complementary",
				},
				Fixture: fixture.ChromePolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeAsh,
					restriction: restrictionlevel.Allowed,
				},
			}, {
				Name: "lacros_allowed",
				ExtraAttr: []string{
					"group:golden_tier",
					"group:medium_low_tier",
					"group:hardware",
					"group:complementary",
				},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosPolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeLacros,
					restriction: restrictionlevel.Allowed,
				},
			}, {
				Name:      "ash_blocked",
				ExtraAttr: []string{"group:mainline"},
				Fixture:   fixture.ChromePolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeAsh,
					restriction: restrictionlevel.Blocked,
				},
			}, {
				Name:              "lacros_blocked",
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosPolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeLacros,
					restriction: restrictionlevel.Blocked,
				},
			}, {
				Name:      "ash_warn_proceeded",
				ExtraAttr: []string{"group:mainline", "informational"},
				Fixture:   fixture.ChromePolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeAsh,
					restriction: restrictionlevel.WarnProceeded,
				},
			}, {
				Name:              "lacros_warn_proceeded",
				ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosPolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeLacros,
					restriction: restrictionlevel.WarnProceeded,
				},
			}, {
				Name: "ash_warn_cancelled",
				ExtraAttr: []string{
					"group:golden_tier",
					"group:medium_low_tier",
					"group:hardware",
					"group:complementary",
				},
				Fixture: fixture.ChromePolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeAsh,
					restriction: restrictionlevel.WarnCancelled,
				},
			}, {
				Name: "lacros_warn_cancelled",
				ExtraAttr: []string{
					"group:golden_tier",
					"group:medium_low_tier",
					"group:hardware",
					"group:complementary",
				},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosPolicyLoggedInFilesUXEnabled,
				Val: fileUSBCopyTestParams{
					browserType: browser.TypeLacros,
					restriction: restrictionlevel.WarnCancelled,
				},
			},
		},
		Data: []string{
			"download.html",
			"data.txt",
		},
	})
}

func DataLeakPreventionRulesListFilesUSB(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	appliedRestriction := s.Param().(fileUSBCopyTestParams).restriction

	// Update the policy.
	var policy []policy.Policy
	switch appliedRestriction {
	case restrictionlevel.Blocked:
		policy = filesUSBCopyBlockPolicy()
	case restrictionlevel.WarnCancelled, restrictionlevel.WarnProceeded:
		policy = filesUSBCopyWarnPolicy()
	}
	if policy != nil {
		if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, policy); err != nil {
			s.Fatal("Failed to serve and verify: ", err)
		}
	}

	if err := files.ClearDownloads(ctx, cr); err != nil {
		s.Fatal("Failed to clear Downloads directory: ", err)
	}

	tconnAsh, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconnAsh); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconnAsh)

	// Create Browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(fileUSBCopyTestParams).browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer func(ctx context.Context) {
		if err := closeBrowser(ctx); errors.Is(err, lacros.ErrAlreadyStoppedBeforeClose) {
			// The Lacros browser is not closed in other places in the test.
			s.Error("The Lacros browser probably crashed: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	tconnBrowser, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser's test API: ", err)
	}

	// The browsers sometimes restore some tabs, so we manually close all unneeded tabs.
	closeTabsFunc := browser.CloseAllTabs
	if s.Param().(fileUSBCopyTestParams).browserType == browser.TypeLacros {
		// For lacros-Chrome, it should leave a new tab to keep the Chrome process alive.
		closeTabsFunc = browser.ReplaceAllTabsWithSingleNewTab
	}
	if err := closeTabsFunc(ctx, tconnBrowser); err != nil {
		s.Fatal("Failed to close all unneeded tabs: ", err)
	}
	defer closeTabsFunc(cleanupCtx, tconnBrowser)

	// Close all prior notifications.
	if err := ash.CloseNotifications(ctx, tconnAsh); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	if err := files.DownloadFile(ctx, tconnAsh, br, s.DataFileSystem()); err != nil {
		s.Fatal("Failed to download file: ", err)
	}

	// Open the Files app to cleanup USB devices. Closed at relaunch or Chrome reset.
	filesApp, err := filesapp.Launch(ctx, tconnAsh)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}

	// Eject all USB devices if some are still around.
	if err := filesApp.EjectAll()(ctx); err != nil {
		s.Fatal("Failed to eject: ", err)
	}

	// Create the virtual USB device.
	if err := setupVirtualUSBDevice(ctx); err != nil {
		s.Fatal("Failed to setup virtual USB device: ", err)
	}
	defer cleanupVirtualUSBDevice(ctx)

	// Re-open the Files app to retrieve the new USB drive, format it and try to copy the file.
	filesApp, err = filesapp.Relaunch(ctx, tconnAsh, filesApp)
	if err != nil {
		s.Fatal("Failed to relaunch the Files App: ", err)
	}
	defer filesApp.Close(cleanupCtx)

	if err := filesApp.OpenUSBDrive()(ctx); err != nil {
		s.Fatal("Failed to open unformatted USB drive: ", err)
	}
	if err := filesApp.FormatDevice()(ctx); err != nil {
		s.Fatal("Failed to format USB drive: ", err)
	}
	s.Log("USB drive is formatted")
	if err := filesApp.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads folder: ", err)
	}
	if err := filesApp.CopyFileToClipboard(files.DlFileName)(ctx); err != nil {
		s.Fatal("Failed to copy downloaded file to the clipboard: ", err)
	}
	if err := filesApp.OpenUSBDriveWithName("UNTITLED")(ctx); err != nil {
		s.Fatal("Failed to open formatted USB drive: ", err)
	}
	if err := filesApp.FileExists(files.DlFileName)(ctx); err == nil {
		if err := filesApp.DeleteFileOrFolder(keyboard, files.DlFileName)(ctx); err != nil {
			s.Error("Failed to delete file before pasting: ", err)
		}
	}
	if err := filesApp.PasteFileFromClipboard(keyboard)(ctx); err != nil {
		s.Fatal("Failed to paste copied file: ", err)
	}

	switch appliedRestriction {
	case restrictionlevel.WarnProceeded:
		ui := uiauto.New(tconnAsh)
		if err := files.AcceptWarningAndVerify(ctx, ui, tconnAsh, files.DlFileName); err != nil {
			s.Fatal("Failed to proceed the warning: ", err)
		}
	case restrictionlevel.WarnCancelled:
		ui := uiauto.New(tconnAsh)
		if err := files.CancelWarningAndVerify(ctx, ui, tconnAsh, files.DlFileName); err != nil {
			s.Fatal("Failed to cancel the warning: ", err)
		}
	}

	// Eject all USB devices before force-unmounting.
	if err := filesApp.EjectAll()(ctx); err != nil {
		s.Fatal("Failed to eject: ", err)
	}
}

// Constants to create a virtual USB drive.
const (
	usbVID          = "dddd"
	usbPID          = "ffff"
	usbManufacturer = "Tast"
	usbProduct      = "VirtualTestUSBDrive"
	usbSerialNumber = "12345"
)

// setupVirtualUSBDevice creates a virtual USB drive.
func setupVirtualUSBDevice(ctx context.Context) error {
	// The file could be absent, so ignoring the error.
	os.Remove("/tmp/backing_file")

	if err := testexec.CommandContext(ctx, "modprobe",
		"dummy_hcd").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "fail to load dummy_hcd module")
	}

	if err := testexec.CommandContext(ctx, "dd", "bs=1024", "count=64", "if=/dev/zero",
		"of=/tmp/backing_file").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "fail to create temporary backing_file")
	}

	if err := testexec.CommandContext(ctx, "modprobe", "g_mass_storage",
		"file=/tmp/backing_file", "idVendor=0x"+usbVID, "idProduct=0x"+usbPID,
		"iManufacturer="+usbManufacturer, "iProduct="+usbProduct,
		"iSerialNumber="+usbSerialNumber).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "fail to create virtual USB storage")
	}

	return nil
}

// cleanupVirtualUSBDevice removes previously created virtual USB drive and the backing file.
func cleanupVirtualUSBDevice(ctx context.Context) {
	testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "-r").Run()

	// The file could be absent, so ignoring the error.
	os.Remove("/tmp/backing_file")
}
