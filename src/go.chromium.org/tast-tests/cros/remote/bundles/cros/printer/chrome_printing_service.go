// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"go.chromium.org/tast/core/errors"
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
		Data:         []string{androidPDF},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.printer.ChromePrintingService"},
		Vars:         []string{"printer.targetPrinterName"},
	})
}

const androidPDF = "android.pdf"

// ChromePrintingService test all methods implemented by the tast gRPC service 'ChromePrintingService'.
func ChromePrintingService(ctx context.Context, s *testing.State) {
	androidPDFPath := s.DataPath(androidPDF)
	androidPDFBytes, err := os.ReadFile(androidPDFPath)
	if err != nil {
		s.Fatal("Failed to read android PDF")
	}
	base64AndroidPDF := base64.StdEncoding.EncodeToString(androidPDFBytes)

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

	// Initial printers to log in case printer.targetPrinterName is empty.
	initialPrinters, err := svc.GetPrinters(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to call GetPrinters: ", err)
	}
	// Get the target printer name from tast Vars.
	targetPrinterName, ok := s.Var("printer.targetPrinterName")
	if !ok || targetPrinterName == "" {
		// Format the printer name and URI.
		var printerDetails []string
		for _, p := range initialPrinters.Printers {
			printerDetails = append(printerDetails, fmt.Sprintf("Name: %q, URI: %q", p.Name, p.Uri))
		}

		s.Fatalf("Parameter 'printer.targetPrinterName' is missing. Available printers: %s", "\n"+strings.Join(printerDetails, "\n"))
	}

	emptyReq := &emptypb.Empty{}
	var printers *pb.GetPrintersResponse
	var printerID string

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		printers, err = svc.GetPrinters(ctx, emptyReq)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to call getPrinters"))
		}

		for _, p := range printers.Printers {
			s.Log("Checking printer: ", p)
			if strings.Contains(p.Name, targetPrinterName) {
				s.Log("Found target printer")
				printerID = p.Id
				return nil
			}
		}
		return errors.New("target Printer not discovered")
	}, &testing.PollOptions{
		// Arbitrary, enough time for tast to initiate and start looking for printers.
		Timeout:  100 * time.Second,
		Interval: 5 * time.Second,
	}); err != nil {
		s.Fatal("Failed to find printer '", targetPrinterName, "': ", err)
	}

	submitJobRequest := &pb.SubmitJobRequest{
		Job: &pb.Job{
			PrinterId: printerID,
			Title:     "remoteTest",
			Document:  base64AndroidPDF,
			Ticket: &pb.Ticket{
				Version: "1.0",
				Print: &pb.Print{
					MediaSize: &pb.MediaSize{
						HeightMicrons: 279400,
						WidthMicrons:  215900,
					},
					Copies:          &pb.Copies{Copies: 1},
					PageOrientation: &pb.PageOrientation{Type: pb.PageOrientation_PORTRAIT.Enum()},
					Color:           &pb.Color{Type: pb.Color_STANDARD_MONOCHROME.Enum()},
					FitToPage:       &pb.FitToPage{Type: pb.FitToPage_AUTO.Enum()},
					Duplex:          &pb.Duplex{Type: pb.Duplex_NO_DUPLEX.Enum()},
					Dpi:             &pb.Dpi{HorizontalDpi: 600, VerticalDpi: 600},
					Collate:         &pb.Collate{Collate: new(bool)},
				},
			},
			ContentType: "application/pdf",
		},
	}

	resp, err := svc.SubmitJob(ctx, submitJobRequest)
	if err != nil {
		s.Fatal("Failed to call submitJob: ", err)
	}

	s.Log("jobId: ", resp.JobId)
}
