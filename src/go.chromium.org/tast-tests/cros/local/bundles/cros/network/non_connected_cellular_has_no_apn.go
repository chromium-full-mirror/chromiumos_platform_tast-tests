// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type nonConnectedCellularHasNoApnTestParam struct {
	loggedIn bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         NonConnectedCellularHasNoApn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that no APNs are shown for cellular networks that are not connected",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			Name: "logged_out",
			Val: nonConnectedCellularHasNoApnTestParam{
				loggedIn: false,
			},
		}, {
			Name: "logged_in",
			Val: nonConnectedCellularHasNoApnTestParam{
				loggedIn: true,
			},
		}},
		VarDeps: []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

func NonConnectedCellularHasNoApn(ctx context.Context, s *testing.State) {
	login := s.Param().(nonConnectedCellularHasNoApnTestParam).loggedIn

	var options []chrome.Option
	options = append(options, chrome.EnableFeatures("ApnRevamp"))
	if login {
		options = append(options, chrome.FakeLogin(chrome.Creds{User: chrome.DefaultUser, Pass: chrome.DefaultPass}))
	} else {
		options = append(options, chrome.NoLogin(), chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))
	}

	cr, err := chrome.New(ctx, options...)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(ctx)

	var testAPIConn func(context.Context) (*chrome.TestConn, error)
	if login {
		testAPIConn = cr.TestAPIConn
	} else {
		testAPIConn = cr.SigninProfileTestAPIConn
	}

	tconn, err := testAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to the default cellular service: ", err)
	}

	name, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Failed to get the name of the default cellular service: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err = quicksettings.SelectNetwork(ctx, tconn, name); err != nil {
		s.Fatal("Failed to open the Quick Settings and select the default cellular service from the network detailed view: ", err)
	}

	lastGoodApn, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		s.Fatal("Failed to get the last good APN: ", ctx)
	}

	lastGoodApnInfoApnName := lastGoodApn[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]

	ui := uiauto.New(tconn)

	lastGoodApnNode := nodewith.NameContaining(lastGoodApnInfoApnName)
	if err := ui.WaitUntilExists(lastGoodApnNode)(ctx); err != nil {
		s.Fatal("Expected APN information to be shown but last good APN name was not found: ", err)
	}

	// Wait until after we have opened the network details via the Quick Settings to disconnect.
	if _, err := helper.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect to the default cellular service: ", err)
	}

	// TODO(b/298522793): Check that no APN name is shown in OOBE.
	if login {
		if err := ui.WithTimeout(5 * time.Second).WaitUntilGone(lastGoodApnNode)(ctx); err != nil {
			s.Fatal("Expected no APN information to be shown but last good APN name was found: ", err)
		}
	}

	var apnSubpageButton *nodewith.Finder
	if login {
		apnSubpageButton = ossettings.APNSubpageButton
	} else {
		apnSubpageButton = quicksettings.APNSubpageButton
	}
	if err := cellular.CheckThatApnListIsEmpty(ctx, tconn, apnSubpageButton, lastGoodApnNode); err != nil {
		s.Fatal("Failed to navigate to the APN subpage and verify no APN information is shown: ", err)
	}
}
