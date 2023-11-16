// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package debugd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/debugd"
	pb "go.chromium.org/tast-tests/cros/services/cros/debugd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterDebugdServiceServer(srv, &Service{serviceState: s})
		},
	})
}

// Service implements tast.cros.debugd.DebugdService.
type Service struct {
	serviceState *testing.ServiceState
	mutex        sync.Mutex
}

// PacketCapture is a streaming RPC, it starts a packet capture based on the
// requested options and streams the captured packet data back to the client.
// This RPC accepts one job at a time, additional requests will be hanged until
// the previous job returns.
// This service only supports frequency-based (Layer-2) capture for now and not
// device-based (Layer-3) capture.
func (s *Service) PacketCapture(req *pb.PacketCaptureRequest, server pb.DebugdService_PacketCaptureServer) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	opts, err := parsePacketCaptureOption(req)
	if err != nil {
		return err
	}

	ctx := server.Context()
	debugdConn, err := debugd.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to debugd D-Bus service")
	}

	statReadPipe, statWritePipe, err := os.Pipe()
	if err != nil {
		return errors.Wrap(err, "failed to open pipe for the packet capture output information")
	}
	defer statWritePipe.Close()
	defer statReadPipe.Close()

	pcapReadPipe, pcapWritePipe, err := os.Pipe()
	if err != nil {
		return errors.Wrap(err, "failed to open pipe for the packets information recorded by PacketCaptureStart")
	}
	defer pcapWritePipe.Close()
	defer pcapReadPipe.Close()

	handle, err := debugdConn.PacketCaptureStart(ctx, pcapWritePipe, statWritePipe, opts)
	if err != nil {
		statReadPipe.SetReadDeadline(time.Now().Add(5 * time.Second))
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, statReadPipe); err != nil {
			s.serviceState.Log("Failed to copy the state information of the PacketCaptureStart: ", err)
		} else {
			if result := buf.String(); result != "" {
				s.serviceState.Log("The state information of the PacketCaptureStart: ", result)
			}
		}
		return errors.Wrap(err, "failed to start packet capture process")
	}
	defer debugdConn.PacketCaptureStop(s.serviceState.ServiceContext(), handle)

	buffer := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			n, err := pcapReadPipe.Read(buffer)
			if err != nil {
				return err
			}
			if err := server.Send(&pb.PacketCaptureResponse{Data: buffer[:n]}); err != nil {
				return err
			}
		}
	}
}

func parsePacketCaptureOption(req *pb.PacketCaptureRequest) (map[string]dbus.Variant, error) {
	opts := map[string]dbus.Variant{}

	if req.GetMonitoredInterface() != "" {
		opts["monitor_connection_on"] = dbus.MakeVariant(req.GetMonitoredInterface())
	}

	if req.GetFrequency() != 0 {
		opts["frequency"] = dbus.MakeVariant(req.GetFrequency())
	}

	if req.GetHtLocation() != pb.PacketCaptureRequest_HTLOCATION_UNSPECIFIED {
		opts["ht_location"] = dbus.MakeVariant(strings.ToLower(req.GetHtLocation().String()))
	}

	switch req.GetVhtWidth() {
	case pb.PacketCaptureRequest_VHTCh_WIDTH_80:
		if _, ok := opts["frequency"]; !ok {
			return nil, errors.New(`"vht_width" option is only available with the frequency`)
		}
		opts["vht_width"] = dbus.MakeVariant("80")
	case pb.PacketCaptureRequest_VHTCh_WIDTH_160:
		if _, ok := opts["frequency"]; !ok {
			return nil, errors.New(`"vht_width" option is only available with the frequency`)
		}
		opts["vht_width"] = dbus.MakeVariant("160")
	default:
		// No actions are required if the RPC caller doesn't specify any vht_width option.
	}
	return opts, nil
}
