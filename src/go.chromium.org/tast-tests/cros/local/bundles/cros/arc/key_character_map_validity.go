// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var inputDumpsysKcmPattern = regexp.MustCompile(`KeyCharacterMap: (.*)`)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeyCharacterMapValidity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks all KeyCharacterMap are generated and can be loaded successfully",
		Contacts:     []string{"arc-framework+tast@google.com", "nergi@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func KeyCharacterMapValidity(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	p := s.FixtValue().(*arc.PreData)
	cr := p.Chrome
	a := p.ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	imeID, err := ime.CurrentInputMethod(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get current ime: ", err)
	}
	imePrefix, err := ime.Prefix(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get ime prefix: ", err)
	}
	defer func(ctx context.Context) {
		if err := ime.SetCurrentInputMethod(ctx, tconn, imeID); err != nil {
			s.Error("Failed to set the default input method: ", err)
		}
	}(cleanupCtx)

	for _, im := range ime.InputMethods() {
		s.Run(ctx, im.Name, func(ctx context.Context, s *testing.State) {
			if err := ime.AddInputMethod(ctx, tconn, imePrefix+im.ID); err != nil {
				s.Fatalf("Failed to add IME %q: %v", im.Name, err)
			}
			// Don't remove default IME.
			if im.ID != ime.DefaultInputMethod.ID {
				defer func(ctx context.Context) {
					if err := ime.RemoveInputMethod(ctx, tconn, imePrefix+im.ID); err != nil {
						s.Errorf("Failed to remove the IME %q: %v", im.Name, err)
					}
				}(cleanupCtx)
			}
			// The timeout here will be used for time.sleep to wait for IME to warm up.
			// Since we are only testing KCM generation, we can pass 0 second.
			if err := ime.SetCurrentInputMethodAndWaitWarmUp(ctx, tconn, imePrefix+im.ID, 0*time.Second); err != nil {
				s.Fatalf("Failed to set IME %q: %v", im.Name, err)
			}
			keyboardLayout, err := ime.CurrentInputMethodKeyboardLayout(ctx, tconn)
			if err != nil {
				s.Fatalf("Failed to get current keyboard layout for IME %q: %v", im.Name, err)
			}
			s.Logf("Layout: %q", keyboardLayout)

			expectedKcm := expectedKcmNameFromKeyboardLayout(keyboardLayout)
			err = validateKcm(ctx, a, expectedKcm)
			if err != nil {
				s.Fatalf("Failed to validate kcm %q: %v", im.Name, err)
			}
		})
	}
}

func expectedKcmNameFromKeyboardLayout(keyboardLayout string) string {
	keyboardLayout = strings.Replace(keyboardLayout, "(", "_", -1)
	return strings.Replace(keyboardLayout, ")", "_", -1) + ".kcm"
}

// validateKcm indirectly validates `loadKeyCharacterMap` in Android
// is able to read and load the requested KCM. This works based on the fact that
// `loadKeyCharacterMap` will revert to `Generic.kcm` when it fails to load KCM.
// Hence, we are only checking if loaded KCM name matches IME's keyboard layout name
// Testing actual file content of some layouts are tested in `key_character_map.go`.
func validateKcm(ctx context.Context, a *arc.ARC, expectedKcm string) error {
	// Adding long delay here as running `adb dumpsys input` takes a long time,
	// sometimes leading to multiple timeouts.
	return testing.Poll(ctx, func(ctx context.Context) error {
		inputDumpsys, err := a.Command(ctx, "dumpsys", "input").Output()
		if err != nil {
			return errors.Wrap(err, "failed to run \"dumpsys input\"")
		}

		// Returns ["KeyCharactermap: .../us.kcm", ".../us.kcm"].
		match := inputDumpsysKcmPattern.FindStringSubmatch(string(inputDumpsys))
		if len(match) < 2 {
			return errors.Wrapf(err, "failed to find pattern \"KeyCharacterMap: \" in dumpsys %q", inputDumpsys)
		}
		// Parse to get filename only.
		parts := strings.Split(match[1], "/")
		selectedKcm := parts[len(parts)-1]

		if selectedKcm != expectedKcm {
			return errors.Wrapf(err, "selected KCM %q doesn't match %q", selectedKcm, expectedKcm)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
}
