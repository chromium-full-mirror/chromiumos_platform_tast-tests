// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/inputs/fixture"
	"chromiumos/tast/local/bundles/cros/inputs/pre"
	"chromiumos/tast/local/bundles/cros/inputs/testserver"
	"chromiumos/tast/local/bundles/cros/inputs/util"
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {

	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardEmojiSearch,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check that emoji search works well",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:input-tools", "group:mainline", "group:input-tools-upstream", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...), hwdep.SkipOnModel("kefka")),
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.DefaultInputMethod}),
		Params: []testing.Param{
			{
				Fixture: fixture.ClamshellNonVK,
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
		},
	})
}

func PhysicalKeyboardEmojiSearch(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to access keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	if err := its.InputEmojiWithEmojiPickerSearch(uc, testserver.TextAreaInputField, keyboard, "melting face", "🫠")(ctx); err != nil {
		s.Fatal("Failed to verify emoji picker: ", err)
	}
}
