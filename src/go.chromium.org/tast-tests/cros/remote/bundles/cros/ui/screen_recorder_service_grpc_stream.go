// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenRecorderServiceGRPCStream,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check video streaming functionalities of ScreenRecorderService",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

// ScreenRecorderServiceGRPCStream Verifies that that the screen recorder streaming API works.
// Steps:
// 1, Establish cros server and gRPC channel.
// 2. Start screen recorder gRPC streaming.
// 3. Start a goroutine to write screen recorder stream into a file.
// 4, Perform some UI actions.
// 5. Clients cancel streaming.
// 6. Verify that the recording file exists.
func ScreenRecorderServiceGRPCStream(ctx context.Context, s *testing.State) {
	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Start Chrome on the DUT.
	cs := pb.NewChromeServiceClient(cl.Conn)
	loginReq := &pb.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(ctx, &empty.Empty{})

	// Start screen recording stream
	streamScreenRecordingClient := pb.NewScreenRecorderServiceClient(cl.Conn)
	streamScreenRecordingRequest := &pb.StreamScreenRecordingRequest{}
	maxSizeOption := grpc.MaxCallRecvMsgSize(16 * 10e6)

	// Note that according to the gRPC specification, the only way to close a stream
	// is to cancel the context. Closing the underlying gRPC channel will also have the
	// same effect.
	streamCtx, streamCancel := context.WithCancel(ctx)
	stream, err := streamScreenRecordingClient.StreamScreenRecording(streamCtx, streamScreenRecordingRequest, maxSizeOption)
	if err != nil {
		s.Fatal("Failed to start screen recording stream: ", err)
	}

	// error channel to interact with the screen recording file writing goroutine.
	fileWritingErrCh := make(chan error, 1)

	// goroutine to write screen recording blobs to file.
	recordingFileName := filepath.Join(s.OutDir(), "record.webm")
	defer os.Remove(recordingFileName)
	go writeStreamToFile(ctx, fileWritingErrCh, stream, recordingFileName)

	// Performs some actions on the UI so we have something to record.
	if err := performUIActions(ctx, cl); err != nil {
		s.Fatal("Failed to perform UI actions: ", err)
	}

	testing.ContextLog(ctx, "Client cancels gRPC stream")
	streamCancel()

	// Wait for goroutine to process last bit of video blobs.
	testing.ContextLog(ctx, "Waiting for write recording file goroutine to finish")
	if err := <-fileWritingErrCh; err != nil {
		s.Fatal("Failed to write video stream to file: ", err)
	}
	testing.ContextLog(ctx, "Terminated write recording file goroutine")

	// Wait until the screen recording button to disappear to ensure that
	// chrome screen recorder is properly stopped and released.
	waitUntilScreenRecorderFinish(ctx, cl)

	// Verify recording.
	if err := verifyRecordingFile(ctx, recordingFileName); err != nil {
		s.Fatal("Failed to verify screen recording file: ", err)
	}
}

// waitUntilScreenRecorderFinish waits until the screen recording button to disappear.
func waitUntilScreenRecorderFinish(ctx context.Context, cl *crosserverutil.Client) error {
	uiautoSvc := pb.NewAutomationServiceClient(cl.Conn)
	req := &pb.WaitUntilGoneRequest{
		Finder: &pb.Finder{
			NodeWiths: []*pb.NodeWith{
				{Value: &pb.NodeWith_HasClass{HasClass: "ImageView"}},
				{Value: &pb.NodeWith_Name{Name: "You're sharing your screen"}},
			},
		},
		Timeout: durationpb.New(5 * time.Second),
	}
	_, err := uiautoSvc.WaitUntilGone(ctx, req)
	return err
}

// performUIActions performs some UI actions.
func performUIActions(ctx context.Context, cl *crosserverutil.Client) error {
	// Performs some actions on the UI like Opening Files App
	uiautoSvc := pb.NewAutomationServiceClient(cl.Conn)
	filesAppShelfButtonFinder := &pb.Finder{
		NodeWiths: []*pb.NodeWith{
			{Value: &pb.NodeWith_HasClass{HasClass: "ash/ShelfAppButton"}},
			{Value: &pb.NodeWith_Name{Name: "Files"}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &pb.WaitUntilExistsRequest{Finder: filesAppShelfButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to find Files shelf button")
	}
	for i := 0; i < 10; i++ {
		if _, err := uiautoSvc.LeftClick(ctx, &pb.LeftClickRequest{Finder: filesAppShelfButtonFinder}); err != nil {
			return errors.Wrap(err, "failed to click on Files app")
		}
		// GoBigSleepLint: Ensure that there is enough to UI actions for recording.
		// The passing decision is not hinging on this timing.
		testing.Sleep(ctx, time.Second)
	}
	return nil
}

// writeStreamToFile write screen recording stream messages to a file.
// The return value will be passed through the error channel which is an input parameter.
func writeStreamToFile(ctx context.Context, errCh chan error,
	stream pb.ScreenRecorderService_StreamScreenRecordingClient, recordingFileName string) {
	fRecording, err := os.OpenFile(recordingFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		errCh <- err
		return
	}

	for {
		value, err := stream.Recv()
		if err == io.EOF {
			testing.ContextLog(ctx, "Write recording file goroutine: Streaming received EOF: ", err)
			fRecording.Close()
			errCh <- nil
			return
		}
		if err != nil {
			fRecording.Close()
			errStatus, _ := status.FromError(err)

			if errStatus.Code() == codes.Canceled {
				testing.ContextLog(ctx, "Write recording file goroutine: Streaming is canceled: ", err)
				errCh <- nil
				return
			}
			testing.ContextLog(ctx, "Write recording file goroutine: Streaming failed with error: ", err)
			errCh <- err
			return
		}
		length := value.GetLength()
		if length >= 0 {
			if _, err := fRecording.Write(value.GetData()); err != nil {
				fRecording.Close()
				errCh <- err
				return
			}
		}
		testing.ContextLog(ctx, "Write recording file goroutine: Receive bytes: ", length)
	}
}

// verifyRecordingFile verifies that the screen recording file was created.
func verifyRecordingFile(ctx context.Context, recordingFileName string) error {
	// Note: with ffmpeg and ffprobe, we can validate the format and retrieve the metadata
	// of the screen recording file. However those tools are not available with autotest
	// lxc container. So for now, we will just verify the exitence of the file.
	stat, err := os.Stat(recordingFileName)
	if err != nil {
		return err
	}
	if stat.Size() == 0 {
		return errors.Errorf("Screen recording file %s is empty", recordingFileName)
	}
	return nil
}
