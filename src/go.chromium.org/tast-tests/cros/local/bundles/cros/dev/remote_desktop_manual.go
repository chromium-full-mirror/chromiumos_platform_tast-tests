// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dev

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/crd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteDesktopManual,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Connect to Chrome Remote Desktop for manual test",
		Contacts:     []string{"chromoting-team@google.com", "jinrongwu@google.com"},
		BugComponent: "b:47377", // Chrome > Chromoting
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"user", "pass", "contact", "extra_args",
		},
		Params: []testing.Param{{
			Name: "",
			Val:  browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
		Timeout: 10 * time.Minute,
	})
}

// RemoteDesktopManual is similar to RemoteDesktop.
// It is for manual test on the lab devices when the tester do not have
// the required devices locally.
// Some testers do not have access to the internal repo.
// Therefore make a clean copy for them.
func RemoteDesktopManual(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	user, hasUser := s.Var("user")
	if !hasUser {
		s.Fatal("You must provide a user")
	}

	pass, hasPass := s.Var("pass")

	contact, hasContact := s.Var("contact")
	if !hasPass && !hasContact {
		s.Fatal("You must ether provide password or contact for the user to login")
	}

	extraArgsStr, ok := s.Var("extra_args")
	if !ok {
		extraArgsStr = ""
	}
	extraArgs := strings.Fields(extraArgsStr)

	var opts []chrome.Option

	chromeARCOpt := chrome.ARCDisabled()
	if arc.Supported() {
		chromeARCOpt = chrome.ARCSupported()
	}
	opts = append(opts, chromeARCOpt)
	opts = append(opts, chrome.GAIALogin(chrome.Creds{
		User:    user,
		Pass:    pass,
		Contact: contact,
	}))

	opts = append(opts, chrome.ExtraArgs(extraArgs...))

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

	s.Log("Waiting connection")
	if err := crd.WaitConnection(ctx, tconn); err != nil {
		s.Fatal("No client connected: ", err)
	}
}
