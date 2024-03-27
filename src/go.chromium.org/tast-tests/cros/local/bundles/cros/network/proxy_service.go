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

	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
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
	s    *testing.ServiceState
	p    proxy.Proxy
	pctx context.Context // a service-scoped context used for mitmproxy
}

// StartServer starts a new proxy server instance with a specific configuration.
// This is the implementation of network.ProxyService/Start gRPC.
func (s *ProxyService) StartServer(ctx context.Context, request *network.StartServerRequest) (resp *network.StartServerResponse, retErr error) {
	var opts []proxy.Option

	var ignoreList string

	if len(request.Allowlist) > 0 {
		ignoreList = strings.Join(request.Allowlist, " \n - ")
	} else {
		ignoreList = ".*" // allow all hostnames to bypass mitmproxy cert inspection
	}

	// Create a service-scoped context for mitmproxy. The mitmproxy server is shutdown after the test is finished.
	pctx := s.s.ServiceContext() // NOLINT: the proxy server needs to continue execution in the background, after the ctx is destroyed

	// ServiceContext doesn't have OutDir associated. So, OutDir should explicitly be passed as an option to mitmproxy for saving its logs.
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return nil, errors.New("OutDir should be set in the context")
	}
	opts = append(opts,
		proxy.CustomOptions(fmt.Sprintf("ignore_hosts: \n - %s", ignoreList)),
		proxy.OutDir(outDir),
		proxy.HealthCheck(false),
	)

	p, err := proxy.NewMitmProxy(pctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start a local proxy on the DUT")
	}
	s.pctx = pctx
	s.p = p

	return &network.StartServerResponse{
		HostAndPort: s.p.ProxyAddress(),
	}, nil
}

// StopServer stops a previously started server instance. Returns an error if no proxy server instance was started on the DUT.
// This is the implementation of network.ProxyService/Stop gRPC.
func (s *ProxyService) StopServer(ctx context.Context, request *empty.Empty) (*empty.Empty, error) {
	if s.p == nil {
		return nil, errors.New("no proxy server instance was started")
	}
	if err := s.p.Close(s.pctx); err != nil {
		return nil, errors.Wrap(err, "failed to stop proxy server")
	}
	return &empty.Empty{}, nil
}
