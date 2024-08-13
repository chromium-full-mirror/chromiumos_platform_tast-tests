// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bytes"
	"context"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/testexec"
	pb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterWaltServiceServer(srv, &WaltService{s: s})
		},
	})
}

// WaltService implements tast.cros.inputs.WaltService.
type WaltService struct {
	s  *testing.ServiceState
	wg sync.WaitGroup
}

// waltProgram contains information to run and interact with WALT.
type waltProgram struct {
	cmd          *testexec.Cmd
	outputBuffer *bytes.Buffer
}

// StartWaltService runs the WALT command and returns the command's output.
// It waits for a call to StopWaltService before exiting.
//
// For example, passing the request:
// WaltRequest{TestType: "drag", InputDevicePath: "/dev/input/event3", ExtraArgs: []string{"-m"}}
// would run the command:
// `walt --type drag --input /dev/input/event3 -m`.
func (svc *WaltService) StartWaltService(ctx context.Context, req *pb.WaltRequest) (*pb.WaltResponse, error) {
	svc.wg.Add(1)

	waltPrgm, err := startWaltCommand(ctx, req.TestType, req.InputDevicePath, req.ExtraArgs)
	if err != nil {
		return nil, errors.Wrap(err, "unable to start the WALT command")
	}

	// Waits until StopWaltService is called.
	svc.wg.Wait()

	if err := stopWaltCommand(waltPrgm.cmd); err != nil {
		return nil, err
	}

	return &pb.WaltResponse{Output: waltPrgm.outputBuffer.String()}, nil
}

// startWaltCommand starts the WALT command with the specified test type and
// device path as well as any extra arguments provided. Returns a waltProgram
// struct on success or an error on failure.
func startWaltCommand(ctx context.Context, testType, inputDevicePath string, extraArgs []string) (*waltProgram, error) {
	waltArgs := []string{"--type", testType, "--input", inputDevicePath}
	waltArgs = append(waltArgs, extraArgs...)
	waltCmd := testexec.CommandContext(ctx, "walt", waltArgs...)

	// Direct the WALT command to write its output to a buffer.
	var outputBuffer bytes.Buffer
	waltCmd.Stdout = &outputBuffer

	waltPrgm := &waltProgram{cmd: waltCmd, outputBuffer: &outputBuffer}

	if err := waltCmd.Start(); err != nil {
		return nil, err
	}

	return waltPrgm, nil
}

// stopWaltCommand kills the WALT command and waits for it to exit.
func stopWaltCommand(waltCmd *testexec.Cmd) error {
	if err := waltCmd.Kill(); err != nil {
		return err
	}

	waltCmd.Wait()
	return nil
}

// StopWaltService informs the StartWaltService function to stop running the WALT command.
func (svc *WaltService) StopWaltService(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	svc.wg.Done()
	return &empty.Empty{}, nil
}
