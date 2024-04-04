// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AllSections,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Open OS Settings and check main sections are displayed properly",
		Contacts: []string{
			"cros-settings@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// OS > Systems > Settings
		BugComponent: "b:1246072",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

// AllSections goes through all main sections of OS settings.
func AllSections(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	osSettings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS Settings: ", err)
	}
	defer osSettings.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	if err := osSettings.WaitForSearchBox()(ctx); err != nil {
		s.Fatal("Failed to wait for OS-settings is ready to use: ", err)
	}

	sections := ossettings.CommonSections()
	for sectionName, sectionFinder := range sections {
		section := sectionTest{
			name:   sectionName,
			finder: sectionFinder,
		}
		if sectionFinder == ossettings.Internet {
			section.subSectionToExpand = "Add network connection"
		}
		if sectionFinder == ossettings.SystemPreferences {
			section.subSettingToToggle = "Use 24-hour clock"
		}
		if err := checkSection(ctx, cr, osSettings, section); err != nil {
			s.Fatalf("Failed to check section %s: %v", sectionName, err)
		}
	}
}

type sectionTest struct {
	name               string
	finder             *nodewith.Finder
	subSectionToExpand string
	subSettingToToggle string
}

// checkSection checks section within the ossettings, and verifies specified subsection or sub-setting is properly displayed by expands/toggles on it.
func checkSection(ctx context.Context, cr *chrome.Chrome, osSettings *ossettings.OSSettings, section sectionTest) error {
	if err := uiauto.Combine(fmt.Sprintf("look for section: %q", section.name),
		ensureVisible(osSettings, section.finder),
		osSettings.WaitUntilExists(section.finder.Onscreen()),
	)(ctx); err != nil {
		return err
	}

	if section.subSectionToExpand != "" {
		node := nodewith.Name(section.subSectionToExpand).Role(role.Button)
		if err := osSettings.LeftClick(section.finder)(ctx); err != nil {
			return errors.Wrapf(err, "failed to navigate to section %q", section.name)
		}
		if err := expandSubSection(osSettings, node, true)(ctx); err != nil {
			return errors.Wrap(err, "failed to expand sub section")
		}
	}

	if section.subSettingToToggle != "" {
		if err := osSettings.LeftClick(section.finder)(ctx); err != nil {
			return errors.Wrapf(err, "failed to navigate to section %q", section.name)
		}
		if err := toggleSetting(cr, osSettings, section.subSettingToToggle)(ctx); err != nil {
			return errors.Wrap(err, "failed to toggle setting")
		}
	}

	return nil
}

func expandSubSection(osSettings *ossettings.OSSettings, node *nodewith.Finder, expected bool) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("expand sub section: %s", node.Pretty()),
		osSettings.WaitForLocation(node),
		osSettings.EnsureFocused(node),
		osSettings.LeftClick(node.State(state.Expanded, !expected)),
		osSettings.WaitUntilExists(node.State(state.Expanded, expected)),
	)
}

func toggleSetting(cr *chrome.Chrome, osSettings *ossettings.OSSettings, name string) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("toggle setting: %s", name),
		osSettings.SetToggleOption(cr, name, true),
		osSettings.SetToggleOption(cr, name, false),
	)
}

func ensureVisible(osSettings *ossettings.OSSettings, node *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		if found, err := osSettings.IsNodeFound(ctx, nodewith.Role(role.Navigation).First()); err != nil {
			return errors.Wrap(err, "failed to try to find node")
		} else if !found {
			// The main menu might be collapsed depending on window size, expand the main menu to ensure the input node is visible.
			if err = osSettings.LeftClick(ossettings.MenuButton)(ctx); err != nil {
				return errors.Wrap(err, "failed to click menu button")
			}
		}

		info, err := osSettings.Info(ctx, node)
		if err != nil {
			return err
		}
		if !info.State[state.Offscreen] {
			return nil
		}
		return osSettings.MakeVisible(node)(ctx)
	}
}
