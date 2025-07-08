// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"

	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromePrintingService,
		Desc:         "Test the gRPC Tast Service ChromePrintingService",
		Contacts:     []string{"project-bolton@google.com", "alepgn@google.com"},
		BugComponent: "b:430578866",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.printer.ChromePrintingService"},
	})
}

// ChromePrintingService test all methods implemneted by the tast gRPC service 'ChromePrintingService'
func ChromePrintingService(ctx context.Context, s *testing.State) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to initialize the RPC service on the DUT: ", err)
	}

	svc := pb.NewChromePrintingServiceClient(cl.Conn)
	req := &emptypb.Empty{}
	printers, err := svc.GetPrinters(ctx, req)
	if err != nil {
		s.Fatal("Failed to call DiscoverPrinters: ", err)
	}
	if _, err := svc.Close(ctx, req); err != nil {
		s.Fatal("Failed to close service: ", err)
	}
	s.Log("Printers: ", printers)
}
