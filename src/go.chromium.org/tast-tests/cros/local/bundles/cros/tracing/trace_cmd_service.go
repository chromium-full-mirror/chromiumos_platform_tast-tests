// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tracing implements service for tracing by trace-cmd and perfetto.
package tracing

import (
	"context"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/tracing"
	tcpb "go.chromium.org/tast-tests/cros/services/cros/tracing"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			tcpb.RegisterTraceCmdServiceServer(srv, &TraceCmdService{s: s})
		},
	})
}

// TraceCmdService implements a remote service for trace-cmd tracing.
type TraceCmdService struct {
	s *testing.ServiceState

	ti *tracing.TraceInstance
}

// Remote test needs to call CreateInstance or AttachInstance first for attaching
// the service to a specific instance. If you know the instance is not there,
// (e.g. the first time to call), use CreateInstance. Or when you lost connection
// and reconnect to the DUT which is not rebooted, use AttachInstance instead.
// If you need to change the setting, call Finalize to destroy the instance and
// call CreateInstance again.

// CreateInstance creates a trace-cmd ring buffer instance with given configs.
func (t *TraceCmdService) CreateInstance(ctx context.Context, req *tcpb.TraceCmdConfig) (*empty.Empty, error) {
	if t.ti != nil {
		return nil, errors.Errorf("this service already attached to %q instance", t.ti.Name())
	}
	if req.Name == "" {
		return nil, errors.New("no instance name specified")
	}

	// Build options
	opts := tracing.EmptyTraceCmdOptions()
	if req.BufferSizeKb != 0 {
		opts = append(opts, tracing.CPUBufferKiB(int(req.BufferSizeKb)))
	}
	if len(req.Events) != 0 {
		var events []string
		for _, e := range req.Events {
			// If the event is specified with filter or trigger, use EnableEventWith() option.
			if e.Filter != "" || e.Trigger != "" {
				opts = append(opts, tracing.EnableEventWith(e.Name, e.Filter, e.Trigger))
			} else {
				// Or, just collect event names and set by EnableEvents().
				events = append(events, e.Name)
			}
		}
		if len(events) != 0 {
			opts = append(opts, tracing.EnableEvents(events...))
		}
	}
	if req.EnableFtrace {
		opts = append(opts, tracing.EnableFunctionTracer(req.FtraceFunctions...))

	}

	ti, err := tracing.CreateTraceInstance(ctx, req.Name, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a new instance")
	}
	t.ti = ti

	if req.InitialStop {
		ti.Stop(ctx)
		ti.ClearCPUBuffer(ctx)
	}
	return &empty.Empty{}, nil
}

// ReconnectInstance re-connect this service to an existing instance.
// If there is not a specific instance, this does not create a new one, but just return an error.
func (t *TraceCmdService) ReconnectInstance(ctx context.Context, req *tcpb.TraceCmdName) (*empty.Empty, error) {
	if t.ti != nil {
		if t.ti.Name() == req.Name {
			return &emptypb.Empty{}, nil
		}
		return nil, errors.Errorf("this service already attached to %q instance", t.ti.Name())
	}
	if req.Name == "" {
		return nil, errors.New("no instance name specified")
	}

	ti, err := tracing.ReconnectTraceInstance(ctx, req.Name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to attach an existing instance")
	}
	t.ti = ti
	return &empty.Empty{}, nil
}

// Restart restarts tracing on the stopped trace instance.
// This is not detect that the instance is currently stopped or not.
func (t *TraceCmdService) Restart(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if t.ti == nil {
		return nil, errors.New("this service doesn't attach to any instance")
	}

	return &emptypb.Empty{}, t.ti.Restart(ctx)
}

// Save saves the contents of the ring buffer into a file and return that path.
func (t *TraceCmdService) Save(ctx context.Context, req *tcpb.TraceCmdSaveOption) (*tcpb.TraceCmdOutput, error) {
	if t.ti == nil {
		return nil, errors.New("this service doesn't attach to any instance")
	}
	ti := t.ti

	if !req.KeepTracing {
		ti.Stop(ctx)
	}

	f, err := os.CreateTemp("", "trace-cmd-*.dat")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a saved data file")
	}
	fname := f.Name()
	f.Close()
	remove := true
	defer func() {
		if remove {
			os.Remove(fname)
		}
	}()

	if err := ti.Save(ctx, fname); err != nil {
		return nil, errors.Wrap(err, "failed to save trace data")
	}
	remove = false
	if !req.KeepData {
		ti.ClearCPUBuffer(ctx)
	}

	return &tcpb.TraceCmdOutput{Path: fname}, nil
}

// Finalize removes saved data and trace instance.
func (t *TraceCmdService) Finalize(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if t.ti != nil {
		t.ti.Finalize(ctx)
		t.ti = nil
	}

	return &empty.Empty{}, nil
}

// CleanupAll removes all trace instances.
func (t *TraceCmdService) CleanupAll(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := tracing.CleanupAllTraceInstances(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to cleanup all instances")
	}
	return &empty.Empty{}, nil
}
