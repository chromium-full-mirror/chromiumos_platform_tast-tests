// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/printer/chromeprintingsvc"
	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromePrintingServicePrinterInfo,
		Desc:         "Test the ChromePrintingService getPrinters and GetPrinterInfo",
		Contacts:     []string{"project-bolton@google.com", "alepgn@google.com"},
		BugComponent: "b:167231",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.printer.ChromePrintingService"},
		Vars:         []string{"printer.targetPrinterName"},
	})
}

// ChromePrintingServicePrinterInfo test the workflow of getting the info/capabilities of a printer.
func ChromePrintingServicePrinterInfo(ctx context.Context, s *testing.State) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to initialize the RPC service on the DUT: ", err)
	}
	svc := pb.NewChromePrintingServiceClient(cl.Conn)
	defer func() {
		if _, err := svc.Close(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to close service: ", err)
		}
	}()

	targetPrinterName, _ := s.Var("printer.targetPrinterName")
	printerID, err := chromeprintingsvc.FindTargetPrinterID(ctx, svc, targetPrinterName)
	if err != nil {
		s.Fatal("Failed to call FindTargetPrinterID: ", err)
	}

	info, err := svc.GetPrinterInfo(ctx, &pb.GetPrinterInfoRequest{PrinterId: printerID})
	if err != nil {
		s.Fatal("Failed to call GetPrinterInfo: ", err)
	}
	s.Log("Info: ", info)
}
