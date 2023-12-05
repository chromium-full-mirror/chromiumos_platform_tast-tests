// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/captiveportalconsts"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type params struct {
	serviceState         string
	httpResponseHandler  func(rw http.ResponseWriter, req *http.Request)
	httpsResponseHandler func(rw http.ResponseWriter, req *http.Request)
	oncSource            string
	checkPortal          bool
}

func init() {
	testing.AddTest(&testing.Test{
		BugComponent: "b:1166446",
		Func:         ShillCaptivePortalHTTP,
		Desc:         "Ensures that setting up a virtual ethernet pair with a DNS server that points portal detection queries to an http server that responds via the handler. This results in a service state of |ServiceState| via the params for the ethernet service",
		Contacts: []string{
			"cros-network-health-team@google.com", // Network Health team
			"cros-networking@google.com",          // Platform networking team
			"michaelrygiel@google.com",            // Test author
		},
		Attr:    []string{"group:mainline", "group:hw_agnostic", "informational", "group:criticalstaging"},
		Fixture: "shillReset",
		Params: []testing.Param{{
			Name: "redirectfound",
			Val: &params{
				serviceState:         shillconst.ServiceStateRedirectFound,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
			},
		}, {
			Name: "managednetworkdevicepolicy",
			Val: &params{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            shillconst.ServiceONCSourceDevicePolicy,
				checkPortal:          true,
			},
		}, {
			Name: "managednetworkuserpolicy",
			Val: &params{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            shillconst.ServiceONCSourceUserPolicy,
				checkPortal:          true,
			},
		}, {
			Name: "managednetworknone",
			Val: &params{
				serviceState:         shillconst.ServiceStateRedirectFound,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            shillconst.ServiceONCSourceNone,
				checkPortal:          true,
			},
		}, {
			Name: "checkportalfalse",
			Val: &params{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          false,
			},
		}, {

			Name: "portalsuspected",
			Val: &params{
				serviceState:         shillconst.ServiceStatePortalSuspected,
				httpResponseHandler:  captiveportalconsts.RedirectWithNoLocationHandler,
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
			},
		}, {
			Name: "online",
			Val: &params{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.NoContentHandler,
				httpsResponseHandler: captiveportalconsts.NoContentHandler,
				oncSource:            "",
				checkPortal:          true,
			},
		}, {
			Name: "noconnectivity",
			Val: &params{
				serviceState:         shillconst.ServiceStateNoConnectivity,
				httpResponseHandler:  nil,
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
			},
		}, {
			Name: "redirectfoundtempredirect",
			Val: &params{
				serviceState:         shillconst.ServiceStateRedirectFound,
				httpResponseHandler:  captiveportalconsts.TempRedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
			},
		}},
	})
}

func ShillCaptivePortalHTTP(ctx context.Context, s *testing.State) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	testing.ContextLog(ctx, "Enabling portal detection on ethernet")
	// Relying on shillReset test fixture to undo the enabling of portal detection.
	if err := m.EnablePortalDetection(ctx); err != nil {
		s.Fatal("Enable Portal Detection failed: ", err)
	}

	if err := m.SetProperty(ctx, shillconst.ManagerPropertyPortalHTTPSURL, captiveportalconsts.HTTPSPortalURL); err != nil {
		s.Fatal("Failed to set portal httpsurl: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	params := s.Param().(*params)

	var httpsCerts *certs.Certs
	if params.httpsResponseHandler != nil {
		httpsCerts = certs.New(certs.SSLCrtPath, certificate.TestCert3())
		cleanupCerts, err := httpsCerts.InstallTestCerts(ctx)
		if err != nil {
			s.Fatal("Failed to setup certificates: ", err)
		}
		defer cleanupCerts(cleanupCtx)
	}

	opts := virtualnet.EnvOptions{
		Priority:                   5,
		NameSuffix:                 "",
		EnableDHCP:                 true,
		EnableDNS:                  true,
		RAServer:                   false,
		HTTPSServerResponseHandler: params.httpsResponseHandler,
		HTTPServerResponseHandler:  params.httpResponseHandler,
		HTTPSCerts:                 httpsCerts,
	}
	pool := subnet.NewPool()
	service, portalEnv, err := virtualnet.CreateRouterEnv(ctx, m, pool, opts)
	if err != nil {
		s.Fatal("Failed to create a portal env: ", err)
	}
	defer portalEnv.Cleanup(cleanupCtx)

	pw, err := service.CreateWatcher(ctx)
	if err != nil {
		s.Fatal("Failed to create watcher: ", err)
	}
	defer pw.Close(cleanupCtx)

	// Set oncSource property on service. For UserPolicy and DevicePolicy, shill will skip portal detection.
	// For no policy, shill will do portal detection.
	if params.oncSource != "" {
		if err := service.SetProperty(ctx, shillconst.ServicePropertyONCSource, params.oncSource); err != nil {
			s.Fatal("Failed to set ServicePropertyONCSource: ", err)
		}
	}

	// Set checkPortal on service. If |false|, shill will skip portal detection.
	if !params.checkPortal {
		if err := service.SetProperty(ctx, shillconst.ServicePropertyCheckPortal, "false"); err != nil {
			s.Fatal("Failed to invoke CheckPortal service: ", err)
		}
	}

	s.Log("Make service restart portal detector")
	if err := m.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	s.Logf("Check if service state is %q", params.serviceState)
	var expectedServiceState = []interface{}{
		params.serviceState,
	}
	_, err = pw.ExpectIn(timeoutCtx, shillconst.ServicePropertyState, expectedServiceState)
	if err != nil {
		s.Fatal("Service state is unexpected: ", err)
	}
}
