// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"chromiumos/tast/remote/crosserverutil"
	pb "chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LogStreamingServiceGRPC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check basic functionality of LogStreamingService",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "base",
				Val: &pb.StreamLogFilesRequest{
					Files: []*pb.StreamFile{
						{FileName: "/var/log/chrome/chrome"},
						{FileName: "/var/log/ui/ui.LATEST"},
						{FileName: "/var/log/messages"},
						{FileName: "/var/log/net.log"},
					},
				},
			},
			{
				Name: "include_default_files",
				Val: &pb.StreamLogFilesRequest{
					IncludeDefaultFiles: true,
				},
			},
		},
	})
}

// Minimum lines expected from streaming requests.
const lineCountThreshold = 20

// LogStreamingServiceGRPC tests ChromeService functionalities for managing chrome lifecycle.
func LogStreamingServiceGRPC(ctx context.Context, s *testing.State) {
	// Maps for counting number of log lines from streaming.
	logFilesCountingMap := map[string]int{
		"/var/log/chrome/chrome": 0,
		"/var/log/ui/ui.LATEST":  0,
		"/var/log/messages":      0,
		"/var/log/net.log":       0,
	}
	commandCountingMap := map[string]int{
		"/usr/bin/btmon": 0,
	}

	// The execution of the test on DUT and log file streaming is put in a function
	// This ensures that the gRPC server and communication is properly close.
	collectLogStreams(ctx, s, logFilesCountingMap, commandCountingMap)

	testing.ContextLog(ctx, "Lines from log files streamed: ", logFilesCountingMap)
	testing.ContextLog(ctx, "Lines from logging commands streamed: ", commandCountingMap)

	// Check if each of the requested files has crossed the minimum threshold of lines expected.
	for file, cnt := range logFilesCountingMap {
		if cnt < lineCountThreshold {
			s.Fatalf("Failed to collect the minimum threshold (%d < %d) for file: %s", cnt, lineCountThreshold, file)
		}
	}
	for cmd, cnt := range commandCountingMap {
		if cnt < lineCountThreshold {
			s.Fatalf("Failed to collect the minimum threshold (%d < %d) for command: %s", cnt, lineCountThreshold, cmd)
		}
	}
}

func collectLogStreams(ctx context.Context, s *testing.State, logFilesCountingMap,
	commandCountingMap map[string]int) {
	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	streamLogFilesClient := pb.NewLogStreamingServiceClient(cl.Conn)
	streamLogFilesRequest := s.Param().(*pb.StreamLogFilesRequest)
	logFilesStream, err := streamLogFilesClient.StreamLogFiles(ctx, streamLogFilesRequest)

	if err != nil {
		s.Fatal("Failed to set up log file stream: ", err)
	}

	streamCommandClient := pb.NewLogStreamingServiceClient(cl.Conn)
	streamCommandRequest := &pb.StreamCommandRequest{
		Command: &pb.StreamCommand{
			Command: "/usr/bin/btmon",
		},
	}
	btmonStream, err := streamCommandClient.StreamCommand(ctx, streamCommandRequest)

	if err != nil {
		s.Fatal("Failed to set up btmon stream: ", err)
	}

	f, err := os.Create(path.Join(s.OutDir(), "streaming.log"))
	if err != nil {
		s.Fatal("Failed to setup log file: ", err)
	}
	defer f.Close()

	errChannel := make(chan error, 1)

	// Handle log file streaming.
	go func() {
		for {
			value, err := logFilesStream.Recv()
			if err == io.EOF {
				testing.ContextLog(ctx, "Streaming received EOF: ", err)
				errChannel <- nil
				return
			}
			if err != nil {
				errStatus, _ := status.FromError(err)

				if errStatus.Code() == codes.Canceled {
					testing.ContextLog(ctx, "Streaming is canceled: ", err)
					return
				}
				s.Fatal("Streaming failed with error: ", errStatus.Message())
				return
			}
			line := value.GetLine()

			if val, ok := logFilesCountingMap[line.GetFileName()]; ok {
				logFilesCountingMap[line.GetFileName()] = val + 1
			}
			f.WriteString(fmt.Sprintf("%s ( %d ) (%s) %s\n", line.GetFileName(),
				line.GetLineNum(), line.GetTimestamp().AsTime(), line.GetMessage()))
		}
	}()

	// Handle Btmon streaming.
	go func() {
		for {
			value, err := btmonStream.Recv()
			if err == io.EOF {
				testing.ContextLog(ctx, "btmon Streaming received EOF: ", err)
				errChannel <- nil
				return
			}
			if err != nil {
				errStatus, _ := status.FromError(err)

				if errStatus.Code() == codes.Canceled {
					testing.ContextLog(ctx, "btmon Streaming is canceled: ", err)
					return
				}
				s.Fatal("btmon Streaming failed with error: ", errStatus.Message())
				return
			}
			line := value.GetLine()

			if val, ok := commandCountingMap[line.GetFileName()]; ok {
				commandCountingMap[line.GetFileName()] = val + 1
			}
			f.WriteString(fmt.Sprintf("%s ( %d ) (%s) %s\n", line.GetFileName(),
				line.GetLineNum(), line.GetTimestamp().AsTime(), line.GetMessage()))
		}
	}()

	// Populate credentials from Tast variable for the Gaia login test case.
	loginReq := &pb.NewRequest{}

	// Start Chrome on DUT.
	cs := pb.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	// Close Chrome on DUT.
	if _, err := cs.Close(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close Chrome: ", err)
	}
}
