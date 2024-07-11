// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dev

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dev"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/crd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// rdpVars represents the configurable parameters for the remote desktop session.
type rdpVars struct {
	user      string
	pass      string
	contact   string
	wait      bool
	reset     bool
	extraArgs []string
}

var variousPlatformModels = []string{
	"atlas",
	"careena",
	"dru",
	"eve",
	"kohaku",
	"krane",
	"nocturne",
}

var dededeModels = []string{
	"beadrix",
	"beetley",
	"blipper",
	"bookem",
	"boten",
	"botenflex",
	"boxy",
	"bugzzy",
	"cret",
	"cret360",
	"dexi",
	"dibbi",
	"dita",
	"drawcia",
	"drawlat",
	"drawman",
	"drawper",
	"galith",
	"galith360",
	"gallop",
	"galnat",
	"galnat360",
	"galtic",
	"galtic360",
	"kracko",
	"kracko360",
	"landia",
	"landrid",
	"lantis",
	"madoo",
	"magister",
	"maglet",
	"maglia",
	"maglith",
	"magma",
	"magneto",
	"magolor",
	"magpie",
	"metaknight",
	"oscino",
	"pasara",
	"peezer",
	"pirette",
	"pirika",
	"palutena",
	"sasuke",
	"sasukette",
	"shotzo",
	"storo",
	"storo360",
	"taranza",
	"waddledee",
}

func init() {
	// Example usage:
	// $ tast run -var=user=<username> -var=pass=<password> <dut ip> dev.RemoteDesktop
	// <username> and <password> are the credentials of the test GAIA account.
	testing.AddTest(&testing.Test{
		Func:         RemoteDesktop,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Connect to Chrome Remote Desktop for working remotely",
		Contacts:     []string{"chromoting-team@google.com", "shik@chromium.org"},
		BugComponent: "b:47377", // Chrome > Chromoting
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			// For running manually.
			"user", "pass", "contact", "wait", "extra_args", "reset",
		},
		VarDeps: []string{
			dev.AccountVarName,
		},
		Params: []testing.Param{{
			// For running manually.
			Name: "",
			Val:  browser.TypeAsh,
		}, {
			// For automated testing.
			Name:      "test",
			ExtraAttr: []string{"group:mainline", "informational"},
			// TODO(b/151111783): This is a speculative fix to limit the number of sessions. It
			// seems that the test account is throttled by the CRD backend, so the test is failing
			// with a periodic pattern. The model list is handcrafted to cover various platforms.
			// Please keep dededeModels. It's important for DMA testing.
			// Although it's a long list, it only add 2 test runs per build.
			ExtraHardwareDeps: hwdep.D(hwdep.Model(append(variousPlatformModels, dededeModels...)...)),
			ExtraSoftwareDeps: []string{"gaia"},
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}, {
			// For automated testing.
			Name:              "test_lacros",
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraHardwareDeps: hwdep.D(hwdep.Model(append(variousPlatformModels, dededeModels...)...)),
			Val:               browser.TypeLacros,
		}},
	})
}

// getVars extracts the testing parameters from testing.State. The user
// provided credentials would override the credentials from config file.
func getVars(s *testing.State) rdpVars {

	defaultUser, defaultPass, err := dma.UserPassFromPool(dev.AccountVarName)
	if err != nil {
		s.Fatal("Failed to pick up creds: ", err)
	}

	user, hasUser := s.Var("user")
	if !hasUser {
		user = defaultUser
	}

	pass, hasPass := s.Var("pass")
	if !hasPass {
		pass = defaultPass
	}

	contact, hasContact := s.Var("contact")

	// Manual test requires username + password/contact.
	manual := false
	if hasUser && (hasPass || hasContact) {
		manual = true
	}

	resetStr, ok := s.Var("reset")
	if !ok {
		// Default to reset Chrome between tests. Note that setting false means that the test will pick up whatever dirty state the device is in (eg. profile used from a previous test).
		resetStr = "true"
	}
	reset, err := strconv.ParseBool(resetStr)
	if err != nil {
		s.Fatal("Failed to parse the variable `reset`: ", err)
	}

	waitStr, ok := s.Var("wait")
	if !ok {
		// Only wait for remote connection when running manually.
		if manual {
			waitStr = "true"
		} else {
			waitStr = "false"
		}
	}
	wait, err := strconv.ParseBool(waitStr)
	if err != nil {
		s.Fatal("Failed to parse the variable `wait`: ", err)
	}

	extraArgsStr, ok := s.Var("extra_args")
	if !ok {
		extraArgsStr = ""
	}
	extraArgs := strings.Fields(extraArgsStr)

	return rdpVars{
		user:      user,
		pass:      pass,
		contact:   contact,
		wait:      wait,
		reset:     reset,
		extraArgs: extraArgs,
	}
}

func RemoteDesktop(ctx context.Context, s *testing.State) {
	// TODO(shik): The button names only work in English locale, and adding
	// "lang=en-US" for Chrome does not work.

	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	vars := getVars(s)
	var opts []chrome.Option

	chromeARCOpt := chrome.ARCDisabled()
	if arc.Supported() {
		chromeARCOpt = chrome.ARCSupported()
	}
	opts = append(opts, chromeARCOpt)
	opts = append(opts, chrome.GAIALogin(chrome.Creds{
		User:    vars.user,
		Pass:    vars.pass,
		Contact: vars.contact,
	}))

	if !vars.reset {
		opts = append(opts, chrome.KeepState())
	}

	if len(vars.extraArgs) > 0 {
		opts = append(opts, chrome.ExtraArgs(vars.extraArgs...))
	}

	// Set up the browser.
	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, s.Param().(browser.Type), lacrosfixt.NewConfig(), opts...)
	if err != nil {
		// In case of authentication error, provide a more informative message to the user.
		if strings.Contains(err.Error(), "chrome.Auth") {
			err = errors.Wrap(err, "please supply a password with -var=pass=<password>")
		} else if strings.Contains(err.Error(), "chrome.Contact") {
			err = errors.Wrap(err, "please supply a contact email with -var=contact=<contact>")
		}
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if err := crd.Launch(ctx, br, tconn); err != nil {
		s.Fatal("Failed to Launch: ", err)
	}

	if vars.wait {
		s.Log("Waiting connection")
		if err := crd.WaitConnection(ctx, tconn); err != nil {
			s.Fatal("No client connected: ", err)
		}
	} else {
		s.Log("Skip waiting remote connection")
	}
}
