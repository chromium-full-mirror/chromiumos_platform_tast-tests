// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package linuxperf implements the remote LinuxPerfService interface.
package linuxperf

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/protobuf/types/known/emptypb"

	pb "go.chromium.org/tast-tests/cros/services/cros/tracing/linuxperf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
)

// This requires "tast.cros.tracing.linuxperf.LinuxPerfService" in ServiceDeps.

// RemotePerfToken is a wrapper of LinuxPerfReconnectRequest.
type RemotePerfToken = pb.LinuxPerfReconnectRequest

// RemoteLinuxPerf implements the remote LinuxPerfService
type RemoteLinuxPerf struct {
	service        pb.LinuxPerfServiceClient
	token          *RemotePerfToken
	traceDataPath  string
	scriptDataPath string
}

// Stop stops the remote perf record.
func (s *RemoteLinuxPerf) Stop(ctx context.Context) error {
	if _, err := s.service.Stop(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to stop remote session")
	}
	return nil
}

// Finalize finishes the remote perf record and remove temporary trace
// data.
func (s *RemoteLinuxPerf) Finalize(ctx context.Context) error {
	if _, err := s.service.Finalize(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to finalize remote session")
	}
	s.service = nil
	return nil
}

type remoteScriptOption func(o *pb.LinuxPerfSaveScriptRequest)

// SaveScript converts the trace data into script data.
func (s *RemoteLinuxPerf) SaveScript(ctx context.Context, opts ...remoteScriptOption) error {
	var req pb.LinuxPerfSaveScriptRequest
	for _, o := range opts {
		o(&req)
	}
	res, err := s.service.SaveScript(ctx, &req)
	if err != nil {
		return errors.Wrap(err, "failed to save script file")
	}
	s.scriptDataPath = res.OutFile
	return nil
}

// ScriptFormat is an option to set format string to SaveScript.
func ScriptFormat(format string) remoteScriptOption {
	return func(o *pb.LinuxPerfSaveScriptRequest) {
		o.Args = append(o.Args, "-F", format)
	}
}

// Compress enables gzip compress on SaveScript.
func Compress() remoteScriptOption {
	return func(o *pb.LinuxPerfSaveScriptRequest) {
		o.Compress = true
	}
}

// WithScriptDataPath configures the SaveScript() to save the script file
// to the given path.
func WithScriptDataPath(path string) remoteScriptOption {
	return func(o *pb.LinuxPerfSaveScriptRequest) {
		o.OutFile = path
		o.Args = append(o.Args, "-o", path)
	}
}

// Token gets a session token from a background perf record.
func (s *RemoteLinuxPerf) Token() *RemotePerfToken {
	return s.token
}

// TraceDataPath returns the remote file path of trace data.
func (s *RemoteLinuxPerf) TraceDataPath() string {
	return s.traceDataPath
}

// ScriptDataPath returns the remote file path of perf-script data.
// This will be empty until calling SaveScript().
func (s *RemoteLinuxPerf) ScriptDataPath() string {
	return s.scriptDataPath
}

type remotePerfOption func(o *pb.LinuxPerfOptions)

// WithTraceDataPath configures the session with trace data written to the
// given path.
func WithTraceDataPath(path string) remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
		o.OutFile = path
		o.Args = append(o.Args, "-o", path)
	}
}

// AllCpus enables system wide collection of perf events. See perf record -a
// option.
func AllCpus() remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
		o.Args = append(o.Args, "-a")
	}
}

// Timeout sets the timeout in second.
func Timeout(timeout int) remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
		o.TimeoutSeconds = int32(timeout)
	}
}

// Event adds tracing event.
func Event(event string) remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
		o.Args = append(o.Args, "-e", event)
	}
}

// EventWithPeriod selects a PMU event. See perf record -e option.
func EventWithPeriod(event string, period uint64) remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
		// If we skip the name param, then the name shown in perf script includes
		// the period param.
		o.Args = append(o.Args, fmt.Sprintf("--event=%s/period=%d,name=%s/", event, period, event))
	}
}

// MaxSize stops a RecordInstance once the output file reaches the provided
// size. RecordInstance.Stop will fail if the output reached the size limit.
func MaxSize(mib int) remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
		o.Args = append(o.Args, fmt.Sprintf("--max-size=%dM", mib))
	}
}

// Processes enables collection of perf events from the passed processes. See
// perf record -o option.
func Processes(pids ...int) remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
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
		o.Args = append(o.Args, "-p", pidsString)
	}
}

// Stacks enables call graph collection (stack backtrace) on a perf event. See
// perf record -g option.
func Stacks() remotePerfOption {
	return func(o *pb.LinuxPerfOptions) {
		o.Args = append(o.Args, "-g")
	}
}

// StartRemoteLinuxPerf starts LinuxPerf record session in remote.
func StartRemoteLinuxPerf(ctx context.Context, cl *rpc.Client, opts ...remotePerfOption) (*RemoteLinuxPerf, error) {
	service := pb.NewLinuxPerfServiceClient(cl.Conn)
	option := &pb.LinuxPerfOptions{}
	for _, opt := range opts {
		opt(option)
	}
	res, err := service.Start(ctx, option)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start remote perf record")
	}
	return &RemoteLinuxPerf{
		service:       service,
		token:         &RemotePerfToken{Pid: res.Pid, OutFile: res.OutFile, DeleteOnClose: res.DeleteOnClose},
		traceDataPath: res.OutFile,
	}, nil
}

// ReconnectRemoteLinuxPerf reconnects the disconnected LinuxPerf record session.
func ReconnectRemoteLinuxPerf(ctx context.Context, cl *rpc.Client, tok *RemotePerfToken) (*RemoteLinuxPerf, error) {
	service := pb.NewLinuxPerfServiceClient(cl.Conn)
	res, err := service.Reconnect(ctx, tok)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to remote perf record")
	}
	return &RemoteLinuxPerf{
		service:       service,
		token:         &RemotePerfToken{Pid: res.Pid, OutFile: res.OutFile, DeleteOnClose: res.DeleteOnClose},
		traceDataPath: res.OutFile,
	}, nil
}
