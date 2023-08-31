// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/feedbackapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type settingsSearchTestParams struct {
	arc        bool
	searchtype settingsSearchType
}

type settingsSearchType int

const (
	normalOptions settingsSearchType = iota
	arcOptions
	optionsAndSubpage
	optionsAndDeepLinking
	guestMode
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchSections,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Search with keywords and verify the related results from OS Settings",
		Contacts: []string{
			"cros-settings@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Settings
		BugComponent: "b:1246072",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "arc"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-8cd33196-24d6-4541-aec7-3b21bd827990",
		}},
		Params: []testing.Param{
			{
				Name: "normal_options",
				Val: settingsSearchTestParams{
					arc:        false,
					searchtype: normalOptions,
				},
				Fixture: "chromeLoggedInWithOsSettingsSearchFeedback",
			}, {
				Name: "arc_options",
				Val: settingsSearchTestParams{
					arc:        true,
					searchtype: arcOptions,
				},
				Fixture: "arcBootedWithoutUIAutomator",
			}, {
				Name: "options_and_subpage",
				Val: settingsSearchTestParams{
					arc:        false,
					searchtype: optionsAndSubpage,
				},
				Fixture: "chromeLoggedIn",
			}, {
				Name: "options_and_deep_linking",
				Val: settingsSearchTestParams{
					arc:        false,
					searchtype: optionsAndDeepLinking,
				},
				Fixture: "chromeLoggedIn",
			}, {
				Name: "guest_mode",
				Val: settingsSearchTestParams{
					arc:        false,
					searchtype: guestMode,
				},
				Fixture: "chromeLoggedInGuestWithOsSettingsSearchFeedback",
			},
		},
	})
}

type settingsSearchDetail struct {
	keyword string

	expectedResult     string
	expectedMismatch   bool
	expectedResultRole role.Role

	deepLinkingSection string
	subpageLabel       string
}

type settingsOSSearch struct {
	keyword  string
	ui       *uiauto.Context
	settings *ossettings.OSSettings
	tconn    *chrome.TestConn
}

func searchDetail(st settingsSearchType) []settingsSearchDetail {
	switch st {
	case normalOptions:
		return []settingsSearchDetail{
			{
				keyword:            "turnoff",
				expectedResult:     `Turn off (Bluetooth|Wi\-Fi|networks)`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "plays",
				expectedResult:     `Display`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "photo",
				expectedResult:     `.*photos.*`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "wall",
				expectedResult:     `Change wallpaper`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "Drive",
				expectedResult:     `.*Drive.*`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "Input",
				expectedResult:     `Inputs`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "acces", // An incomplete keyword to search.
				expectedResult:     `Accessibility`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "keyboa", // An incomplete keyword to search.
				expectedResult:     `Keyboard`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "langu", // An incomplete keyword to search.
				expectedResult:     `Languages`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "printters", // A typo keyword to search.
				expectedResult:     `Printers`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "gpu",
				expectedResult:     `No search results found`,
				expectedResultRole: role.StaticText,
				expectedMismatch:   true,
			},
		}
	case arcOptions:
		return []settingsSearchDetail{
			{
				keyword:            "android",
				expectedResult:     `Android preferences`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "android",
				expectedResult:     `Google Play Store`,
				expectedResultRole: role.GenericContainer,
			},
		}
	case optionsAndSubpage:
		return []settingsSearchDetail{
			{
				keyword:            "wifi",
				expectedResult:     `Wi-Fi (networks|Sync)`,
				expectedResultRole: role.GenericContainer,
				subpageLabel:       "Known networks",
			},
		}
	case optionsAndDeepLinking:
		return []settingsSearchDetail{
			{
				keyword:            "onscreen",
				expectedResult:     `On-screen keyboard`,
				expectedResultRole: role.GenericContainer,
				deepLinkingSection: "On-screen keyboard",
			}, {
				keyword:            "Night light",
				expectedResult:     `Night Light`,
				expectedResultRole: role.GenericContainer,
				deepLinkingSection: `Night Light`,
			}, {
				keyword:            "high contrast",
				expectedResult:     `High contrast mode`,
				expectedResultRole: role.GenericContainer,
				deepLinkingSection: `Color inversion`,
			},
		}
	case guestMode:
		return []settingsSearchDetail{
			{
				keyword:            "turnoff",
				expectedResult:     `Turn off (Bluetooth|Wi\-Fi|networks)`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "photo",
				expectedResult:     `No search results found`,
				expectedResultRole: role.StaticText,
				expectedMismatch:   true,
			}, {
				keyword:            "wallpaper",
				expectedResult:     `Change wallpaper`,
				expectedResultRole: role.StaticText,
			}, {
				keyword:            "Drive",
				expectedResult:     `.*Drive.*`,
				expectedResultRole: role.GenericContainer,
			}, {
				keyword:            "Input",
				expectedResult:     `Inputs`,
				expectedResultRole: role.GenericContainer,
			},
		}
	default:
		return []settingsSearchDetail{}
	}
}

// SearchSections searches specified settings and checks corresponding section.
func SearchSections(ctx context.Context, s *testing.State) {
	params, ok := s.Param().(settingsSearchTestParams)
	if !ok {
		s.Fatal("Failed to get test parameters: ")
	}

	var cr *chrome.Chrome
	if params.arc {
		cr = s.FixtValue().(*arc.PreData).Chrome
	} else {
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	osSettings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS Settings: ", err)
	}
	defer func() {
		faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")
		osSettings.Close(cleanupCtx)
	}()

	ui := uiauto.New(tconn)

	// Verify that cursor is focus in the search field.
	if err := ui.WaitUntilExists(ossettings.SearchBoxFinder.Focused())(ctx); err != nil {
		s.Fatal("Failed to wait for cursor be focused on the search field in OS settings: ", err)
	}

	for _, search := range searchDetail(params.searchtype) {
		result, err := searchAndCheck(ctx, osSettings, kb, search)
		if err != nil {
			s.Fatal("Failed to search with keyword: ", err)
		}

		if search.expectedMismatch {
			resource := &settingsOSSearch{search.keyword, ui, osSettings, tconn}
			if err := sendSearchFeedback(ctx, resource, s); err != nil {
				s.Fatal("Failed to send feedback for no search results: ", err)
			}
		}

		if search.subpageLabel != "" {
			if err := ui.WithTimeout(30*time.Second).RetryUntil(
				mouse.Click(tconn, result.Location.CenterPoint(), mouse.LeftButton),
				osSettings.WithTimeout(10*time.Second).WaitUntilExists(nodewith.Name(search.subpageLabel)),
			)(ctx); err != nil {
				s.Fatal("Failed to enter the corresponding subpage for choosen option: ", err)
			}
		}

		// Click and display results with focus section of deep linking.
		if search.deepLinkingSection != "" {
			if err := ui.WithTimeout(30*time.Second).RetryUntil(
				mouse.Click(tconn, result.Location.CenterPoint(), mouse.LeftButton),
				osSettings.WithTimeout(10*time.Second).WaitUntilExists(nodewith.NameRegex(regexp.MustCompile(search.deepLinkingSection)).Role(role.ToggleButton).Onscreen()),
			)(ctx); err != nil {
				s.Fatal("Failed to enter the exact section by deep linking: ", err)
			}
		}

		if err := osSettings.ClearSearch()(ctx); err != nil {
			s.Fatal("Failed to clear search: ", err)
		}
	}
}

func searchAndCheck(ctx context.Context, osSettings *ossettings.OSSettings, kb *input.KeyboardEventWriter,
	detail settingsSearchDetail) (*uiauto.NodeInfo, error) {

	testing.ContextLogf(ctx, "Search for %q", detail.keyword)
	infos, mismatched, err := osSettings.SearchWithKeyword(ctx, kb, detail.keyword)
	if err != nil {
		return nil, err
	}

	// Verify mismatch.
	if detail.expectedMismatch != mismatched {
		return nil, errors.Errorf("unexpected search result, want: [mismatch: %t], got: [mismatch: %t]", detail.expectedMismatch, mismatched)
	}

	if len(infos) == 0 {
		// The keyword did not return a search result (ie. there is a mismatch).
		// We can return as we do not need to verify the resulting string any further.
		// The check for whether the correct node with the name `No search results found` is
		// returned is done in function osSettings.SearchWithKeyword(ctx, kb, detail.keyword),
		// so there is no need for further verification.
		return nil, nil
	} else if len(infos) > 5 || len(infos) < 1 {
		// The results should show a minimum of 1 or maximum of 5 results.
		return nil, errors.Errorf("unexpected result count, want: [1,5], got: %d", len(infos))
	}

	// Verify result.
	rExpected := regexp.MustCompile(detail.expectedResult)
	for idx, info := range infos {
		if rExpected.MatchString(info.Name) {
			testing.ContextLogf(ctx, "Found: %q", infos[idx].Name)
			return &infos[idx], nil
		}
	}

	return nil, errors.Errorf("no match results found, the first result is %q", infos[0].Name)
}

func sendSearchFeedback(ctx context.Context, resource *settingsOSSearch, s *testing.State) error {
	testing.ContextLogf(ctx, "Send search feedback when there are no search results for query: %q", resource.keyword)

	if err := resource.settings.LeftClick(ossettings.SearchFeedbackButton)(ctx); err != nil {
		return err
	}

	if err := feedbackapp.VerifyFeedbackAppIsLaunched(ctx, resource.tconn, resource.ui); err != nil {
		return err
	}

	feedbackDescriptionPlaceholderNodeName := fmt.Sprintf("#Settings No search results returned for '%v'", resource.keyword)
	feedbackAncestor := nodewith.Name("Send feedback").Role(role.Heading)

	if err := uiauto.Combine("check search feedback app with pre-populated description",
		resource.ui.Exists(nodewith.Name("Send feedback").Role(role.StaticText).Ancestor(feedbackAncestor)),
		resource.ui.Exists(nodewith.Name("Description").Role(role.InlineTextBox)),
		resource.ui.Exists(nodewith.Name(feedbackDescriptionPlaceholderNodeName).Role(role.StaticText)),
	)(ctx); err != nil {
		return err
	}

	sysInfoMetricsCheckboxAncestor := nodewith.Name("Send system & app info and metrics").Role(role.GenericContainer)
	sysInfoMetricsCheckbox := nodewith.Role(role.CheckBox).Ancestor(sysInfoMetricsCheckboxAncestor)

	if err := uiauto.Combine("search feedback's checkbox `send sys info and metrics` is unchecked, send feedback and close the feedback app",
		resource.ui.LeftClick(nodewith.Name("Continue").Role(role.Button)),
		resource.ui.WaitUntilExists(sysInfoMetricsCheckbox.Attribute("checked", "false")),
		resource.ui.LeftClick(nodewith.Name("Send").Role(role.Button)),
		resource.ui.LeftClick(nodewith.Name("Done").Role(role.Button)),
		resource.ui.WaitUntilGone(feedbackAncestor),
	)(ctx); err != nil {
		return err
	}

	return nil
}
