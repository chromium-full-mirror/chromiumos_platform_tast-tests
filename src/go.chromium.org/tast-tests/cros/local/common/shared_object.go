// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package common provides common functionalities and utilities
package common

import (
	"context"
	"sync"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// SharedObjectsForService allows services to shared states of important objects, such as
// chrome and arc. While this provides access to the important objects, the lifecycle management
// of these objects is not the responsibility of this struct. Instead individual services
// will share the responsibility of managing the lifecycle of these objects.
// A common pattern is to include a reference during Service instantiation and registration. e.g.
//
//	 testing.AddService(&testing.Service{
//		  Register: func(srv *grpc.Server, s *testing.ServiceState) {
//				automationService := AutomationService{s: s, sharedObject: common.SharedObjectsForServiceSingleton}
//				pb.RegisterAutomationServiceServer(srv, &automationService)
//			},
//		})
type SharedObjectsForService struct {
	// Chrome instance connected to Ash Chrome. The value is set once ChromeService.New is called.
	Chrome *chrome.Chrome
	// Mutex to protect against concurrent access to Chrome and Browser.
	ChromeMutex sync.Mutex
}

// UseTconn performs an action that requires access to tconn.
// A common pattern is to make T the response type of the service, eg.
//
//	func (svc *service) MyRPC(ctx context.Context, req *RequestProto) (*ResponseProto, err) {
//	  return UseTconn(ctx, so, func(tconn) (*ResponseProto, err) {
//	    <do stuff with tconn>
//	    return &ResponseProto{...}, nil
//	  })
//	}
func UseTconn[T any](ctx context.Context, so *SharedObjectsForService, fn func(tconn *chrome.TestConn) (*T, error)) (*T, error) {
	so.ChromeMutex.Lock()
	defer so.ChromeMutex.Unlock()

	// When in OOBE, use SigninProfileTestAPIConn to create the test connection.
	testAPIConn := so.Chrome.TestAPIConn
	if so.Chrome.LoginMode() == "NoLogin" {
		testAPIConn = so.Chrome.SigninProfileTestAPIConn
	}
	tconn, err := testAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	return fn(tconn)
}

// SharedObjectsForServiceSingleton is the Singleton object that allows sharing states
// between services
var SharedObjectsForServiceSingleton = &SharedObjectsForService{}
