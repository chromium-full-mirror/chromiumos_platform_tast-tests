// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const csvShiftLabel = "shift"
const csvAltgrLabel = "altgr"
const csvCapsLabel = "caps"

var imeID = testing.RegisterVarString(
	"inputs.imeID",
	"",
	"The target imeID string",
)

var altGr = testing.RegisterVarString(
	"inputs.altGr",
	"true",
	"The flag for adding altGr case, it will be set true by default.",
)

// keystroke struct represent the potential dead key with it's mofider keys status.
type keystroke struct {
	keycode        util.LinuxKeyCode
	modifierstatus util.ModifiersStatus
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeyboardLayout,
		Desc:         "Captures e2e physical keyboard layout behaviours",
		Contacts:     []string{"xiuwen@google.com", "tranbaoduy@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{},
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Timeout:      150 * time.Minute,
		Fixture:      fixture.ClamshellNonVK,
	})
}

func KeyboardLayout(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	inputField := testserver.TextAreaInputField

	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)
	id := imeID.Value()

	var needAltGrCase bool = true
	altGr := altGr.Value()

	if altGr == "false" {
		needAltGrCase = false
	}

	// Check if the target ime info exists or not.
	var inputMethod *ime.InputMethod

	if inputMethod, err = ime.FindInputMethodByID(id); err != nil {
		s.Fatal("Missing input method info in ime.inputMethods")
	}

	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatal("Failed to set input method: ", err)
	}

	filename := fmt.Sprintf("%s.csv", inputMethod.ID)
	path := filepath.Join(s.OutDir(), filename)
	file, err := os.Create(path)
	if err != nil {
		s.Fatalf("Failed to create file %s: %v", filename, err)
	}
	defer file.Close()

	w := csv.NewWriter(file)
	w.Write([]string{"shift-1", "altgr-1", "caps-1", "location-1", "shift-2", "altgr-2", "caps-2", "location-2", "string", "unicode"})
	noOpKey1ModifierList := make([]keystroke, 0)

	for _, key1ModifiersStatus := range util.ModifiersStatusCombo {
		if !needAltGrCase && key1ModifiersStatus.Altgr {
			continue
		}
		for _, key := range util.LinuxKeyCodes {
			if err := uiauto.NamedCombine(fmt.Sprintf("typing %s + %s", getModifierInfo(key1ModifiersStatus), key.KeyName),
				its.Clear(inputField),
				its.ClickFieldAndWaitForActive(inputField),
				util.SingleKeyAction(key1ModifiersStatus, key.LinuxKeyCode, kb),
			)(ctx); err != nil {
				s.Fatal("Failed to typing key: ", err)
			}

			nodeInfo, err := ui.Info(ctx, inputField.Finder())
			if err != nil {
				s.Fatal("Failed to get node info: ", err)
			}

			if nodeInfo.Value == "" {
				noOpKey1ModifierList = append(noOpKey1ModifierList, keystroke{keycode: key, modifierstatus: key1ModifiersStatus})
			}

			w.Write([]string{
				getModifierInCsv(csvShiftLabel, key1ModifiersStatus.Shift),
				getModifierInCsv(csvAltgrLabel, key1ModifiersStatus.Altgr),
				getModifierInCsv(csvCapsLabel, key1ModifiersStatus.Caps),
				key.KeyName,
				"",
				"",
				"",
				"",
				nodeInfo.Value,
				getUniCode(nodeInfo.Value)})
		}
	}

	for _, key1keystroke := range noOpKey1ModifierList {
		for _, key2Modifiers := range util.ModifiersStatusCombo {
			if !needAltGrCase && key2Modifiers.Altgr {
				continue
			}
			for _, key := range util.LinuxKeyCodes {
				if err := uiauto.NamedCombine(fmt.Sprintf("typing %s + %s, then %s + %s", getModifierInfo(key1keystroke.modifierstatus), key1keystroke.keycode.KeyName, getModifierInfo(key2Modifiers), key.KeyName),
					its.Clear(inputField),
					its.ClickFieldAndWaitForActive(inputField),
					util.TwoKeysAction(key1keystroke.modifierstatus, key2Modifiers, key1keystroke.keycode.LinuxKeyCode, key.LinuxKeyCode, kb),
				)(ctx); err != nil {
					s.Fatal("Failed to typing key: ", err)
				}

				nodeInfo, err := ui.Info(ctx, inputField.Finder())
				if err != nil {
					s.Fatal("Failed to get node info: ", err)
				}

				w.Write([]string{
					getModifierInCsv(csvShiftLabel, key1keystroke.modifierstatus.Shift),
					getModifierInCsv(csvAltgrLabel, key1keystroke.modifierstatus.Altgr),
					getModifierInCsv(csvCapsLabel, key1keystroke.modifierstatus.Caps),
					key1keystroke.keycode.KeyName,
					getModifierInCsv(csvShiftLabel, key2Modifiers.Shift),
					getModifierInCsv(csvAltgrLabel, key2Modifiers.Altgr),
					getModifierInCsv(csvCapsLabel, key2Modifiers.Caps),
					key.KeyName,
					nodeInfo.Value,
					getUniCode(nodeInfo.Value)})
			}
		}
	}

	w.Flush()
}

func getUniCode(str string) string {
	if str == "" {
		return "(n/a)"
	}

	var unicodeArr []string

	for _, runeValue := range str {
		unicodeArr = append(unicodeArr, fmt.Sprintf("%U", runeValue))
	}
	return strings.Join(unicodeArr, ",")
}

func getModifierInCsv(modifier string, modifierStatus bool) string {
	if modifierStatus {
		return modifier
	}
	return ""
}

func getModifierInfo(status util.ModifiersStatus) []string {
	var activeKeys []string
	if status.Shift {
		activeKeys = append(activeKeys, "SHIFT")
	}
	if status.Altgr {
		activeKeys = append(activeKeys, "ALTGR")
	}
	if status.Caps {
		activeKeys = append(activeKeys, "CAPS")
	}
	return activeKeys
}
