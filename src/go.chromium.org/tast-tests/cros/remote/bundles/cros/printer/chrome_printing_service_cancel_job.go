// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"encoding/base64"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/printer/chromeprintingsvc"
	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromePrintingServiceCancelJob,
		Desc:         "Test the ChromePrintingService submit job and canceling a job workflow",
		Contacts:     []string{"project-bolton@google.com", "alepgn@google.com"},
		BugComponent: "b:167231",
		Data:         []string{cancelAndroidPDF},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.printer.ChromePrintingService"},
		Vars:         []string{"printer.targetPrinterName"},
	})
}

const cancelAndroidPDF = "android.pdf"

// ChromePrintingServiceCancelJob test the workflow of looking for a printer, submitting a job,
// canceling the job and tracking the jobStatus until it is cancelled.
func ChromePrintingServiceCancelJob(ctx context.Context, s *testing.State) {
	androidPDFPath := s.DataPath(cancelAndroidPDF)
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

	targetPrinterName, _ := s.Var("printer.targetPrinterName")
	printerID, err := chromeprintingsvc.FindTargetPrinterID(ctx, svc, targetPrinterName)
	if err != nil {
		s.Fatal("Failed to call FindTargetPrinterID: ", err)
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

	submitJobResp, err := svc.SubmitJob(ctx, submitJobRequest)
	if err != nil {
		s.Fatal("Failed to call submitJob: ", err)
	}

	s.Log("jobId: ", submitJobResp.JobId)
	// Get the initial job status
	getJobStatusReq := &pb.GetJobStatusRequest{JobId: submitJobResp.JobId}
	initialStatusResponse, err := svc.GetJobStatus(ctx, getJobStatusReq)
	if err != nil {
		s.Fatal("Failed to call getJobStatus: ", err)
	}
	s.Log("Initial Job Status: ", initialStatusResponse.Status)

	// Cancel the job
	if _, err := svc.CancelJob(ctx, &pb.CancelJobRequest{JobId: submitJobResp.JobId}); err != nil {
		s.Fatal("Failed to call cancelJob: ", err)
	}
	// Track the status until it changes to canceled or times out.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		getJobStatusResp, err := svc.GetJobStatus(ctx, getJobStatusReq)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to call getJobStatus"))
		}
		s.Log("JobStatus: ", getJobStatusResp.Status)
		if getJobStatusResp.Status == pb.JobStatus_CANCELED {
			return nil
		}

		return errors.New("job status is not canceled")
	}, &testing.PollOptions{
		// Arbitrary, if the job takes more than one minute we can assume it's stuck.
		Timeout:  1 * time.Minute,
		Interval: 3 * time.Second,
	}); err != nil {
		s.Fatal("Failed to cancel job: ", err)
	}
}
