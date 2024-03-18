// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome/proxy/mitmproxy"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			network.RegisterProxyServiceServer(srv, &ProxyService{s: s})
		},
	})
}

// ProxyService implements the tast.cros.network.ProxyService gRPC service.
type ProxyService struct {
	s     *testing.ServiceState
	proxy *mitmproxy.MitmProxy
}

// StartServer starts a new proxy server instance with a specific configuration.
// This is the implementation of network.ProxyService/Start gRPC.
func (s *ProxyService) StartServer(ctx context.Context, request *network.StartServerRequest) (resp *network.StartServerResponse, retErr error) {
	var opts []mitmproxy.Option

	var ignoreList string

	if len(request.Allowlist) > 0 {
		ignoreList = strings.Join(request.Allowlist, " \n - ")
	} else {
		ignoreList = ".*" // allow all hostnames to bypass mitmproxy cert inspection
	}

	opts = append(opts, mitmproxy.CustomOptions(fmt.Sprintf("ignore_hosts: \n - %s", ignoreList)),
		mitmproxy.HealthCheck(false))

	proxy, err := mitmproxy.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a local proxy on the DUT")
	}

	// Create a service-scoped context for mitmproxy. The mitmproxy server is shutdown after the test is finished.
	pctx := context.Background() // NOLINT: the proxy server needs to continue execution in the background, after the ctx is destroyed

	if err := proxy.Start(pctx); err != nil {
		proxy.Close(pctx)
		return nil, errors.Wrap(err, "failed to create a local proxy on the DUT")
	}

	s.proxy = proxy

	return &network.StartServerResponse{
		HostAndPort: s.proxy.ProxyAddress(),
	}, nil
}

// StopServer stops a previously started server instance. Returns an error if no proxy server instance was started on the DUT.
// This is the implementation of network.ProxyService/Stop gRPC.
func (s *ProxyService) StopServer(ctx context.Context, request *empty.Empty) (*empty.Empty, error) {
	if s.proxy == nil {
		return nil, errors.New("no proxy server instance was started")
	}

	if err := s.proxy.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to stop proxy server")
	}
	return &empty.Empty{}, nil
}
