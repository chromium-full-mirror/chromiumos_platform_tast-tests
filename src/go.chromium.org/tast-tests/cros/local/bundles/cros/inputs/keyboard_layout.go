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
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
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

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeyboardLayout,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Captures e2e physical keyboard layout behaviours",
		Contacts:     []string{"xiuwen@google.com", "tranbaoduy@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{},
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Timeout:      150 * time.Minute,
		Fixture:      fixture.ClamshellNonVK,
		Vars: []string{
			"imeID",
		},
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

	its, err := testserver.LaunchBrowser(ctx, browser.TypeAsh, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	inputField := testserver.TextAreaInputField

	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)

	id, hasID := s.Var("imeID")
	if !hasID {
		s.Fatal("Missing input method ID arg when running tast command")
	}

	// Check if the target ime info exists or not.
	var inputMethod *ime.InputMethod

	if inputMethod, err = ime.FindInputMethodByID(id); err != nil {
		s.Fatal("Missing input method info in ime.inputMethods")
	}

	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatal("Failed to set input method: ", err)
	}

	filename := fmt.Sprintf("%s.csv", inputMethod.Name)
	path := filepath.Join(s.OutDir(), filename)
	file, err := os.Create(path)
	if err != nil {
		s.Fatalf("Failed to create file %s: %v", filename, err)
	}
	defer file.Close()

	w := csv.NewWriter(file)
	w.Write([]string{"key1-shift", "key1-altgr", "key1-caps", "key1-location", "key2-shift", "key2-altgr", "key2-caps", "key2-location", "char", "unicode"})

	for _, key1ModifiersStatus := range util.ModifiersStatusCombo {
		testing.ContextLogf(ctx, "Start single key case with modifer shift: %t + altgr: %t + caps: %t ", key1ModifiersStatus.Shift, key1ModifiersStatus.Altgr, key1ModifiersStatus.Caps)

		for _, key := range util.LinuxKeyCodes {
			if err := uiauto.Combine("typing key",
				its.Clear(inputField),
				its.ClickFieldAndWaitForActive(inputField),
				util.SingleKeyAction(key1ModifiersStatus, key.LinuxKeyCode, kb),
			)(ctx); err != nil {
				s.Fatal("Failed to typeing key: ", err)
			}

			nodeInfo, err := ui.Info(ctx, inputField.Finder())
			if err != nil {
				s.Fatal("Failed to get node info: ", err)
			}

			unicode := getUniCode(nodeInfo.Value)

			w.Write([]string{
				strconv.FormatBool(key1ModifiersStatus.Shift),
				strconv.FormatBool(key1ModifiersStatus.Altgr),
				strconv.FormatBool(key1ModifiersStatus.Caps),
				key.KeyName,
				"false",
				"false",
				"false",
				"n/a",
				nodeInfo.Value,
				unicode})
		}
	}

	w.Flush()
}

func getUniCode(str string) string {
	if str == "" {
		return "no-op"
	}

	var unicodeArr []string

	for _, runeValue := range str {
		unicodeArr = append(unicodeArr, fmt.Sprintf("%U", runeValue))
	}
	return strings.Join(unicodeArr, ",")
}
