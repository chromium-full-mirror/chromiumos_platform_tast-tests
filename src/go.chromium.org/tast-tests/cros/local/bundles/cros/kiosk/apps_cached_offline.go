// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/network/firewall"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/network"
	local_firewall "go.chromium.org/tast-tests/cros/local/network/firewall"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppsCachedOffline,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks if Kiosk apps can be cached and launched offline",
		Contacts:     []string{"chromeos-kiosk-eng+TAST@google.com"},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{{
			Name: "ash",
			Val: kioskmode.TestData{
				IsLacros: false,
			},
		}, {
			Name: "lacros",
			Val: kioskmode.TestData{
				IsLacros: true,
				Policies: []policy.Policy{
					&policy.LacrosAvailability{Val: "lacros_only"},
				},
			},
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		Fixture: fixture.FakeDMSEnrolled,
		Timeout: kioskmode.SetupDuration + 2*kioskmode.LaunchDuration + kioskmode.CleanupDuration + time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Launch Chrome app kiosk offline.
				Value: "screenplay-79897752-32ad-43e4-826c-7c21f5bef5e8",
			},
		},
	})
}

func AppsCachedOffline(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(kioskmode.TestData)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, _, err := kioskmode.New(
		ctx,
		fdms,
		s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		kioskmode.PublicAccountPolicies(kioskmode.KioskAppAccountID, param.Policies),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	s.Log("Waiting for Kiosk crx to be cached")
	if err := kioskmode.WaitForCrxInCache(ctx, kioskmode.KioskAppID); err != nil {
		s.Fatal("Kiosk crx is not cached: ", err)
	}

	s.Log("Trying to launch Kiosk app offline")
	restartAndLaunchKiosk := func(ctx context.Context) error {
		// Additionally block access to FakeDMS to make Chrome think it's offline.
		firewallRunner := local_firewall.NewLocalRunner()
		fdmsURL, err := url.Parse(fdms.URL)
		if err != nil {
			return errors.Wrap(err, "failed to parse FakeDMS URL")
		}
		fdmsPort, err := strconv.Atoi(fdmsURL.Port())
		if err != nil {
			return errors.Wrap(err, "failed to parse port from FakeDMS URL")
		}

		commonRuleArgs := []firewall.RuleOption{
			firewall.OptionProto(firewall.L4ProtoTCP),
			firewall.OptionUIDOwner("chronos"),
			firewall.OptionDPort(fdmsPort),
			firewall.OptionJumpTarget(firewall.TargetDrop),
		}
		ruleArgs := []firewall.RuleOption{firewall.OptionAppendRule(firewall.OutputChain)}
		ruleArgs = append(ruleArgs, commonRuleArgs...)
		if err := firewallRunner.ExecuteCommand(ctx, ruleArgs...); err != nil {
			return errors.Wrap(err, "failed to block access to FakeDMS")
		}

		// Reserve 3 seconds to resume firewall settings.
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
		defer cancel()

		defer func(cleanupCtx context.Context) {
			ruleArgs := []firewall.RuleOption{firewall.OptionDeleteRule(firewall.OutputChain)}
			ruleArgs = append(ruleArgs, commonRuleArgs...)
			if err := firewallRunner.ExecuteCommand(ctx, ruleArgs...); err != nil {
				s.Fatal("Failed to restore access to FakeDMS: ", err)
			}
		}(cleanupCtx)

		cr, err := kiosk.RestartChromeWithOptions(
			ctx,
			chrome.DMSPolicy(fdms.URL),
			chrome.NoLogin(),
			chrome.KeepState(),
		)
		if err != nil {
			return errors.Wrap(err, "failed to restart Chrome")
		}

		if err := kiosk.WaitLaunchLogs(ctx); err != nil {
			return errors.Wrap(err, "failed to launch Kiosk after restarting Chrome")
		}

		if param.IsLacros {
			testing.ContextLog(ctx, "Checking if Kiosk started in Lacros mode")
			testConn, err := cr.TestAPIConn(ctx)
			_, err = lacrosproc.Root(ctx, testConn)
			if err != nil {
				return errors.Wrap(err, "failed to get lacros proc")
			}
		}
		return nil
	}

	// Launch kiosk in offline mode.
	if err := network.ExecFuncOnChromeOffline(ctx, restartAndLaunchKiosk); err != nil {
		s.Fatal("Failed to launch kiosk app offline: ", err)
	}
}
