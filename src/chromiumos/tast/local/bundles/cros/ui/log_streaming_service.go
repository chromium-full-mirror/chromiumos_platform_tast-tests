// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"bufio"
	"io"
	"sync"
	"time"

	"github.com/nxadm/tail"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	pb "chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterLogStreamingServiceServer(srv, &LogStreamingService{})
		},
		GuaranteeCompatibility: true,
	})
}

// defaultLogFiles captures the most common CrOS log files.
var defaultLogFilesMap = map[string]bool{
	"/var/log/chrome/chrome": true,
	"/var/log/ui/ui.LATEST":  true,
	"/var/log/messages":      true,
	"/var/log/net.log":       true,
}

// LogStreamingService implements tast.cros.ui.LogStreamingService.
type LogStreamingService struct {
}

// StreamLogFiles streams contents of log files.
// Note that when user starts streaming log files, it is only streaming new contents starting from the end of the file.
// The line will be given a line number of 1.
// StreamLogFiles can detect the renaming of files and will continue to stream from the new files.
// When log rotation happens, line number restarts from 1.
func (svc *LogStreamingService) StreamLogFiles(req *pb.StreamLogFilesRequest, stream pb.LogStreamingService_StreamLogFilesServer) error {
	fileNames, err := svc.getFileNames(req)
	if err != nil {
		return err
	}

	// Keep track of the tails and lineChannels corresponding to the requested log files.
	tails := make([]*tail.Tail, len(fileNames))
	lineChannels := make([]<-chan *tail.Line, len(fileNames))

	// Set up tails and lineChannels for log files.
	for i, fileName := range fileNames {
		t, err := tail.TailFile(
			fileName, tail.Config{
				// Start streaming from the end of the file and include only new contents.
				Location: &tail.SeekInfo{
					Offset: -1,
					Whence: io.SeekEnd,
				},
				Follow: true,
				// ReOpen has the effect of "tail -F" which handles file renaming and log rotation.
				ReOpen: true,
				// Poll for file changes instead of using the default inotify.  When ReOpen is used,
				// this flag enables the new lines to be tailed after a file is renamed.
				Poll:   true,
				Logger: tail.DiscardingLogger,
			})
		if err != nil {
			return err
		}
		tails[i] = t
		lineChannels[i] = t.Lines
	}

	// Stop tailing when clients cancels and closes streaming.
	defer func() {
		// Stop and clean up tail objects.
		for _, t := range tails {
			t.Stop()
			t.Cleanup()
		}
	}()

	// Merge all channels into a single channel.
	lineWithMetadataChannel := merge(lineChannels, tails)

	for {
		select {
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "Stream has ended")

		case lineWithMetadata := <-lineWithMetadataChannel:
			res := &pb.StreamLogFilesResponse{
				Line: &pb.LogLine{
					Message:   lineWithMetadata.Line.Text,
					FileName:  lineWithMetadata.Tail.Filename,
					LineNum:   int64(lineWithMetadata.Line.Num),
					Timestamp: timestamppb.New(lineWithMetadata.Line.Time),
				},
			}
			err := stream.SendMsg(res)
			if err != nil {
				return err
			}
		}
	}
}

// getFileNames returns a list of log filenames from request.
func (svc *LogStreamingService) getFileNames(req *pb.StreamLogFilesRequest) ([]string, error) {
	logFilesMap := make(map[string]bool)

	// Include common CrOS log files as default.
	if req.IncludeDefaultFiles {
		for k, v := range defaultLogFilesMap {
			logFilesMap[k] = v
		}
	}

	// Add and dedupe user requested log files.
	for _, f := range req.Files {
		logFilesMap[f.FileName] = true
	}
	fileNames := make([]string, len(logFilesMap))
	i := 0
	for k := range logFilesMap {
		fileNames[i] = k
		i++
	}

	if len(fileNames) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "No files specified for streaming.")
	}
	return fileNames, nil
}

// StreamCommand initiates a logging command and streams the output of the command.
// The command is expected to be a long running instrumentation command, e.g. btmon.
// StreamCommand mananges the lifecycle of the command and terminates the command
// when the stream is closed.
func (svc *LogStreamingService) StreamCommand(req *pb.StreamCommandRequest, stream pb.LogStreamingService_StreamCommandServer) error {
	lineNum := 1
	if req.Command.Command == "" {
		return status.Errorf(codes.InvalidArgument, "Command cannot be empty.")
	}
	cmd := testexec.CommandContext(stream.Context(), req.Command.Command, req.Command.Args...)

	lineChannel := make(chan string)

	// combine stdout and stderr to a single reader by assigning a single pipe to cmd.Stdout
	// and cmd.Stderr.
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	scanner := bufio.NewScanner(cmdReader)

	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "failed to start cmd: %v", req.Command)
	}
	// Explicitly kill the command just to be safe.
	// Command should have been terminated when context is cancelled.
	defer cmd.Kill()

	// Start goroutine to feed scanner output into a string channel.
	go func() {
		// The command session will close the stderr and stdout upon termination
		// causing the scanner to exit the loop.
		// Scanner does not interplay well with channels.
		// i.e. As the for loop is watiing on the scanner, there is not a good way
		// for the main loop to inform the goroutine and terminate the scanner.
		for scanner.Scan() {
			line := scanner.Text()
			lineChannel <- line
		}
	}()

	// This loop listens to the string channel and send stream responses.
	for {
		select {
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "Stream has ended")
		case line := <-lineChannel:
			res := &pb.StreamCommandResponse{
				Line: &pb.LogLine{
					Message:   line,
					FileName:  req.Command.Command,
					LineNum:   int64(lineNum),
					Timestamp: timestamppb.New(time.Now()),
				},
			}
			lineNum++
			err := stream.SendMsg(res)
			if err != nil {
				return err
			}
		}
	}
}

// lineWithMetadata contains both the content and the metadata of the tail.
type lineWithMetadata struct {
	// Line contains a line of the file content.
	Line *tail.Line
	// Tail contains tail metadata such as target file name.
	Tail *tail.Tail
}

// merge multiple line channels into a single channel
// The output channel is based on lineWithMetadata. The wrapper provides additional metadata
// like filename which makes it a lot easier to identify and filter log file contents.
func merge(cs []<-chan *tail.Line, tailDefs []*tail.Tail) <-chan *lineWithMetadata {
	outputChannel := make(chan *lineWithMetadata)
	var wg sync.WaitGroup

	// Copies values from input channel to output channel until input channel is closed, then calls wg.Done.
	output := func(ch <-chan *tail.Line, t *tail.Tail) {
		for v := range ch {
			// Wrap line object with the corresponding tail metadata.
			lineWithConfig := &lineWithMetadata{
				Line: v,
				Tail: t,
			}
			outputChannel <- lineWithConfig
		}
		wg.Done()
	}
	wg.Add(len(cs))

	// Start goroutine for each tailing operations.
	for i, c := range cs {
		go output(c, tailDefs[i])
	}

	// Start a goroutine to close outputChannel once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(outputChannel)
	}()
	return outputChannel
}
