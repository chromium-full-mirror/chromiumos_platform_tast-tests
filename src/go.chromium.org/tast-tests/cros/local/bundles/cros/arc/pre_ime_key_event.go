// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testKeyStroke struct {
	Key                 string
	ExpectedPreIMEKey   int
	ExpectedPostIMEKey  int
	ExpectedTextOnField string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     PreIMEKeyEvent,
		Desc:     "Checks View.onKeyPreIme() works on Android apps",
		Contacts: []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > IME VK
		BugComponent: "b:537350",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Timeout:      4 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
	})
}

func testPreIMEKeyEvent(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, a *arc.ARC, d *ui.Device, kb *input.KeyboardEventWriter, s *testing.State, testName, fieldID string, keystrokes []testKeyStroke) {
	const (
		apk          = "ArcPreImeKeyEventTest.apk"
		pkg          = "org.chromium.arc.testapp.preime"
		activityName = ".MainActivity"

		lastKeyDownLabelID     = pkg + ":id/last_key_down"
		lastPreIMEKeyLabelID   = pkg + ":id/last_pre_ime_key"
		startConsumingButtonID = pkg + ":id/start_consuming_events"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	act, err := arc.NewActivity(a, pkg, activityName)
	if err != nil {
		s.Fatalf("Failed to create a new activity %q", activityName)
	}
	defer act.Close(ctx)

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatalf("Failed to start the activity %q", activityName)
	}
	defer act.Stop(cleanupCtx, tconn)

	defer faillog.SaveScreenshotToFileOnError(cleanupCtx, cr, s.OutDir(), s.HasError, testName+"-screenshot.png")

	textField := d.Object(ui.ID(fieldID))
	if err := textField.WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to find the text field: ", err)
	}
	if err := textField.Click(ctx); err != nil {
		s.Fatal("Failed to click the text field: ", err)
	}
	if err := textField.SetText(ctx, ""); err != nil {
		s.Fatal("Failed to empty the text field: ", err)
	}

	if err := d.Object(ui.ID(fieldID), ui.Focused(true)).WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to focus a text field: ", err)
	}

	getExpectedKeyLabelText := func(keyCode int) string {
		if keyCode == 0 {
			return "null"
		}
		return fmt.Sprintf("key down: keyCode=%d", keyCode)
	}

	preIMEKeyLabel := d.Object(ui.ID(lastPreIMEKeyLabelID))
	keyDownLabel := d.Object(ui.ID(lastKeyDownLabelID))
	for _, key := range keystrokes {
		if err := kb.Type(ctx, key.Key); err != nil {
			s.Fatalf("Failed to type %q", key.Key)
		}
		if err := preIMEKeyLabel.WaitForText(ctx, getExpectedKeyLabelText(key.ExpectedPreIMEKey), 5*time.Second); err != nil {
			if actual, err := preIMEKeyLabel.GetText(ctx); err != nil {
				s.Fatal("Failed to get text: ", err)
			} else {
				s.Fatalf("Got input %q from preIMEKeyLabel field; want %q", actual, getExpectedKeyLabelText(key.ExpectedPreIMEKey))
			}
		}
		if err := keyDownLabel.WaitForText(ctx, getExpectedKeyLabelText(key.ExpectedPostIMEKey), 5*time.Second); err != nil {
			if actual, err := keyDownLabel.GetText(ctx); err != nil {
				s.Fatal("Failed to get text: ", err)
			} else {
				s.Fatalf("Got input %q from keyDownLabel field; want %q", actual, getExpectedKeyLabelText(key.ExpectedPostIMEKey))
			}
		}
		if err := textField.WaitForText(ctx, key.ExpectedTextOnField, 5*time.Second); err != nil {
			if actual, err := textField.GetText(ctx); err != nil {
				s.Fatal("Failed to get text: ", err)
			} else {
				s.Fatalf("Got input %q from text field; want %q", actual, key.ExpectedTextOnField)
			}
		}
	}

	// Press the button to make the app consume every event in onKeyPreIme().
	button := d.Object(ui.ID(startConsumingButtonID))
	if err := button.WaitForExists(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to find the button: ", err)
	}
	if err := button.Click(ctx); err != nil {
		s.Fatal("Failed to click the button: ", err)
	}

	// View's onKeyDown() and IME's onKeyDown() never get the key event if it's consumed in onKeyPreIme().
	const initialLabelText = "null"
	const initialFieldText = "hello"
	textField.SetText(ctx, initialFieldText)
	for _, key := range keystrokes {
		if err := kb.Type(ctx, key.Key); err != nil {
			s.Fatalf("Failed to type %q", key.Key)
		}
		if err := preIMEKeyLabel.WaitForText(ctx, getExpectedKeyLabelText(key.ExpectedPreIMEKey), 5*time.Second); err != nil {
			if actual, err := preIMEKeyLabel.GetText(ctx); err != nil {
				s.Fatal("Failed to get text: ", err)
			} else {
				s.Fatalf("Got input %q from preIMEKeyLabel field; want %q", actual, getExpectedKeyLabelText(key.ExpectedPreIMEKey))
			}
		}
		if err := keyDownLabel.WaitForText(ctx, initialLabelText, 5*time.Second); err != nil {
			if actual, err := keyDownLabel.GetText(ctx); err != nil {
				s.Fatal("Failed to get text: ", err)
			} else {
				s.Fatalf("Got input %q from keyDownlabel field; want %q", actual, initialLabelText)
			}
		}
		if err := textField.WaitForText(ctx, initialFieldText, 5*time.Second); err != nil {
			if actual, err := textField.GetText(ctx); err != nil {
				s.Fatal("Failed to get text: ", err)
			} else {
				s.Fatalf("Got input %q from text field; want %q", actual, initialFieldText)
			}
		}
	}
}

func PreIMEKeyEvent(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	const (
		apk          = "ArcPreImeKeyEventTest.apk"
		pkg          = "org.chromium.arc.testapp.preime"
		activityName = ".MainActivity"

		editTextID     = pkg + ":id/text"
		nullEditTextID = pkg + ":id/null_edit"
	)

	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	for _, tc := range []struct {
		name    string
		fieldID string
		strokes []testKeyStroke
	}{
		{
			// On a normal text field, onKeyPreIme() is called, however, onKeyDown() is not called
			// because IME consumes the key event.
			name:    "normal",
			fieldID: editTextID,
			strokes: []testKeyStroke{
				{"a", 29, 0, "a"},    // AKEYCODE_A
				{"b", 30, 0, "ab"},   // AKEYCODE_B
				{"c", 31, 0, "abc"},  // AKEYCODE_C
				{"\b", 67, 67, "ab"}, // AKEYCODE_DEL
				{"\n", 66, 66, "ab"}, // AKEYCODE_ENTER
			},
		},
		{
			// On a TYPE_NULL text field, both of onKeyPreIme() and onKeyDown() are called.
			name:    "null",
			fieldID: nullEditTextID,
			strokes: []testKeyStroke{
				{"a", 29, 29, "a"},   // AKEYCODE_A
				{"b", 30, 30, "ab"},  // AKEYCODE_B
				{"c", 31, 31, "abc"}, // AKEYCODE_C
				{"\b", 67, 67, "ab"}, // AKEYCODE_DEL
				{"\n", 66, 66, "ab"}, // AKEYCODE_ENTER
			},
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			testPreIMEKeyEvent(ctx, tconn, cr, a, d, kb, s, tc.name, tc.fieldID, tc.strokes)
		})
	}
}
