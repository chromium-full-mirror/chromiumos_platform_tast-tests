// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayStorePersistent,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Makes sure that Play Store remains open after it is fully initialized",
		Contacts:     []string{"arc-core@google.com"},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"arc_android_data_cros_access", "chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Val:               browser.TypeLacros,
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros_vm",
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Val:               browser.TypeLacros,
		}},
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
	})
}

// getPlayStorePid gets the PID of Play Store application.
func getPlayStorePid(ctx context.Context, a *arc.ARC) (uint, error) {
	out, err := a.Command(ctx, "pidof", "com.android.vending").Output()
	if err != nil {
		return 0, err
	}

	m := regexp.MustCompile(`(\d+)\n`).FindAllStringSubmatch(string(out), -1)
	if m == nil || len(m) != 1 {
		return 0, errors.New("could not find Play Store app")
	}

	pid, err := strconv.ParseUint(m[0][1], 10, 32)
	if err != nil {
		return 0, err
	}

	return uint(pid), nil
}

// waitForDailyHygieneDone waits for Play Store daily hygiene is done. dailyhygiene-last-version
// in shared Finsky pref is set in case this flow is finished. Usually this happens in 2 minutes.
// At this moment, Play Store self-update might be executing. This also handles the case when
// daily hygiene fails internally. This is not ARC fault and we detect this as a signal that
// daily hygiene ends. Next potentially successful attempt should happen in 20 min which is
// problematic to wait in test.
func waitForDailyHygieneDone(ctx context.Context, user string) (bool, []byte, error) {
	reOk := regexp.MustCompile(`<int name="dailyhygiene-last-version" value="\d+"`)
	reFail := regexp.MustCompile(`<int name="dailyhygiene-failed" value="1" />`)

	androidDataDir, err := arc.AndroidDataDir(ctx, user)
	if err != nil {
		return false, []byte{}, errors.Wrap(err, "failed to get android-data dir for the user")
	}
	finskyPrefsPath := filepath.Join(androidDataDir, "data/data/com.android.vending/shared_prefs/finsky.xml")

	var ok bool
	var fileContent []byte
	err = arc.PollWithReadOnlyAndroidData(ctx, user, func(ctx context.Context) error {
		fileContent, err = ioutil.ReadFile(finskyPrefsPath)
		if err != nil {
			// It is OK if it does not exist yet
			return err
		}
		if reOk.Find(fileContent) != nil {
			ok = true
			return nil
		}
		if reFail.Find(fileContent) != nil {
			ok = false
			return nil
		}
		return errors.New("dailyhygiene is not yet complete")
	}, &testing.PollOptions{Timeout: 4 * time.Minute, Interval: 5 * time.Second})
	return ok, fileContent, err
}

func PlayStorePersistent(ctx context.Context, s *testing.State) {

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	opts := []chrome.Option{
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
	}

	bt := s.Param().(browser.Type)
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(), opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Optin to Play Store.
	s.Log("Opting into Play Store")
	maxAttempts := 2
	if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	pidBefore, err := getPlayStorePid(ctx, a)
	if err != nil {
		s.Fatal("Failed to get initial PlayStore PID: ", err)
	}

	s.Log("Waiting for daily hygiene done")
	ok, out, err := waitForDailyHygieneDone(ctx, cr.NormalizedUser())
	if err != nil {
		if rerr := ioutil.WriteFile(filepath.Join(s.OutDir(), "finsky.xml"), out, 0644); rerr != nil {
			s.Error("Failed to write Finsky prefs: ", rerr)
		} else {
			s.Log("Finsky prefs is saved to finsky.xml")
		}
		s.Log("Failed to wait daily hygiene done")
	}

	if ok {
		s.Log("Daily hygiene finished successfully")
	} else {
		s.Log("Daily hygiene failed but continue")
	}

	// Daily hygiene may start the self-update flow and now system is busy. This waiting just waits
	// everything is stabilized. That means new Play Store is installed if self-update flow was
	// started.
	s.Log("Waiting for CPU idle")
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Fatal("Failed to wait CPU is idle: ", err)
	}

	pidAfter, err := getPlayStorePid(ctx, a)
	if err != nil {
		s.Fatal("Failed to get PlayStore PID: ", err)
	}

	if pidAfter != pidBefore {
		s.Fatal("Play Store was restarted")
	}
}
