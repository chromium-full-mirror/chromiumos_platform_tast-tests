// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	fpb "go.chromium.org/tast-tests/cros/services/cros/feedback"
	uipb "go.chromium.org/tast-tests/cros/services/cros/ui"

	"go.chromium.org/tast/core/testing"
)

const systemLogsZip = "system_logs.zip"
const systemLogsTxt = "system_logs.txt"

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserFeedbackServiceGRPC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check basic functionalities of UserFeedbackService",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

// userFeedback specifies a subset of the user feedback report schema.
// It is used to unmarshal the json string output from UserFeedbackService
// such that we can perform basic validation.
type userFeedback struct {
	WebData struct {
		ProductSpecificData []struct {
			Key   string `json:"key"`
			Value string `json:"value,omitempty"`
		} `json:"productSpecificData"`
	} `json:"webData"`
	ProductSpecificBinaryData []struct {
		Name     string `json:"name"`
		MimeType string `json:"mimeType,omitempty"`
		Data     []byte `json:"data"`
	} `json:"productSpecificBinaryData"`
}

func UserFeedbackServiceGRPC(ctx context.Context, s *testing.State) {
	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Start Chrome on the DUT.
	cs := uipb.NewChromeServiceClient(cl.Conn)
	// Specify chrome options to skip sending user feedback reports to the server.
	loginReq := &uipb.NewRequest{
		EnableFeatures: []string{"SkipSendingFeedbackReportInTastTests",
			"OsFeedbackSaveReportToLocalForE2ETesting"},
	}

	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(ctx, &empty.Empty{})

	feedbackSvc := fpb.NewUserFeedbackServiceClient(cl.Conn)
	req := &fpb.GetUserFeedbackRequest{}
	res, err := feedbackSvc.GetUserFeedback(ctx, req)
	if err != nil {
		s.Fatal("Failed to get user feedback report: ", err)
	}

	// Unmarshal json output into userFeedback struct for validation.
	var ufb userFeedback
	if err := json.Unmarshal([]byte(res.GetUserFeedback()), &ufb); err != nil {
		s.Fatal("Failed to unmarshal user feedback report json: ", err)
	}

	// Ensure that ProductSpecificData is not empty.
	if len(ufb.WebData.ProductSpecificData) == 0 {
		s.Fatal("ProduceSpecificData should not be empty")
	}

	// Ensure that ProductSpecificBinaryData is not empty.
	if len(ufb.ProductSpecificBinaryData) == 0 {
		s.Fatal("ProductSpecificBinaryData should not be empty")
	}

	// Verify user feedback report contains system_log.zip in ProductSpecificBinaryData.
	systemLogsZipExists := false
	systemLogsTxtExists := false
	var systemLogsZipBytes []byte
	for _, element := range ufb.ProductSpecificBinaryData {
		if element.Name == systemLogsZip {
			systemLogsZipExists = true
			systemLogsZipBytes = element.Data
		}
	}
	if !systemLogsZipExists {
		s.Fatalf("Failed to find %s in user feedback report", systemLogsZip)
	}

	// Ensure that system_logs.zip can be properly unzipped to provide access to system_logs.txt.
	reader := bytes.NewReader(systemLogsZipBytes)
	zipReader, err := zip.NewReader(reader, int64(len(systemLogsZipBytes)))
	if err != nil {
		s.Fatalf("Failed to read %s: %v", systemLogsZip, err)
	}
	if len(zipReader.File) == 0 {
		s.Fatalf("%s is empty", systemLogsZip)
	}

	for _, f := range zipReader.File {
		if f.Name == systemLogsTxt {
			// Note: Here are the steps to extract the system_logs.txt content.
			// rc, err := f.Open()
			// systemlogsTxtBytes, err := ioutil.ReadAll(rc)
			systemLogsTxtExists = true
		}
	}
	if !systemLogsTxtExists {
		s.Fatalf("Failed to find %s in %s", systemLogsTxt, systemLogsZip)
	}
}
