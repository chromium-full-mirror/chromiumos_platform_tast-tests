// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"sync"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	var chromePrintingService ChromePrintingService
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			chromePrintingService = ChromePrintingService{
				sharedObject: common.SharedObjectsForServiceSingleton}
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
