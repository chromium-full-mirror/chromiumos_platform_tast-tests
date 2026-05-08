// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mahi provides Mahi related tests (e.g. power test).
package mahi

import (
	"context"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/ui/mahicuj/mahiutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	urlCount             = 50
	interval             = 10 * time.Second
	localTextZip         = "simplify_text.zip"
	powerDefaultInterval = 10 * time.Second
)

type simplifyTestParameters struct {
	expectSimplifyButton bool
	doSimplify           bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: PowerMetricsForSimplify,
		Desc: "Collect power metrics of using Simplify",
		Contacts: []string{
			"ml-service-team@google.com",
			"chenjih@google.com",
			"thanhdng@google.com",
		},
		BugComponent: "b:1673015",
		Timeout:      20*time.Minute + power.RecorderTimeout,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Params: []testing.Param{
			{
				Name: "disabled",
				Val: simplifyTestParameters{
					expectSimplifyButton: false,
					doSimplify:           false,
				},
				Fixture: "powerAshMahi",
			},
			{
				Name: "enabled_not_used",
				Val: simplifyTestParameters{
					expectSimplifyButton: true,
					doSimplify:           false,
				},
				Fixture: "powerAshMahiAndSimplify",
			},
			{
				Name: "enabled_and_used",
				Val: simplifyTestParameters{
					expectSimplifyButton: true,
					doSimplify:           true,
				},
				Fixture: "powerAshMahiAndSimplify",
			},
		},
		Data: []string{
			localTextZip,
		},
	})
}

func PowerMetricsForSimplify(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open the browser
	conn, err := cr.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := mahiutil.InstallScreenAIDLC(ctx); err != nil {
		s.Fatal("Install DLC failed: ", err)
	}

	ui := uiauto.New(tconn)
	window, _ := ash.GetActiveWindow(ctx, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Unzip txt zip file and start local server
	localTextPath, localTextFiles, localServer, err := mahiutil.PrepareLocalServer(ctx, s.DataPath(localTextZip))
	if err != nil {
		s.Fatal("Failed to prepare local text files: ", err)
	}
	defer os.RemoveAll(localTextPath)
	defer localServer.Close()

	r := power.NewRecorder(ctx, powerDefaultInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	// Cool down test device.
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
		s.Fatal("Failed to clean UI element beforehand: ", err)
	}

	if err := mahiutil.MaybePassConsentFlow(
		ctx, conn, tconn, window, ui, kb, localServer.URL+"/"+localTextFiles[0].Name()); err != nil {
		s.Fatal("Failed to pass the consent flow: ", err)
	}

	params := s.Param().(simplifyTestParameters)
	index := 0
	succeedCount := 0
	localTextCount := len(localTextFiles)

	testing.Poll(ctx, func(ctx context.Context) error {
		if index == urlCount {
			s.Log("finished")
			return nil
		}

		fileName := localTextFiles[index%localTextCount].Name()
		index++

		if err := mahiutil.NavigateToURL(ctx, conn, localServer.URL+"/"+fileName); err != nil {
			return errors.Wrapf(err, "failed to open local file %s", fileName)
		}

		// Selects the content, right click then clicks the Simplify button.
		content, err := mahiutil.ReadTextFile(path.Join(localTextPath, fileName))
		if err != nil {
			return errors.Wrapf(err, "failed to read content of file %s", fileName)
		}

		var expectedFinder *nodewith.Finder = nil
		if params.expectSimplifyButton {
			expectedFinder = mahiutil.SimplifyButton
		}

		if err := mahiutil.SelectContentAndRightClick(ctx, ui, content, expectedFinder); err != nil {
			return errors.Wrap(err, "failed to do a right click")
		}

		if params.doSimplify {
			if err := mahiutil.DoSimplify(ctx, ui, false /*expectResponse*/); err != nil {
				s.Log("Failed to click the simplify button and wait for the result panel: ", err)
				return errors.Wrap(err, "failed to click the simplify button and wait for the result panel")
			}
		}

		if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
			s.Fatal("Failed to clean UI element: ", err)
		}

		succeedCount++
		return errors.New("not finish yet")
	}, &testing.PollOptions{
		Timeout:  15 * time.Minute,
		Interval: interval,
	})

	s.Log("succeedCount = ", succeedCount, " urlCount = ", urlCount)
	if urlCount-succeedCount > 5 {
		s.Error("Got too many failures, failed count = ", urlCount-succeedCount)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
