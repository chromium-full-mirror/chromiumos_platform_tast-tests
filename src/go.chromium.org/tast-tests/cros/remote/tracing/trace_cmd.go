// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracing

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "go.chromium.org/tast-tests/cros/services/cros/tracing"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
)

// RemoteInstance implements a wrapper of TraceCmdService.
type RemoteInstance struct {
	service pb.TraceCmdServiceClient
}

type remoteInstanceOption func(config *pb.TraceCmdConfig)

// CPUBufferKiB returns an option to set up the size of per-cpu buffer
func CPUBufferKiB(size int) remoteInstanceOption {
	return func(config *pb.TraceCmdConfig) {
		config.BufferSizeKb = int32(size)
	}
}

// EnableEvents enables a series of events
func EnableEvents(events ...string) remoteInstanceOption {
	return func(config *pb.TraceCmdConfig) {
		for _, e := range events {
			config.Events = append(config.Events, &pb.TraceEvent{Name: e})
		}
	}
}

// EnableEventWith enables an event with specified filter and trigger (e.g. "stacktrace")
// If filter or trigger is empty, it is ignored.
func EnableEventWith(event, filter, trigger string) remoteInstanceOption {
	return func(config *pb.TraceCmdConfig) {
		config.Events = append(config.Events, &pb.TraceEvent{Name: event, Filter: filter, Trigger: trigger})
	}
}

// EnableFunctionTracer enables function tracer with filtered functions
func EnableFunctionTracer(functions ...string) remoteInstanceOption {
	return func(config *pb.TraceCmdConfig) {
		config.FtraceFunctions = functions
		config.EnableFtrace = true
	}
}

// InitialStop stops tracing when creating RemoteInstance.
func InitialStop() remoteInstanceOption {
	return func(config *pb.TraceCmdConfig) {
		config.InitialStop = true
	}
}

// NewRemoteInstance creates a trace-cmd instance on DUT
func NewRemoteInstance(ctx context.Context, cl *rpc.Client, name string, opts ...remoteInstanceOption) (*RemoteInstance, error) {
	service := pb.NewTraceCmdServiceClient(cl.Conn)

	var req pb.TraceCmdConfig
	for _, opt := range opts {
		opt(&req)
	}
	req.Name = name

	if _, err := service.CreateInstance(ctx, &req); err != nil {
		return nil, errors.Wrapf(err, "failed to create a trace instance %q", name)
	}
	return &RemoteInstance{service: service}, nil
}

// ReconnectRemoteInstance attaches to a trace-cmd instance on DUT
func ReconnectRemoteInstance(ctx context.Context, cl *rpc.Client, name string) (*RemoteInstance, error) {
	service := pb.NewTraceCmdServiceClient(cl.Conn)
	if _, err := service.ReconnectInstance(ctx, &pb.TraceCmdName{Name: name}); err != nil {
		return nil, errors.Wrap(err, "failed to attach to a trace instance")
	}
	return &RemoteInstance{service: service}, nil
}

// Restart cleans up the current trace buffer and starts tracing
func (ri *RemoteInstance) Restart(ctx context.Context) error {
	if ri.service == nil {
		return errors.New("RemoteInstance has been detached")
	}
	if _, err := ri.service.Restart(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start tracing")
	}
	return nil
}

type remoteInstanceSaveOption func(option *pb.TraceCmdSaveOption)

// KeepTracing indicates the save operation does not stop tracing.
func KeepTracing() remoteInstanceSaveOption {
	return func(option *pb.TraceCmdSaveOption) {
		option.KeepTracing = true
	}
}

// KeepOldData indicates the save operation does not clear old trace data.
func KeepOldData() remoteInstanceSaveOption {
	return func(option *pb.TraceCmdSaveOption) {
		option.KeepData = true
	}
}

// Save saves trace data in the kernel buffer to the disk and returns that path.
func (ri *RemoteInstance) Save(ctx context.Context, opts ...remoteInstanceSaveOption) (string, error) {
	var option pb.TraceCmdSaveOption
	for _, opt := range opts {
		opt(&option)
	}

	tracedata, err := ri.service.Save(ctx, &option)
	if err != nil {
		return "", errors.Wrap(err, "failed to save the trace data")
	}

	return tracedata.Path, nil
}

// Finalize removes the working directory and remove instance.
func (ri *RemoteInstance) Finalize(ctx context.Context) error {
	if _, err := ri.service.Finalize(ctx, &empty.Empty{}); err != nil {
		return err
	}
	return nil
}

// Simplified APIs
// These are handy and not depends on RemoteInstance's methods.
//
// _, err := tracing.NewRemoteInstance(ctx, instanceName, opts...)
// if err != nil {
// 	s.Fatal("Failed to create trace instance")
// }
// defer tracing.CleanupRemoteInstance(ctx, cl, instanceName)
// if err := tracing.StartRemoteInstanceTrace(ctx, cl, insatanceName); err != nil {
// 	s.Fatal("Failed to start tracing")
// }
// runWorkload() // if cl is disconnected please update it by rpc.Dial().
// if err := tracing.SaveRemoteInstanceTraceData(ctx, cl, instanceName,
// 		func(src string) error {
// 			return s.DUT().GetFile(ctx, src, localTraceDataPath)
// 		}); err != nil {
// 	s.Fatal("Failed to start tracing")
// }
//
// If you want to make sure to remove all trace-cmd instances created by tast
// tests, you can use CreateAllRemoteInstances() instead of CleanupRemoteInstance().

// StartRemoteInstanceTrace will start tracing on DUT.
func StartRemoteInstanceTrace(ctx context.Context, cl *rpc.Client, name string) error {
	ri, err := ReconnectRemoteInstance(ctx, cl, name)
	if err != nil {
		return errors.Wrap(err, "failed to reconnect tracing")
	}

	if err := ri.Restart(ctx); err != nil {
		return errors.Wrap(err, "failed to start tracing")
	}
	return nil
}

// SaveRemoteInstanceTraceData fetches the trace data from DUT and call saveFunc.
func SaveRemoteInstanceTraceData(ctx context.Context, cl *rpc.Client, name string, saveFunc func(src string) error) error {
	ri, err := ReconnectRemoteInstance(ctx, cl, name)
	if err != nil {
		return errors.Wrap(err, "failed to reconnect tracing")
	}

	src, err := ri.Save(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to save the trace data")
	}

	if err := saveFunc(src); err != nil {
		return errors.Wrapf(err, "failed to copy the data file from %s", src)
	}
	return nil
}

// CleanupRemoteInstance removes given remote trace instance.
func CleanupRemoteInstance(ctx context.Context, cl *rpc.Client, name string) error {
	ri, err := ReconnectRemoteInstance(ctx, cl, name)
	if err != nil {
		return errors.Wrap(err, "failed to reconnect tracing")
	}
	return ri.Finalize(ctx)
}

// CleanupAllRemoteInstances removes all remote trace instances in the DUT.
func CleanupAllRemoteInstances(ctx context.Context, cl *rpc.Client) error {
	service := pb.NewTraceCmdServiceClient(cl.Conn)
	if _, err := service.CleanupAll(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to cleanup all trace instances")
	}
	return nil
}
