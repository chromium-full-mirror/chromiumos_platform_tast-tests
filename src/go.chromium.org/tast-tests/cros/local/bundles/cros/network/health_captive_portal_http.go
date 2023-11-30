// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net/http"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/captiveportalconsts"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/health"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type healthCaptivePortalHTTPParams struct {
	serviceState         string
	httpResponseHandler  func(rw http.ResponseWriter, req *http.Request)
	httpsResponseHandler func(rw http.ResponseWriter, req *http.Request)
	oncSource            string
	checkPortal          bool
	networkState         string
	portalState          string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HealthCaptivePortalHTTP,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Ensures that captive portal-related Service state changes are propogated to the Networks obtained by the NetworkHealth API",
		Contacts: []string{
			"cros-network-health-team@google.com", // network-health team
			"khegde@chromium.org",                 // test maintainer
			"stevenjb@chromium.org",               // network-health tech lead
		},
		BugComponent: "b:1166446",
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "shillReset",
		Params: []testing.Param{{
			Name: "redirectfound",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateRedirectFound,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStatePortal,
				portalState:          health.PortalStatePortal,
			},
		}, {
			Name: "managednetworkdevicepolicy",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            shillconst.ServiceONCSourceDevicePolicy,
				checkPortal:          true,
				networkState:         health.NetworkStateOnline,
				portalState:          health.PortalStateOnline,
			},
		}, {
			Name: "managednetworkuserpolicy",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            shillconst.ServiceONCSourceUserPolicy,
				checkPortal:          true,
				networkState:         health.NetworkStateOnline,
				portalState:          health.PortalStateOnline,
			},
		}, {
			Name: "managednetworknone",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateRedirectFound,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            shillconst.ServiceONCSourceNone,
				checkPortal:          true,
				networkState:         health.NetworkStatePortal,
				portalState:          health.PortalStatePortal,
			},
		}, {
			Name: "checkportalfalse",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          false,
				networkState:         health.NetworkStateOnline,
				portalState:          health.PortalStateOnline,
			},
		}, {
			Name: "portalsuspected",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStatePortalSuspected,
				httpResponseHandler:  captiveportalconsts.OkResponseHandler("portal login page"),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStatePortal,
				// Chrome portal detection will override PortalSuspected with Portal
				// in this cases. TODO(b/292141089): Fix the Chrome behavior.
				portalState: health.PortalStatePortal,
			},
		}, {
			Name: "online",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.NoContentHandler,
				httpsResponseHandler: captiveportalconsts.NoContentHandler,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStateOnline,
				portalState:          health.PortalStateOnline,
			},
		}, {
			Name: "noconnectivity",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateNoConnectivity,
				httpResponseHandler:  nil,
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStatePortal,
				portalState:          health.PortalStateNoInternet,
			},
		}, {
			Name: "redirectfoundtempredirect",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateRedirectFound,
				httpResponseHandler:  captiveportalconsts.TempRedirectHandler(captiveportalconsts.RedirectURL),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStatePortal,
				portalState:          health.PortalStatePortal,
			},
		}, {
			Name: "invalidredirect",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateNoConnectivity,
				httpResponseHandler:  captiveportalconsts.RedirectHandler(""),
				httpsResponseHandler: nil,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStatePortal,
				portalState:          health.PortalStateNoInternet,
			},
		}, {
			Name: "empty200answer",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.OkResponseHandler(""),
				httpsResponseHandler: captiveportalconsts.NoContentHandler,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStateOnline,
				portalState:          health.PortalStateOnline,
			},
		}, {
			Name: "1byte200answer",
			Val: &healthCaptivePortalHTTPParams{
				serviceState:         shillconst.ServiceStateOnline,
				httpResponseHandler:  captiveportalconsts.OkResponseHandler("\n"),
				httpsResponseHandler: captiveportalconsts.NoContentHandler,
				oncSource:            "",
				checkPortal:          true,
				networkState:         health.NetworkStateOnline,
				portalState:          health.PortalStateOnline,
			},
		}},
	})
}

func HealthCaptivePortalHTTP(ctx context.Context, s *testing.State) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Disable the physical ethernet so that the only Ethernet service
	// available is the veth service created below.
	if enableFunc, err := m.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Unable to disable Ethernet: ", err)
	} else if enableFunc != nil {
		newCtx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime)
		defer cancel()
		defer enableFunc(ctx)
		ctx = newCtx
	}

	if enabled, err := m.IsEnabled(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Error calling IsEnabled: ", err)
	} else if enabled {
		s.Fatal("Ethernet is still enabled")
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

	params := s.Param().(*healthCaptivePortalHTTPParams)

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

	if err := m.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var expectedServiceState = []interface{}{
		params.serviceState,
	}
	if _, err = pw.ExpectIn(timeoutCtx, shillconst.ServicePropertyState, expectedServiceState); err != nil {
		s.Fatal("Service state is unexpected: ", err)
	}

	services, err := m.Services(ctx)
	if err != nil {
		s.Fatal("Failed to get list of services: ", err)
	}
	if len(services) == 0 {
		s.Fatal("Failed to get non-zero list of services")
	}

	// The default service is guaranteed to to be first in Shill.
	defaultService := services[0]
	if !reflect.DeepEqual(defaultService, service) {
		s.Fatalf("Unexpected default service, got: %v, want: %v", defaultService, service)
	}
	name, err := defaultService.GetName(ctx)
	if err != nil {
		s.Fatal("Failed to get name of default shill service: ", err)
	}
	sType, err := defaultService.GetType(ctx)
	if err != nil {
		s.Fatalf("Failed to get type for %v: %v", name, err)
	}
	guid, err := defaultService.GetGUID(ctx)
	if err != nil {
		s.Fatalf("Failed to get GUID for %v: %v", name, err)
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	netConn, err := health.CreateLoggedInNetworkHealth(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer netConn.Close(cleanupCtx)

	networks, err := netConn.GetNetworkList(ctx, s)
	if err != nil {
		s.Fatal("Failed to run GetNetworkList: ", err)
	}
	network, err := health.FindMatchingNetwork(networks, sType, guid)
	if err != nil {
		s.Fatalf("Network %s not found: %v", name, err)
	}

	if network.State.String() != params.networkState {
		s.Fatalf("Failed to get correct network state, got: %v, want: %v",
			network.State.String(),
			params.networkState)
	}

	if network.PortalState.String() != params.portalState {
		s.Fatalf("Failed to get correct portal state, got: %v, want: %v",
			network.PortalState.String(),
			params.portalState)
	}
}
