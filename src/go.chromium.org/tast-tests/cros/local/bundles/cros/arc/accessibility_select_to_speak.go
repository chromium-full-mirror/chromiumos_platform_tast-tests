// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/sts"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/arc"
	arca11y "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AccessibilitySelectToSpeak,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks Select-to-Speak works on ARC windows",
		Contacts:     []string{"arc-framework+tast@google.com", "hirokisato@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Accessibility
		BugComponent: "b:165222",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithoutUIAutomator",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func AccessibilitySelectToSpeak(ctx context.Context, s *testing.State) {
	d := s.FixtValue().(*arc.PreData)
	a := d.ARC
	cr := d.Chrome

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	_, cleanupS2S, err := arca11y.SetUpSelectToSpeak(ctx, s, cr, a, tconn)
	if err != nil {
		s.Fatal("Failed to setup SelectToSpeak: ", err)
	}
	defer cleanupS2S(cleanupCtx)

	arca11y.AttachFaillog(ctx, s, tconn, "faillog")
	cleanupAct, err := arca11y.StartActivityWithSelectToSpeak(ctx, s, a, tconn, arca11y.MainActivity)
	if err != nil {
		s.Fatal("Failed to setup activity with S2S: ", err)
	}
	defer cleanupAct(cleanupCtx)

	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, tts.GoogleTTSEngine())
	if err != nil {
		s.Fatal("Failed to connect to the TTS background page: ", err)
	}
	defer sm.Close()

	appRoot := nodewith.Name(arca11y.MainActivity.Title).Role(role.Application)

	buttonRegexp := "(?i)ANNOUNCE"
	textNode := nodewith.
		NameRegex(regexp.MustCompile(buttonRegexp)).
		Role(role.Button).
		Ancestor(appRoot)
	expectations := []tts.SpeechExpectation{tts.NewRegexExpectation(buttonRegexp)}
	if err := sts.ClickAndDragToActivate(ctx, cr, textNode, sm, expectations); err != nil {
		s.Fatal("Failed to drag and activate with S2S: ", err)
	}
}
