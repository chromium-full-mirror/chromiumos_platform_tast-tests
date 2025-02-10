// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/peripherals/smartcard"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/peripherals/utils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SmartCard,
		Desc: "Set up the smart card device and perform smart card related tests",
		Contacts: []string{
			"cros-ent-peripherals-team@google.com",
			"rzakarian@google.com",
			"sudhirperka@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:885494", // ChromeOS > Platform > Enablement > Services > Peripherals
		Timeout:      10 * time.Minute,
		ServiceDeps: []string{
			"tast.cros.peripherals.PeriphService",
			utils.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"peripherals.manual_test",
			"peripherals.smart_card_username1",
			"peripherals.smart_card_pin_code1",
			"peripherals.smart_card_username2",
			"peripherals.smart_card_pin_code2",
		},
		Fixture: fixture.CleanOwnership,
	})
}

func SmartCard(ctx context.Context, s *testing.State) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect the DUT: ", err)
	}
	defer cl.Close(ctx)

	manualTest := false
	if val, ok := s.Var("peripherals.manual_test"); ok {
		manualTest, err = strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Failed to parse argument 'peripherals.manual_test' of type bool: ", err)
		}
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	username1 := s.RequiredVar("peripherals.smart_card_username1")
	pinCode1 := s.RequiredVar("peripherals.smart_card_pin_code1")
	account1 := smartcard.Account{
		Username: username1,
		PinCode:  pinCode1,
	}

	username2 := s.RequiredVar("peripherals.smart_card_username2")
	pinCode2 := s.RequiredVar("peripherals.smart_card_pin_code2")
	account2 := smartcard.Account{
		Username: username2,
		PinCode:  pinCode2,
	}

	sc, err := smartcard.New(ctx, cl, manualTest, account1, account2)
	if err != nil {
		s.Fatal("Failed to create smart card: ", err)
	}
	defer func(ctx context.Context) {
		utils.DumpUITreeWithScreenshotToFile(ctx, cl.Conn, s.HasError, "ui_dump")
		if err := sc.CleanupSmartCard(ctx); err != nil {
			s.Log("Failed to cleanup smart card: ", err)
		}
	}(cleanupCtx)

	if err := action.Combine("test smart card",
		sc.FirstTimeLogin(),
		sc.AddNewUser(),
		sc.ReAuthenticationOnline(),
		sc.ReAuthenticationOffline(),
		sc.UnlockScreenWithSmartCardOnline(),
		sc.UnlockScreenWithSmartCardOffline(),
		sc.AutomaticLockOnSmartCardRemoval(),
		sc.AutomaticLogoutOnSmartCardRemoval(),
		sc.WebApplicationAuthentication(),
		sc.ReAuthenticationOnline(),
		sc.DriveLockCSSIAppTesting(),
		sc.VerifyCertificate(),
	)(ctx); err != nil {
		s.Fatal("Failed to test smart card: ", err)
	}
}
