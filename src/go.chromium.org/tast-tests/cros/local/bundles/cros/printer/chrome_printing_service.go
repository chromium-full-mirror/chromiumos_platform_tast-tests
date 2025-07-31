// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"fmt"
	"sync"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	var chromePrintingService ChromePrintingService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			chromePrintingService = ChromePrintingService{
				s:            s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			}
			pb.RegisterChromePrintingServiceServer(srv, &chromePrintingService)
		},
		// GuaranteeCompatibility allows non-Tast test harness clients to call this service.
		GuaranteeCompatibility: true,
	})
}

type ChromePrintingService struct {
	sharedObject *common.SharedObjectsForService
	cr           *chrome.Chrome
	tconn        *chrome.TestConn
	mutex        sync.Mutex
	s            *testing.ServiceState
}

// initializeChrome initializes the Chrome instance and the test API connection.
// It ensures that the init is performed only once. If the Chrome instance and
// the test API connection are already initialized this function returns immediately.
func (svc *ChromePrintingService) initializeChrome(ctx context.Context) error {
	svc.mutex.Lock()
	defer svc.mutex.Unlock()

	if svc.cr != nil && svc.tconn != nil {
		return nil // Already initialized
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Chrome")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		cr.Close(ctx)
		return errors.Wrap(err, "failed to connect to test API extension")
	}
	svc.cr = cr
	svc.tconn = tconn
	svc.s.Log("Initialized chrome and tconn")

	// Add extension to allowlist to eliminate popup that appears when calling chrome.printing.submitJob.
	if err := svc.tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", "printing.printing_api_extensions_whitelist", []string{chrome.TestExtensionID}); err != nil {
		return errors.Wrap(err, "failed to set printing.printing_api_extensions_whitelist")
	}

	return nil
}

func (svc *ChromePrintingService) getSubmitJobScript(req *pb.SubmitJobRequest) (string, error) {
	m := protojson.MarshalOptions{UseProtoNames: true}
	jobBytes, err := m.Marshal(req.Job)
	if err != nil {
		return "", err
	}

	jobJSON := string(jobBytes)
	// TODO (b/435280673): add support for submitting other contentTypes.
	return fmt.Sprintf(`
		const job = JSON.parse('%s');
		// The chrome.printing API requires the document field to be a blob of contentType pdf or png.
		job.document = new Blob(
			[
				new Uint8Array(
				atob(job.document)
					.split('')
					.map(char => char.charCodeAt(0))
				),
			],
			{ type: 'application/pdf' }
		);
		tast.promisify(chrome.printing.submitJob)({job:job})`, jobJSON), nil
}

func (svc *ChromePrintingService) validateSubmitJobRequest(req *pb.SubmitJobRequest) error {
	if req == nil {
		return errors.New("invalid request: request is missing")
	}
	if req.GetJob() == nil {
		return errors.New("invalid request: Job is missing")
	}
	job := req.Job
	if job.GetPrinterId() == "" {
		return errors.New("invalid request: PrinterId is required")
	}
	if job.GetTitle() == "" {
		return errors.New("invalid request: Title is required")
	}
	if job.GetDocument() == "" {
		return errors.New("invalid request: Document is empty")
	}
	if job.GetTicket() == nil || job.GetTicket().GetPrint() == nil {
		return errors.New("invalid request: Print ticket is missing")
	}
	if job.GetTicket().GetVersion() == "" {
		return errors.New("invalid request: ticket.version is required")
	}

	// Validate the required fields within the print ticket
	printTicket := job.GetTicket().GetPrint()
	if printTicket.GetMediaSize() == nil {
		return errors.New("invalid ticket: MediaSize is required")
	}
	if printTicket.GetCopies() == nil {
		return errors.New("invalid ticket: Copies is required")
	}
	if printTicket.GetCopies().Copies <= 0 {
		return errors.New("invalid copies: copies must be greater than zero")
	}
	if printTicket.GetPageOrientation() == nil {
		return errors.New("invalid ticket: PageOrientation is required")
	}
	if printTicket.GetColor() == nil {
		return errors.New("invalid ticket: Color is required")
	}
	if printTicket.GetCollate() == nil {
		return errors.New("invalid ticket: Collate is required")
	}
	if printTicket.GetDuplex() == nil {
		return errors.New("invalid ticket: Duplex is required")
	}
	if printTicket.GetDpi() == nil {
		return errors.New("invalid ticket: Dpi is required")
	}

	return nil
}

// Close releases the resources held by the ChromePrintingService. Closes
// the Chrome instance.
func (svc *ChromePrintingService) Close(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	svc.mutex.Lock()
	defer svc.mutex.Unlock()
	if svc.cr == nil {
		return &emptypb.Empty{}, errors.New("failed to close chrome instance, not initalized")
	}

	svc.cr.Close(ctx)
	svc.cr = nil
	svc.tconn = nil

	return &emptypb.Empty{}, nil
}

func (svc *ChromePrintingService) GetPrinters(ctx context.Context, _ *emptypb.Empty) (*pb.GetPrintersResponse, error) {
	if err := svc.initializeChrome(ctx); err != nil {
		return nil, err
	}

	var printersStruct []struct {
		Description      string
		ID               string
		IsDefault        bool
		Name             string
		RecentlyUsedRank int32
		Source           string
		URI              string
	}
	if err := svc.tconn.Call(ctx, &printersStruct, "tast.promisify(chrome.printing.getPrinters)"); err != nil {
		return nil, errors.Wrap(err, "failed to call getPrinters")
	}
	pbPrinters := make([]*pb.Printer, len(printersStruct))
	for i, p := range printersStruct {
		pbPrinters[i] = &pb.Printer{
			Description:      p.Description,
			Id:               p.ID,
			IsDefault:        p.IsDefault,
			Name:             p.Name,
			RecentlyUsedRank: p.RecentlyUsedRank,
			Source:           p.Source,
			Uri:              p.URI,
		}
	}

	return &pb.GetPrintersResponse{Printers: pbPrinters}, nil
}

func (svc *ChromePrintingService) SubmitJob(ctx context.Context, req *pb.SubmitJobRequest) (*pb.SubmitJobResponse, error) {
	if err := svc.validateSubmitJobRequest(req); err != nil {
		return nil, errors.Wrap(err, "invalid submit job request")
	}
	svc.s.Log("request: ", req)

	submitJobScript, err := svc.getSubmitJobScript(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal the job request")
	}

	var jobResult struct {
		JobID  string
		Status string
	}
	svc.s.Log("code: ", submitJobScript)
	if err := svc.tconn.Eval(ctx, submitJobScript, &jobResult); err != nil {
		return nil, errors.Wrap(err, "failed to call submitJob")
	}
	if jobResult.Status != "OK" {
		return nil, errors.Wrap(errors.New(jobResult.Status), "unexpected status")
	}
	if len(jobResult.JobID) == 0 {
		return nil, errors.New("empty JobID")
	}

	return &pb.SubmitJobResponse{JobId: jobResult.JobID}, nil
}

func (svc *ChromePrintingService) GetJobStatus(ctx context.Context, req *pb.GetJobStatusRequest) (*pb.GetJobStatusResponse, error) {
	var jobStatus string
	if err := svc.tconn.Call(ctx, &jobStatus, "tast.promisify(chrome.printing.getJobStatus)", req.JobId); err != nil {
		return nil, errors.Wrap(err, "failed to call getJobStatus")
	}

	var jobStatusStringToEnum = map[string]pb.JobStatus{
		"PENDING":     pb.JobStatus_PENDING,
		"IN_PROGRESS": pb.JobStatus_IN_PROGRESS,
		"FAILED":      pb.JobStatus_FAILED,
		"CANCELED":    pb.JobStatus_CANCELED,
		"PRINTED":     pb.JobStatus_PRINTED,
	}

	statusEnum, ok := jobStatusStringToEnum[jobStatus]
	if !ok {
		return nil, errors.Wrap(errors.New(jobStatus), "unexpected job status")
	}

	svc.s.Log("jobStatus: ", statusEnum)
	return &pb.GetJobStatusResponse{Status: statusEnum}, nil
}
