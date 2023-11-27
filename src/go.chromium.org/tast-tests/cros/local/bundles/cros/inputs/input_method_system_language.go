// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/util"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// testParameters contains all the data needed to run a single test iteration.
type testParameters struct {
	regionCode             string
	defaultInputMethodID   string
	defaultInputMethodName string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputMethodSystemLanguage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launching ChromeOS in different languages defaults input method",
		Contacts: []string{
			"essential-inputs-gardener-oncall@google.com",
			"essential-inputs-team@google.com",
		},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		Params: []testing.Param{
			{
				Name:              "es",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
				Val: testParameters{
					regionCode:           "es",
					defaultInputMethodID: ime.SpanishSpain.ID,
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.SpanishSpain}),
			}, {
				Name:              "es_informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
				Val: testParameters{
					regionCode:           "es",
					defaultInputMethodID: ime.SpanishSpain.ID,
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.SpanishSpain}),
			}, {
				Name:              "fr",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
				Val: testParameters{
					regionCode:           "fr",
					defaultInputMethodID: ime.FrenchFrance.ID,
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			}, {
				Name:              "fr_informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
				Val: testParameters{
					regionCode:           "fr",
					defaultInputMethodID: ime.FrenchFrance.ID,
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			}, {
				Name:              "jp",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
				Val: testParameters{
					regionCode:           "jp",
					defaultInputMethodID: ime.AlphanumericWithJapaneseKeyboard.ID,
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.AlphanumericWithJapaneseKeyboard}),
			}, {
				Name:              "jp_informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
				Val: testParameters{
					regionCode:           "jp",
					defaultInputMethodID: ime.AlphanumericWithJapaneseKeyboard.ID,
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.AlphanumericWithJapaneseKeyboard}),
			},
		},
	})
}

func InputMethodSystemLanguage(ctx context.Context, s *testing.State) {
	regionCode := s.Param().(testParameters).regionCode
	defaultInputMethodID := ime.ChromeIMEPrefix + s.Param().(testParameters).defaultInputMethodID

	cr, err := chrome.New(ctx, chrome.Region(regionCode))
	if err != nil {
		s.Fatalf("Failed to start Chrome in region %s: %v", regionCode, err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	uc, err := inputactions.NewInputsUserContext(ctx, s, cr, tconn, nil)
	if err != nil {
		s.Fatal("Failed to initiate inputs user context: ", err)
	}

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	action := func(ctx context.Context) error {
		// Verify default input method
		if currentInputMethodID, err := ime.CurrentInputMethod(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to get current input method")
		} else if currentInputMethodID != defaultInputMethodID {
			return errors.Wrapf(err, "unexpected default input method in country %s. got %s; want %s", regionCode, currentInputMethodID, defaultInputMethodID)
		}
		return nil
	}
	if err := uiauto.UserAction("Default input method in region",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureIMEManagement,
				useractions.AttributeDeviceRegion: regionCode,
			},
		},
	)(ctx); err != nil {
		s.Fatalf("Failed to validate default input method in region %q: %v", regionCode, err)
	}
}
