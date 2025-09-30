// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardJapaneseTypingPower,
		Desc:         "Collect power metrics for Japanese physical keyboard typing",
		BugComponent: "b:95887",
		Contacts:     []string{"e14s-eng@google.com"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "enabled",
			Fixture: fixture.ClamshellNonVKRestart,
			Val:     power.TimeParams{Total: 10 * time.Minute, Interval: 5 * time.Second},
		},
		},
		Timeout: 15*time.Minute + power.RecorderTimeout,
		Attr: []string{
			"group:crosbolt",
			"crosbolt_perbuild",
		},
	})
}

func PhysicalKeyboardJapaneseTypingPower(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	powerInterval := s.Param().(power.TimeParams).Interval
	powerTotal := s.Param().(power.TimeParams).Total
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanup, _, err := setup.PowerTestSetup(ctx, "powerd and multicast disabled", nil, &setup.PowerTestOptions{
		Powerd:    setup.DisablePowerd,
		Multicast: setup.DisableMulticast,
	})
	if err != nil {
		s.Fatal("Failed to disable powerd and multicast: ", err)
	}
	defer cleanup(cleanupCtx)

	cr := s.FixtValue().(fixture.FixtData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to access keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputField := testserver.TextAreaInputField

	im := ime.Japanese
	s.Log("Set current input method to: ", im)
	if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %v: %v: ", im, err)
	}

	r := power.NewRecorder(ctx, powerInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if _, err := power.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	sentences := []string{
		"wagahaihanekodearu.",
		"namaehamadanai.",
		"dokodeumaretakatontokentoogatsukanu.",
		"dokodeumaretakatontokentoogatsukanu.",
		"nanidemousuguraijimejimeshitatokorodenya-nya-naiteitakotodakehakiokushiteiru.",
		"wagahaihakokodehajimeteningentoiumonoomita.",
		"shikamoatodekikutosorehashoseitoiuningenchuudeichibandouakunashuzokudeattasouda.",
		"konoshoseitoiunohatokidokiwarewareotoraetenitekuutoiuhanashidearu.",
		"shikashisonotoujihananitoiukoumonakattakarabetsudankowashiitomoomowanakatta.",
		"tadakarenotenohiraninoseraretesu-tomochiageraretatokinandakafuwafuwashitakanjigaattabakaridearu.",
		"tenohiranouedesukoshiochitsuiteshoseinokaoomitanogaiwayuruningentoiumononomihajimedearou",
		"konotokimyounamonodatoomottakanjigaimademonokotteiru.",
		"daiichimouomottesoushokusarebekihazunokaogatsurutsurushitemarudeyakanda.",
		"sonogonekonimodaibuattagakonnakatawanihaichidomodeawashitakotoganai.",
		"nominarazukaonomannakagaamarinitokkishiteiru.",
		"soushitesonoananonakakaratokidokipuupuutokemuriofuku.",
		"doumonondosepokutejitsuniyowatta.",
		"koreganingennonomutabakotoiumonodearukotohayouyakukonokoroshitta.",
	}

	// Run through the sentences repeatedly until `powerTotal` has elapsed.
	for i, startTime := 0, time.Now(); time.Since(startTime) < powerTotal; i++ {
		// Type the sentence with Japanese keyboard
		sentence := sentences[i%len(sentences)]
		if err := uiauto.Combine("type some text",
			its.ClearThenClickFieldAndWaitForActive(inputField),
			keyboard.TypeAction(sentence),
			keyboard.AccelAction("Space"),
			keyboard.AccelAction("Enter"),
		)(ctx); err != nil {
			s.Error("Failed to search type sentence: ", err)
		}
	}

	// End of main test body.
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}

	if err := power.SaveScreenshot(ctx, cr); err != nil {
		s.Error("Failed to take screenshot: ", err)
	}
}
