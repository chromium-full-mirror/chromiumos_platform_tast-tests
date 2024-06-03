// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package linuxperf implements service for recording and processing profiles
// with the Linux Perf tool.
package linuxperf

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast-tests/cros/local/tracing/linuxperf"
	pb "go.chromium.org/tast-tests/cros/services/cros/tracing/linuxperf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterLinuxPerfServiceServer(srv, &Service{s: s})
		},
	})
}

// Service implements the Service to record perf data.
type Service struct {
	s *testing.ServiceState

	proc          *process.Process
	outFile       string
	scriptOut     string
	stopped       bool
	deleteOnClose bool
	background    bool
}

const (
	launchWaitTimeout = 10 * time.Second
	exitWaitTimeout   = 30 * time.Second
)

func findPerfPid() (int32, error) {
	procs, err := process.Processes()
	if err != nil {
		return -1, errors.Wrap(err, "failed to get processes")
	}
	for _, p := range procs {
		name, _ := p.Name()
		if name == "perf" {
			return p.Pid, nil
		}
	}
	return -1, errors.New("perf is not found")
}

// Start starts perf record.
func (s *Service) Start(ctx context.Context, options *pb.LinuxPerfOptions) (*pb.LinuxPerfResponse, error) {
	if s.proc != nil {
		return nil, errors.New("One instance is already connected")
	}
	s.stopped = false
	s.deleteOnClose = false

	// Since the subprocess in this context will be terminated after return,
	// forcibly use background mode always.
	options.Background = true

	instance, err := linuxperf.NewRecordInstanceFromProto(ctx, options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create perf record instance")
	}
	// Before clearing OutFile() by Leak(), saves it to service.
	s.outFile = instance.OutFile()
	if options.GetOutFile() == "" {
		s.deleteOnClose = true
	}
	// Since the background RecordInstance expects calling instance.Leak()
	// before instance.Close(), because it can not send SIGINT to background
	// instance correctly.
	instance.Leak()
	defer instance.Close()

	// Since there is a lag to launch the background perf, we have
	// to wait for it to get a real PID of perf command.
	var pid int32
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		pid, err = findPerfPid()
		return err
	}, &testing.PollOptions{Timeout: launchWaitTimeout}); err != nil {
		return nil, errors.Wrap(err, "failed to find pid of perf")
	}

	return s.Reconnect(ctx, &pb.LinuxPerfReconnectRequest{Pid: pid, OutFile: s.outFile, DeleteOnClose: s.deleteOnClose})
}

// Stop stops perf record.
func (s *Service) Stop(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {

	if s.proc == nil {
		return nil, errors.New("the instance is not initialized")
	}
	if s.stopped {
		return &emptypb.Empty{}, nil
	}
	if err := s.proc.SendSignal(unix.SIGINT); err != nil {
		return nil, errors.Wrap(err, "failed to terminate the perf record")
	}
	// Wait until the perf record writes out all data
	if err := procutil.WaitForTerminated(ctx, s.proc, exitWaitTimeout); err != nil {
		return nil, errors.Wrap(err, "failed to wait perf record termination")
	}
	s.stopped = true
	return &emptypb.Empty{}, nil
}

// Finalize closes the record instance and remove output file.
func (s *Service) Finalize(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	if s.proc == nil {
		return nil, errors.New("the instance is not initialized")
	}
	if !s.stopped {
		if _, err := s.Stop(ctx, req); err != nil {
			return nil, errors.Wrap(err, "failed to stop before close")
		}
	}
	if s.deleteOnClose {
		if err := os.Remove(s.outFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, errors.Wrap(err, "failed to remove perf record output file")
		}
		s.outFile = ""
		s.deleteOnClose = false
	}
	s.proc = nil
	// Remove s.scriptOut because it is a temporary file name which
	// SaveScript() has made.
	if s.scriptOut != "" {
		if err := os.Remove(s.scriptOut); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, errors.Wrap(err, "failed to remove perf script output file")
		}
		s.scriptOut = ""
	}
	return &emptypb.Empty{}, nil
}

// Reconnect connects to disconnected perf record.
func (s *Service) Reconnect(ctx context.Context, req *pb.LinuxPerfReconnectRequest) (*pb.LinuxPerfResponse, error) {
	if s.proc != nil {
		return nil, errors.New("This instance is already connected")
	}
	pid := req.GetPid()
	if pid <= 0 {
		return nil, errors.New("wrong pid is specified")
	}
	outfile := req.GetOutFile()
	if outfile == "" {
		return nil, errors.New("OutFile is empty")
	}
	if _, err := os.Stat(outfile); err != nil {
		return nil, errors.Wrapf(err, "trace data file (%s) does not exist", outfile)
	}
	var err error
	s.proc, err = process.NewProcess(int32(pid))
	if err != nil {
		return nil, errors.Wrapf(err, "the process %d is not found", pid)
	}
	name, err := s.proc.Name()
	if err == nil && name != "perf" {
		s.proc = nil
		return nil, errors.New("specified process is not perf")
	}
	s.outFile = outfile
	s.deleteOnClose = req.GetDeleteOnClose()
	return &pb.LinuxPerfResponse{Pid: pid, OutFile: outfile, DeleteOnClose: req.DeleteOnClose}, nil
}

// SaveScript converts the perf.data file into perf-script output.
func (s *Service) SaveScript(ctx context.Context, req *pb.LinuxPerfSaveScriptRequest) (*pb.LinuxPerfSaveScriptResponse, error) {
	if s.outFile == "" {
		return nil, errors.New("This Service is already finalized")
	}
	args := []string{"perf", "script"}

	outFile := req.GetOutFile()
	if outFile == "" {
		f, err := os.CreateTemp("", "perf.script")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temp output file")
		}
		if err := f.Close(); err != nil {
			return nil, errors.Wrap(err, "failed to close temp outFile for perf script")
		}
		outFile = f.Name()
		s.scriptOut = outFile
	}
	if !s.stopped {
		// Ignore error because Stop() always ensures the instance stopped.
		s.Stop(ctx, &emptypb.Empty{})
	}
	args = append(args, req.GetArgs()...)
	args = append(args, "-i", s.outFile)

	// What we want is run "perf script ... | gzip > outFile".
	script := strings.Join(args, " ")
	if req.Compress {
		script = script + " | gzip -c"
	}
	script = script + " > " + outFile
	testing.ContextLog(ctx, "Execute command: ", script)
	cmd := testexec.CommandContext(ctx, "/bin/sh", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		testing.ContextLog(ctx, "Failure command output: ", string(out))
		return nil, errors.Wrap(err, "failed to run perf script")
	}
	cmd.Wait()
	return &pb.LinuxPerfSaveScriptResponse{OutFile: outFile}, nil
}
