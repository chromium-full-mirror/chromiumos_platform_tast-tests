// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/url"
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

	"golang.org/x/exp/slices"
)

const csvShiftLabel = "shift"
const csvAltgrLabel = "altgr"
const csvCapsLabel = "caps"

var supportedHardwareLayoutTypes = []string{"ISO", "ANSI", "ABNT", "JIS"}

var imeIDArg = testing.RegisterVarString(
	"inputs.imeID",
	"",
	"CrOS input method ID",
)

var hardwareLayoutTypeArg = testing.RegisterVarString(
	"inputs.hardwareLayoutType",
	"ISO",
	"Target hardware layout type: ISO (default), ANSI, ABNT, or JIS.",
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

	kb, err := input.KeyboardWithCustomDelay(ctx, 30*time.Millisecond)
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

	var hardwareLayoutType = hardwareLayoutTypeArg.Value()
	if !slices.Contains(supportedHardwareLayoutTypes, hardwareLayoutType) {
		s.Fatalf("Unknown hardwareLayoutType: %s", hardwareLayoutType)
	}

	// Check if the target ime info exists or not.
	var inputMethod *ime.InputMethod

	if inputMethod, err = ime.FindInputMethodByID(imeIDArg.Value()); err != nil {
		s.Fatal("Missing input method info in ime.inputMethods")
	}

	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatal("Failed to set input method: ", err)
	}

	filename := fmt.Sprintf("%s__%s.csv", url.QueryEscape(inputMethod.ID), hardwareLayoutType)
	path := filepath.Join(s.OutDir(), filename)
	file, err := os.Create(path)
	if err != nil {
		s.Fatalf("Failed to create file %s: %v", filename, err)
	}
	defer file.Close()

	w := csv.NewWriter(file)
	w.Write([]string{"shift_1", "altgr_1", "caps_1", "location_1", "shift_2", "altgr_2", "caps_2", "location_2", "string", "unicode"})
	noOpKeystrokes := make([]keystroke, 0)

	// Whether ESC is needed to abort possible dead-key composition or
	// modifier latch from previous iteration.
	needEsc := false

	// Alt+Search should be equivalent to Capslock, but unlike real Capslock
	// it unexpectedly disrupts dead-key composition (crbug/383673473), so
	// shortcut should be avoided where possible. For JIS, real Capslock
	// doesn't always work (crbug/408113747) and fortunately JIS is known to
	// not have dead keys, so use shortcut for Capslock on JIS only.
	useShortcutForCapslock := hardwareLayoutType == "JIS"

	if err := its.ClickFieldAndWaitForActive(inputField)(ctx); err != nil {
		s.Fatal("Failed to ClickFieldAndWaitForActive: ", err)
	}

	for _, modifiers := range util.ModifiersStatusCombo {
		if !modifiersEligibleForHardwareLayout(modifiers, hardwareLayoutType) {
			continue
		}
		for _, key := range util.LinuxKeyCodes {
			if !keyEligibleForHardwareLayout(key, hardwareLayoutType) {
				continue
			}
			if err := uiauto.NamedCombine(fmt.Sprintf("typing %s + %s", getModifierInfo(modifiers), key.KeyName),
				its.Clear(inputField),
				util.SingleKeyAction(needEsc, useShortcutForCapslock, modifiers, key.LinuxKeyCode, kb),
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

			unicode := getUniCode(nodeInfo.Value)
			testing.ContextLogf(ctx, "result: [%s] %s", nodeInfo.Value, unicode)

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
				unicode})

			// No-op looking outcome indicates either true no-op,
			// or ongoing dead-key composition or modifier latch
			// (hence ESC to abort it in next iteration).
			needEsc = (nodeInfo.Value == "")
		}
	}

	for _, keystroke1 := range noOpKeystrokes {
		for _, modifiers2 := range util.ModifiersStatusCombo {
			if !modifiersEligibleForHardwareLayout(modifiers2, hardwareLayoutType) {
				continue
			}
			for _, key2 := range util.LinuxKeyCodes {
				if !keyEligibleForHardwareLayout(key2, hardwareLayoutType) {
					continue
				}
				if err := uiauto.NamedCombine(fmt.Sprintf("typing %s + %s, then %s + %s", getModifierInfo(keystroke1.modifiers), keystroke1.key.KeyName, getModifierInfo(modifiers2), key2.KeyName),
					its.Clear(inputField),
					util.TwoKeysAction(needEsc, useShortcutForCapslock, keystroke1.modifiers, modifiers2, keystroke1.key.LinuxKeyCode, key2.LinuxKeyCode, kb),
				)(ctx); err != nil {
					s.Fatal("Failed to typing key: ", err)
				}

				nodeInfo, err := ui.Info(ctx, inputField.Finder())
				if err != nil {
					s.Fatal("Failed to get node info: ", err)
				}

				unicode := getUniCode(nodeInfo.Value)
				testing.ContextLogf(ctx, "result: [%s] %s", nodeInfo.Value, unicode)

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
					unicode})

				// No-op looking outcome indicates either true
				// no-op, or dead-key composition or modifier
				// latch (hence ESC to abort in next iteration).
				needEsc = (nodeInfo.Value == "")
			}
		}
	}

	w.Flush()
}

func modifiersEligibleForHardwareLayout(modifiers util.ModifiersStatus, hardwareLayoutType string) bool {
	switch hardwareLayoutType {
	case "ISO":
		return true
	case "ANSI":
		return !modifiers.Altgr
	case "ABNT":
		return true
	case "JIS":
		return !modifiers.Altgr
	default:
		return false
	}
}

func keyEligibleForHardwareLayout(key util.LinuxKeyCode, hardwareLayoutType string) bool {
	switch hardwareLayoutType {
	case "ISO":
		return (key.LinuxKeyCode != input.KEY_YEN) && (key.LinuxKeyCode != input.KEY_RO)
	case "ANSI":
		return (key.LinuxKeyCode != input.KEY_102ND) && (key.LinuxKeyCode != input.KEY_YEN) && (key.LinuxKeyCode != input.KEY_RO)
	case "ABNT":
		return key.LinuxKeyCode != input.KEY_YEN
	case "JIS":
		// KEY_GRAVE exists on JIS but is a functional key, without
		// character assignments for text typing. On CrOS, it toggles
		// between "Alphanumeric for Japanese keyboard" (Latin-script
		// layout) and "Japanese" (IME) input methods.
		return (key.LinuxKeyCode != input.KEY_102ND) && (key.LinuxKeyCode != input.KEY_GRAVE)
	default:
		return false
	}
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
