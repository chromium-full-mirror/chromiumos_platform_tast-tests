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

type keystroke struct {
	key       util.LinuxKeyCode
	modifiers util.ModifiersStatus
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
		Timeout:      500 * time.Minute,
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

	kb, err := input.KeyboardWithCustomDelay(ctx, 15*time.Millisecond)
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
	noOpKeystrokes := make([]keystroke, 0)

	// Whether ESC is needed to abort possible dead-key composition or
	// modifier latch from previous iteration.
	needEsc := false

	if err := its.ClickFieldAndWaitForActive(inputField)(ctx); err != nil {
		s.Fatal("Failed to ClickFieldAndWaitForActive: ", err)
	}

	for _, modifiers := range util.ModifiersStatusCombo {
		if !needAltGrCase && modifiers.Altgr {
			continue
		}
		for _, key := range util.LinuxKeyCodes {
			if err := uiauto.NamedCombine(fmt.Sprintf("typing %s + %s", getModifierInfo(modifiers), key.KeyName),
				its.Clear(inputField),
				util.SingleKeyAction(needEsc, modifiers, key.LinuxKeyCode, kb),
			)(ctx); err != nil {
				s.Fatal("Failed to typing key: ", err)
			}

			nodeInfo, err := ui.Info(ctx, inputField.Finder())
			if err != nil {
				s.Fatal("Failed to get node info: ", err)
			}

			if nodeInfo.Value == "" {
				noOpKeystrokes = append(noOpKeystrokes, keystroke{key: key, modifiers: modifiers})
			}

			w.Write([]string{
				getModifierInCsv(csvShiftLabel, modifiers.Shift),
				getModifierInCsv(csvAltgrLabel, modifiers.Altgr),
				getModifierInCsv(csvCapsLabel, modifiers.Caps),
				key.KeyName,
				"",
				"",
				"",
				"",
				nodeInfo.Value,
				getUniCode(nodeInfo.Value)})

			// No-op looking outcome indicates either true no-op,
			// or ongoing dead-key composition or modifier latch
			// (hence ESC to abort it in next iteration).
			needEsc = (nodeInfo.Value == "")
		}
	}

	for _, keystroke1 := range noOpKeystrokes {
		for _, modifiers2 := range util.ModifiersStatusCombo {
			if !needAltGrCase && modifiers2.Altgr {
				continue
			}
			for _, key2 := range util.LinuxKeyCodes {
				if err := uiauto.NamedCombine(fmt.Sprintf("typing %s + %s, then %s + %s", getModifierInfo(keystroke1.modifiers), keystroke1.key.KeyName, getModifierInfo(modifiers2), key2.KeyName),
					its.Clear(inputField),
					util.TwoKeysAction(needEsc, keystroke1.modifiers, modifiers2, keystroke1.key.LinuxKeyCode, key2.LinuxKeyCode, kb),
				)(ctx); err != nil {
					s.Fatal("Failed to typing key: ", err)
				}

				nodeInfo, err := ui.Info(ctx, inputField.Finder())
				if err != nil {
					s.Fatal("Failed to get node info: ", err)
				}

				w.Write([]string{
					getModifierInCsv(csvShiftLabel, keystroke1.modifiers.Shift),
					getModifierInCsv(csvAltgrLabel, keystroke1.modifiers.Altgr),
					getModifierInCsv(csvCapsLabel, keystroke1.modifiers.Caps),
					keystroke1.key.KeyName,
					getModifierInCsv(csvShiftLabel, modifiers2.Shift),
					getModifierInCsv(csvAltgrLabel, modifiers2.Altgr),
					getModifierInCsv(csvCapsLabel, modifiers2.Caps),
					key2.KeyName,
					nodeInfo.Value,
					getUniCode(nodeInfo.Value)})

				// No-op looking outcome indicates either true
				// no-op, or dead-key composition or modifier
				// latch (hence ESC to abort in next iteration).
				needEsc = (nodeInfo.Value == "")
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

func getModifierInfo(modifiers util.ModifiersStatus) []string {
	var activeKeys []string
	if modifiers.Shift {
		activeKeys = append(activeKeys, "SHIFT")
	}
	if modifiers.Altgr {
		activeKeys = append(activeKeys, "ALTGR")
	}
	if modifiers.Caps {
		activeKeys = append(activeKeys, "CAPS")
	}
	return activeKeys
}
