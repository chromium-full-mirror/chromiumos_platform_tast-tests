// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast/core/ctxutil"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const dlcImagePath = "/run/imageloader/"

func init() {
	testing.AddTest(&testing.Test{
		Func:         LanguagepacksHandwritingDlcDownload,
		Desc:         "Test language packs handwriting dlc mounting",
		Contacts:     []string{"cros-borders-eng@google.com", "xiuwen@google.com"},
		BugComponent: "b:934840",
		Attr:         []string{"group:language_packs_hw_recognition_dlc_download_daily"},
		LacrosStatus: testing.LacrosVariantExists,
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      10 * time.Minute,
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Fixture:      fixture.ClamshellVKWithHandWritingLegacyRecognitionOn,
	})
}

func LanguagepacksHandwritingDlcDownload(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputField := testserver.TextAreaInputField
	vkbCtx := vkb.NewContext(cr, tconn)

	languageTests := map[string]ime.InputMethod{
		// Add more target languages when you need.
		// ref: https://source.corp.google.com/piper///depot/google3/chrome/inputs/common/input_method_config.textproto
		// ime mapping: src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/chrome/ime/input_method.go
		"ar":    ime.Arabic,
		"es":    ime.SpanishSpain,
		"fr":    ime.FrenchFrance,
		"it":    ime.Italian,
		"ja":    ime.Japanese,
		"ko":    ime.Korean,
		"nl":    ime.DutchNetherlands,
		"pt":    ime.Portuguese,
		"ta":    ime.Tamil,
		"vi":    ime.VietnameseTelex,
		"zh":    ime.ChinesePinyin,
		"zh-HK": ime.Cantonese,
	}

	for language, ime := range languageTests {
		s.Run(ctx, language, func(ctx context.Context, s *testing.State) {
			inputMethod := ime
			dlcModuleName := fmt.Sprintf("handwriting-%s", language)

			if exist, _ := dlcFolderExist(ctx, s, dlcModuleName); exist {
				dlcserviceUninstall(ctx, s, dlcModuleName)
			}

			if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
				s.Fatal("Failed to set input method: ", err)
			}
			uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

			// Trigger vk and turn to handwriting mode.
			if err := its.ClickFieldUntilVKShown(inputField)(ctx); err != nil {
				s.Fatal("Failed to trigger vk: ", err)
			}

			if _, err := vkbCtx.SwitchToHandwriting(ctx); err != nil {
				s.Fatal("Failed to switch to handwriting mode: ", err)
			}

			if err := uiauto.Combine("switch back to keyboard mode and hide VK",
				vkbCtx.SwitchToKeyboard(),
				vkbCtx.TapHideVitrualKeyboardButton(),
			)(ctx); err != nil {
				s.Log("Failed to switch back to keyboard mode and hide VK: ", err)
			}

			checkLocalDlcFiles(ctx, s, dlcModuleName)
			dlcserviceUninstall(ctx, s, dlcModuleName)
		})
	}
}

// dlcFolderExist check folder exist or not.
// It calls a bash cmd "test -d {folderName};echo $?", cmd return 0 if folder exist else return 1.
func dlcFolderExist(ctx context.Context, s *testing.State, folderName string) (bool, error) {
	bashCmd := fmt.Sprintf("test -d /run/imageloader/%s;echo $?", folderName)
	output, err := testexec.CommandContext(ctx, "bash", "-c", bashCmd).Output()
	if err != nil {
		s.Logf("Fail to check sub folder %s", err)
		return false, err
	}

	if strings.TrimSpace(string(output)) == "1" {
		return false, nil
	}
	return true, nil
}

func checkLocalDlcFiles(ctx context.Context, s *testing.State, dlcModuleName string) {
	output, err := testexec.CommandContext(ctx, "find", dlcImagePath+dlcModuleName, "-maxdepth", "0", "-empty").Output()
	if err != nil {
		s.Logf("Fail to check sub folder %s", err)
	}

	if len(output) != 0 {
		s.Fatalf("Fail to install dlc %s", dlcModuleName)
	}
}

func dlcserviceUninstall(ctx context.Context, s *testing.State, dlcModuleName string) {
	_, err := testexec.CommandContext(ctx, "dlcservice_util", "--uninstall", fmt.Sprintf("--id=%s", dlcModuleName)).Output()
	if err != nil {
		s.Logf("Fail to uninstall dlc: %s", err)
	}
}
