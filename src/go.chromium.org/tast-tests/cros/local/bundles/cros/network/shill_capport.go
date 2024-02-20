// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/capport"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillCAPPORT,
		BugComponent: "b:1166446",
		Desc:         "Set up a virtual ethernet pair with a simulated CAPPORT server and DNS server, and check whether shill enters the portal redirected state",
		Contacts: []string{
			"cros-networking@google.com", // Platform networking team
			"akahuang@google.com",        // Test author
		},
		Attr:    []string{"group:mainline", "group:hw_agnostic", "informational"},
		Fixture: "shillReset",
	})
}

func ShillCAPPORT(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Relying on shillReset test fixture to undo the enabling of portal detection.
	testing.ContextLog(ctx, "Enabling portal detection on ethernet")
	if err := m.EnablePortalDetection(ctx); err != nil {
		s.Fatal("Enable Portal Detection failed: ", err)
	}

	httpsCerts := certs.New(certs.SSLCrtPath, certificate.TestCert3())
	cleanupCerts, err := httpsCerts.InstallTestCerts(ctx)
	if err != nil {
		s.Fatal("Failed to setup certificates: ", err)
	}
	defer cleanupCerts(cleanupCtx)

	opts := virtualnet.EnvOptions{
		Priority:   5,
		NameSuffix: "",
		EnableDHCP: true,
		EnableDNS:  true,
		// TODO(b/305129516): add a test case for SLAAC.
		RAServer: false,
		// Set HTTP server to nil to make the legacy probing not working.
		HTTPServerResponseHandler:  nil,
		HTTPSServerResponseHandler: capport.HTTPSCapportHandler(ctx, capport.CapportURL, capport.UserPortalURL),
		HTTPSCerts:                 httpsCerts,
		CapportURL:                 capport.CapportURL,
	}

	pool := subnet.NewPool()
	service, portalEnv, err := virtualnet.CreateRouterEnv(ctx, m, pool, opts)
	if err != nil {
		s.Fatal("Failed to create a portal env: ", err)
	}
	defer func() {
		if err := portalEnv.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up portalEnv: ", err)
		}
	}()

	pw, err := service.CreateWatcher(ctx)
	if err != nil {
		s.Fatal("Failed to create watcher: ", err)
	}
	defer pw.Close(cleanupCtx)

	s.Log("Make service restart portal detector")
	if err := m.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var expectedServiceState = []interface{}{
		shillconst.ServiceStateRedirectFound,
	}
	s.Logf("Waiting for service state to be %q", expectedServiceState)
	if _, err := pw.ExpectIn(timeoutCtx, shillconst.ServicePropertyState, expectedServiceState); err != nil {
		s.Fatal("Service state is unexpected: ", err)
	}
}
