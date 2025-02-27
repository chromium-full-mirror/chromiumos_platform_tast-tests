// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package regmon

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Regmond,
		Desc:         "Integration test for Regmon D-Bus daemon",
		BugComponent: "b:1129862", // DPChromeOS > DPChromeOS Engineering
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		Attr: []string{
			"group:criticalstaging",
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{"chrome", "amd64"},
		Timeout:      1 * time.Minute,
	})
}

// Regmond tests the regmon D-Bus daemon to verify that it runs and returns a success response.
func Regmond(ctx context.Context, s *testing.State) {
	const (
		// regmond D-Bus service info. Refer to:
		//     third_party/cros_system_api/dbus/regmon/dbus-constants.h
		dbusServiceName   = "org.chromium.Regmond"
		dbusInterfaceName = "org.chromium.Regmond"
		dbusPath          = "/org/chromium/Regmond"

		// regmond D-Bus APIs.
		dbusMethodName = "RecordPolicyViolation"
	)

	/*
		Example request bytes for the RecordPolicyViolation API, representing the following proto contents:
			violation {
				annotation_hash: 88863520
				destination: "google.com"
				policy: POLICY_UNSPECIFIED
			}

		In the future, we could utilize the proto itself by creating an ebuild for the regmon proto and including that
		in tast-build-deps. For now this should be sufficient, considering this API is very simple and unlikely to see
		much change in the future.
	*/
	requestBytes := []byte{10, 19, 8, 160, 230, 175, 42, 18, 10, 103, 111, 111, 103, 108, 101, 46, 99, 111, 109, 24, 0}
	expectedSuccessResponse := []byte{10, 00}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	opts := []chrome.Option{
		chrome.NoLogin(),
		// Enable regmond feature flag.
		chrome.ExtraArgs("--enable-features=CrOSLateBootRegmonPolicyMonitoringEnabled"),
	}

	// Start Chrome.
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Create system bus, D-Bus connection, and then make the D-Bus call to regmond. Note that we use this instead of
	// `dbusutil.NewDBusObject()` because that API fails due to a check which verifies that the D-Bus service is
	// already running. Since regmond uses lazy startup, and is called very infrequently, we need to make the D-Bus
	// call ourselves in order to start up the daemon.
	var regmondResponse []byte
	conn, err := dbusutil.SystemBus()
	if err != nil {
		s.Fatal("Failed to connect to system bus: ", err)
	}
	dbusObj := conn.Object(dbusServiceName, dbusPath)
	if err := dbusObj.CallWithContext(ctx, dbusutil.BuildIfacePath(dbusInterfaceName, dbusMethodName), 0, requestBytes).Store(&regmondResponse); err != nil {
		s.Fatal("Regmon D-Bus call failed: ", err)
	}

	// Verify D-Bus response.
	if !bytes.Equal(regmondResponse, expectedSuccessResponse) {
		s.Fatalf("Regmond response got = % x, want = % x", regmondResponse, expectedSuccessResponse)
	}
}
