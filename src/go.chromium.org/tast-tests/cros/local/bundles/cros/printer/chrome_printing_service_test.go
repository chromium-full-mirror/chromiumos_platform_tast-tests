// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"encoding/json"
	"strings"
	"testing"

	pb "go.chromium.org/tast-tests/cros/services/cros/printer"
	"google.golang.org/protobuf/encoding/protojson"
)

func newValidSubmitJobRequest() *pb.SubmitJobRequest {
	return &pb.SubmitJobRequest{
		Job: &pb.Job{
			PrinterId: "test-printer-id",
			Title:     "Test Document",
			Document:  "dGVzdCBkb2M=", // base64 for "test doc"
			Ticket: &pb.Ticket{
				Version: "1.0",
				Print: &pb.Print{
					MediaSize:       &pb.MediaSize{HeightMicrons: 279400, WidthMicrons: 215900},
					Copies:          &pb.Copies{Copies: 1},
					PageOrientation: &pb.PageOrientation{Type: pb.PageOrientation_PORTRAIT.Enum()},
					Color:           &pb.Color{Type: pb.Color_STANDARD_COLOR.Enum()},
					Collate:         &pb.Collate{Collate: new(bool)},
					Duplex:          &pb.Duplex{Type: pb.Duplex_NO_DUPLEX.Enum()},
					Dpi:             &pb.Dpi{HorizontalDpi: 450, VerticalDpi: 600},
				},
			},
		},
	}
}

func TestValidateSubmitJobRequest(t *testing.T) {
	// Base valid request
	validReq := newValidSubmitJobRequest()

	// Test cases
	testCases := []struct {
		name      string
		req       *pb.SubmitJobRequest
		expectErr bool
	}{
		{"valid request", validReq, false},
		{"nil request", nil, true},
		{"nil job", &pb.SubmitJobRequest{}, true},
		{"missing printer ID", func() *pb.SubmitJobRequest { r := newValidSubmitJobRequest(); r.Job.PrinterId = ""; return r }(), true},
		{"missing title", func() *pb.SubmitJobRequest { r := newValidSubmitJobRequest(); r.Job.Title = ""; return r }(), true},
		{"missing document", func() *pb.SubmitJobRequest { r := newValidSubmitJobRequest(); r.Job.Document = ""; return r }(), true},
		{"missing print ticket", func() *pb.SubmitJobRequest { r := newValidSubmitJobRequest(); r.Job.Ticket.Print = nil; return r }(), true},
		{"missing media size", func() *pb.SubmitJobRequest {
			r := newValidSubmitJobRequest()
			r.Job.Ticket.Print.MediaSize = nil
			return r
		}(), true},
	}

	svc := &ChromePrintingService{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.validateSubmitJobRequest(tc.req)
			if (err != nil) != tc.expectErr {
				t.Errorf("validateSubmitJobRequest() error = %v, wantErr %v", err, tc.expectErr)
			}
		})
	}
}

func TestGetSubmitJobScript(t *testing.T) {
	svc := &ChromePrintingService{}
	m := protojson.MarshalOptions{UseProtoNames: true}

	t.Run("valid request contains all expected JSON keys in script", func(t *testing.T) {
		req := newValidSubmitJobRequest()

		script, err := svc.getSubmitJobScript(req)
		if err != nil {
			t.Fatalf("getSubmitJobScript() returned an unexpected error: %v", err)
		}

		expectedBytes, err := m.Marshal(req.Job)
		if err != nil {
			t.Fatalf("Failed to marshal source proto job: %v", err)
		}
		var expectedMap map[string]interface{}
		if err := json.Unmarshal(expectedBytes, &expectedMap); err != nil {
			t.Fatalf("Failed to unmarshal expected JSON: %v", err)
		}

		// Loop through the expected keys and check for their presence in the full script.
		for key := range expectedMap {
			// Construct the substring to search for, e.g., `"title":`
			checkStr := `"` + key + `":`
			t.Log(checkStr, strings.Contains(script, checkStr))
			if !strings.Contains(script, checkStr) {
				t.Errorf("generated script is missing expected key: %s", key)
			}
		}
	})

}

func TestGetSubmitJobScriptTicketFields(t *testing.T) {
	svc := &ChromePrintingService{}

	// Define test cases for various enum values.
	testCases := []struct {
		name             string
		modifyRequest    func(req *pb.SubmitJobRequest) // Function to modify the base request.
		expectedSnippets []string                       // Expected JSON snippet in the script.
	}{
		{
			name: "Copies value",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.Copies = &pb.Copies{Copies: 5}
			},
			expectedSnippets: []string{`"copies":{"copies":5}`},
		},
		{
			name: "Collate true value",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				b := true
				req.Job.Ticket.Print.Collate = &pb.Collate{Collate: &b}
			},
			expectedSnippets: []string{`"collate":{"collate":true}`},
		},
		{
			name: "Collate false value",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				b := false
				req.Job.Ticket.Print.Collate = &pb.Collate{Collate: &b}
			},
			expectedSnippets: []string{`"collate":{"collate":false}`},
		},
		{
			name: "DPI value",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.Dpi = &pb.Dpi{HorizontalDpi: 320, VerticalDpi: 300}
			},
			expectedSnippets: []string{
				`"dpi"`,
				`"horizontal_dpi":320`,
				`"vertical_dpi":300`,
			},
		},
		// Enums test cases
		{
			name: "Portrait Orientation",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.PageOrientation.Type = pb.PageOrientation_PORTRAIT.Enum()
			},
			expectedSnippets: []string{`"page_orientation":{"type":"PORTRAIT"}`},
		},
		{
			name: "Landscape Orientation",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.PageOrientation.Type = pb.PageOrientation_LANDSCAPE.Enum()
			},
			expectedSnippets: []string{`"page_orientation":{"type":"LANDSCAPE"}`},
		},
		{
			name: "Standard Color",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.Color.Type = pb.Color_STANDARD_COLOR.Enum()
			},
			expectedSnippets: []string{`"color":{"type":"STANDARD_COLOR"}`},
		},
		{
			name: "Monochrome Color",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.Color.Type = pb.Color_STANDARD_MONOCHROME.Enum()
			},
			expectedSnippets: []string{`"color":{"type":"STANDARD_MONOCHROME"}`},
		},
		{
			name: "Long Edge Duplex",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.Duplex.Type = pb.Duplex_LONG_EDGE.Enum()
			},
			expectedSnippets: []string{`"duplex":{"type":"LONG_EDGE"}`},
		},
		{
			name: "Short Edge Duplex",
			modifyRequest: func(req *pb.SubmitJobRequest) {
				req.Job.Ticket.Print.Duplex.Type = pb.Duplex_SHORT_EDGE.Enum()
			},
			expectedSnippets: []string{`"duplex":{"type":"SHORT_EDGE"}`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Start with a valid base request and modify it for the specific test case.
			req := newValidSubmitJobRequest()
			if tc.modifyRequest != nil {
				tc.modifyRequest(req)
			}

			script, err := svc.getSubmitJobScript(req)
			if err != nil {
				t.Fatalf("getSubmitJobScript() returned an unexpected error: %v", err)
			}

			// Check that the script contains the specific key-value pair for the enum.
			for _, snippet := range tc.expectedSnippets {
				if !strings.Contains(script, snippet) {
					t.Errorf("script: %s missing expected snippet.\nWant to contain: %q", script, snippet)
				}
			}
		})
	}
}
