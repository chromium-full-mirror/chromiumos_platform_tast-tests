// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConnectNotification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that a notification is shown when the user connects to cellular the first time",
		Contacts: []string{
			"cros-connectivity@google.com",
			"ejcaruso@chromium.org",
		},
		BugComponent: "b:1226026",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
	})
}

func ConnectNotification(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	const notificationID = "chrome://settings/internet/mobile_data"
	verifyNotification := func(ctx context.Context) error {
		_, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitIDContains(notificationID))
		return err
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyNotification); err != nil {
		s.Fatal("Failed to wait for notification: ", err)
	}

	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close existing notifications: ", err)
	}

	if err := ash.WaitUntilNotificationGone(ctx, tconn, 15*time.Second, ash.WaitIDContains(notificationID)); err != nil {
		s.Fatal("Cellular notification was not dismissed: ", err)
	}

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create a manager object: ", err)
	}

	s.Log("Reconnecting ethernet")
	// Wait for ethernet to ensure we are connected on something other than cellular before running
	// the second part of the test
	var props = map[string]interface{}{
		shillconst.ServicePropertyType:        shillconst.TypeEthernet,
		shillconst.ServicePropertyIsConnected: true,
	}

	_, err = m.WaitForServiceProperties(ctx, props, 10*time.Second)
	if err != nil {
		s.Fatal("Ethernet did not reconnect after being re-enabled: ", err)
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyNotification); err == nil {
		s.Fatal("Cellular notification was erroneously shown a second time in one session")
	}
}
