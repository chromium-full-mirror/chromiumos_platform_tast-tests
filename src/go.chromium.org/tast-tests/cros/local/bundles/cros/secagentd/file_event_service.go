// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package secagentd

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	pb "go.chromium.org/tast-tests/cros/services/cros/secagentd"

	fe "go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/fileeventsimpl"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterFileEventServiceServer(srv, &FileEventService{s: s,
				hasError: false, hasFatalError: false})
		},
	})
}

type FileEventService struct {
	s             *testing.ServiceState
	hasError      bool
	hasFatalError bool
}

func (f *FileEventService) log(args ...interface{}) {
	f.s.Log(args...)
}

func (f *FileEventService) logf(format string, args ...interface{}) {
	f.s.Logf(format, args...)
}

func (f *FileEventService) error(args ...interface{}) {
	msg := "ERROR:" + fmt.Sprint(args...)
	f.s.Log(msg)
	f.hasError = true
}

func (f *FileEventService) errorf(format string, args ...interface{}) {
	message := "ERROR:" + fmt.Sprintf(format, args...)
	f.s.Log(message)
	f.hasError = true
}

func (f *FileEventService) fatal(args ...interface{}) {
	msg := "FATAL:" + fmt.Sprint(args...)
	f.s.Log(msg)
	f.hasFatalError = true
}

func (f *FileEventService) fatalf(format string, args ...interface{}) {
	msg := "FATAL:" + fmt.Sprintf(format, args...)
	f.s.Logf(format, msg)
	f.hasFatalError = true
}

func (f *FileEventService) TestFileEvents(ctx context.Context, request *pb.TestFileEventsRequest) (*empty.Empty, error) {
	var felogic fe.FileEvent
	felogic.Log = f.log
	felogic.Logf = f.logf
	felogic.Error = f.error
	felogic.Errorf = f.errorf
	felogic.Fatal = f.fatal
	felogic.Fatalf = f.fatalf
	testCase, err := fe.GetFileEventDetails(ctx, *request.Name, nil)
	if err != nil {
		f.s.Log("FATAL:", err)
		return nil, errors.Wrap(err, "FATAL: failed to generate test case")
	}
	felogic.DoTest(ctx, testCase)
	if f.hasError {
		f.s.Log("Test failed.")
		return nil, errors.Errorf("%s failed due to normal error",
			request.String())
	} else if f.hasFatalError {
		f.s.Log("Test failed due to FATAL error.")
		return nil, errors.Errorf("%s failed due to fatal error",
			request.String())
	}
	return &empty.Empty{}, nil
}
