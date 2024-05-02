// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package linuxperf contains helpers for recording and processing profiles
// with the Linux Perf tool.
package linuxperf

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"syscall"

	"go.chromium.org/tast-tests/cros/common/testexec"
	pb "go.chromium.org/tast-tests/cros/services/cros/tracing/linuxperf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

type recordOptions struct {
	args           []string
	background     bool
	outFile        string
	timeoutSeconds int32
}

type recordOption func(o *recordOptions)

// AllCpus enables system wide collection of perf events. See perf record -a
// option.
func AllCpus() recordOption {
	return func(o *recordOptions) {
		o.args = append(o.args, "-a")
	}
}

// Background starts perf record detached from the Tast process. This allows it
// to run even after the Tast process dies. Record.Close will still stop
// recording, unless Record.Leak is called first. Used in remote tests where the
// gRPC server restarts.
// Using the Timeout and MaxSize options with Background is a good idea.
func Background() recordOption {
	return func(o *recordOptions) {
		o.background = true
	}
}

// EventWithPeriod selects a PMU event. See perf record -e option.
func EventWithPeriod(event string, period uint64) recordOption {
	return func(o *recordOptions) {
		// If we skip the name param, then the name shown in perf script includes
		// the period param.
		o.args = append(o.args, fmt.Sprintf("--event=%s/period=%d,name=%s/", event, period, event))
	}
}

// MaxSize stops a RecordInstance once the output file reaches the provided
// size. RecordInstance.Stop will fail if the output reached the size limit.
func MaxSize(mib int) recordOption {
	return func(o *recordOptions) {
		o.args = append(o.args, fmt.Sprintf("--max-size=%dM", mib))
	}
}

// Output sets the file path to save the profile.
func Output(outFile string) recordOption {
	return func(o *recordOptions) {
		o.outFile = outFile
		o.args = append(o.args, "-o", outFile)
	}
}

// Processes enables collection of perf events from the passed processes. See
// perf record -o option.
func Processes(pids ...int) recordOption {
	return func(o *recordOptions) {
		if len(pids) == 0 {
			return
		}
		pidsString := ""
		for _, pid := range pids {
			if len(pidsString) > 0 {
				pidsString += ","
			}
			pidsString += strconv.Itoa(pid)
		}
		o.args = append(o.args, "-p", pidsString)
	}
}

// Stacks enables call graph collection (stack backtrace) on a perf event. See
// perf record -g option.
func Stacks() recordOption {
	return func(o *recordOptions) {
		o.args = append(o.args, "-g")
	}
}

// Timeout adds a timeout to a RecordInstance. RecordInstance.Stop will fail if
// the instance timed out.
func Timeout(timeoutSeconds int32) recordOption {
	return func(o *recordOptions) {
		o.timeoutSeconds = timeoutSeconds
	}
}

// RecordInstance is a helper used to collect Linux Perf profiles.
type RecordInstance struct {
	cmd           *testexec.Cmd
	outFile       string
	deleteOnClose bool
	background    bool
}

func newRecordInstanceFromOptions(ctx context.Context, o *recordOptions) (*RecordInstance, error) {
	pi := &RecordInstance{}
	if o.outFile == "" {
		f, err := os.CreateTemp("", "perf.data")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temp outFile for perf record")
		}
		if err := f.Close(); err != nil {
			return nil, errors.Wrap(err, "failed to close temp outFile for perf record")
		}
		testing.ContextLogf(ctx, "Using %q as temporary output for Linux perf record", f.Name())
		Output(f.Name())(o)
		pi.deleteOnClose = true
	}
	pi.outFile = o.outFile
	args := append([]string{"record"}, o.args...)
	if o.timeoutSeconds > 0 {
		args = append(args, "--", "sleep", fmt.Sprintf("%d", o.timeoutSeconds))
	}
	pi.cmd = testexec.CommandContext(ctx, "perf", args...)

	if o.background {
		pi.cmd.SysProcAttr.Setpgid = false
		pi.cmd.Stdin = nil
		pi.cmd.Stdout = nil
		pi.cmd.Stderr = nil
		pi.background = true
	}

	if err := pi.cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start perf command")
	}

	return pi, nil
}

// NewRecordInstance starts a Linux Perf profile by running 'perf record'.
// Stop() must be called before any analysis is done on the profile.
// Close() must be called on every RecordInstance to free resources.
// If no Output option is provided, a temporary file is created that will be
// deleted on Close.
// Example:
//
//		 r, err := linuxperf.NewRecordInstance(
//			  ctx,
//			  linuxperf.Output(filepath.Join(s.OutDir(), "perf.data")),
//			  linuxperf.AllCpus(),
//			  linuxperf.Stacks(),
//			  linuxperf.EventWithPeriod("cpu-cycles", 2800000),
//		 )
//	  if err != nil { ... }
//	  defer r.Close()
//	  ... Do things that are profiled ...
//	  if err := r.Stop(); err != nil { ... }
func NewRecordInstance(ctx context.Context, opts ...recordOption) (*RecordInstance, error) {
	o := &recordOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return newRecordInstanceFromOptions(ctx, o)
}

// Stop the Linux Perf profile.
func (pi *RecordInstance) Stop() error {
	if pi.cmd == nil {
		return errors.New("perf record process was leaked")
	}
	if pi.cmd.ProcessState != nil {
		return errors.New("perf record is not running")
	}
	if err := pi.cmd.Signal(unix.SIGINT); err != nil {
		return errors.Wrap(err, "failed to SIGINT perf record process")
	}
	err := pi.cmd.Wait()
	if err == nil {
		return errors.New("perf exited with status 0 after a SIGINT, maybe it already exited")
	}
	state := pi.cmd.ProcessState
	if state == nil {
		return errors.Wrap(err, "no ProcessState after sending SIGINT to perf")
	}
	status := state.Sys().(syscall.WaitStatus)
	if status.Signal() != unix.SIGINT {
		return errors.Wrap(err, "failed to wait for perf to finish after SIGINT")
	}
	return nil
}

// Close stops the profile if it is still running, and deletes the trace file if
// the file name was automatically generated.
func (pi *RecordInstance) Close() error {
	if pi.cmd != nil && pi.cmd.ProcessState == nil {
		if err := pi.Stop(); err != nil {
			return err
		}
	}
	if pi.deleteOnClose {
		if err := os.Remove(pi.outFile); err != nil {
			return errors.Wrap(err, "failed to remove perf record output file")
		}
	}
	return nil
}

// OutFile returns the output file of the perf record command.
func (pi *RecordInstance) OutFile() string {
	return pi.outFile
}

// Pid returns the pid of the perf record process. Errors if perf is not
// recording.
func (pi *RecordInstance) Pid() (int, error) {
	if pi.cmd == nil {
		return -1, errors.New("perf record process was leaked")
	}
	process := pi.cmd.Process
	if process == nil {
		return -1, errors.New("perf record process is not running")
	}
	return process.Pid, nil
}

// Leak clears all internal state of the Record wrapper. The perf record process
// will continue to run, but will not be cleaned up on Close. Stop or any other
// method can not be called. THE CALLER IS RESPONSIBLE FOR STOPPING THE PERF
// RECORD PROCESS AND DELETING ANY PROFILE DATA. Used to allow the perf record
// session to live longer than the calling Tast process. E.g. in a suspend test
// where the Tast service process doesn't survive across the suspend.
func (pi *RecordInstance) Leak() error {
	if !pi.background {
		return errors.New("perf record process can not be leaked because it was not started in background mode")
	}
	pi.cmd = nil
	pi.outFile = ""
	pi.deleteOnClose = false

	return nil
}

// OptionsToProto serializes a set of Linux Perf record options to a proto type
// that can be sent over a gRPC. See NewInstance for a description of opts.
func OptionsToProto(opts ...recordOption) *pb.LinuxPerfOptions {
	o := &recordOptions{}
	for _, opt := range opts {
		opt(o)
	}
	proto := &pb.LinuxPerfOptions{}
	proto.Args = o.args
	proto.OutFile = o.outFile
	proto.Background = o.background
	proto.TimeoutSeconds = o.timeoutSeconds
	return proto
}

// NewRecordInstanceFromProto creates a new Linux Perf record instance from a
// proto file created (possibly remotely) by OptionsToProto.
func NewRecordInstanceFromProto(ctx context.Context, proto *pb.LinuxPerfOptions) (*RecordInstance, error) {
	o := &recordOptions{proto.GetArgs(), proto.GetBackground(), proto.GetOutFile(), proto.GetTimeoutSeconds()}
	return newRecordInstanceFromOptions(ctx, o)
}
