// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package common provides common functionalities and utilities
package common

import (
	"context"
	"sync"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
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
	// Pointer to Browser connected to Lacros Chrome. The value is set once one of methods to connect to Lacros on LacrosService is called.
	// Note that this Browser abstraction is used instead of Lacros object because using Lacros will introduce a circular dependency.
	LacrosBrowser *browser.Browser
}

// Browser returns a pointer to a Browser instance on either Ash or Lacros.
func (so *SharedObjectsForService) Browser(usingLacros bool) (*browser.Browser, error) {
	if usingLacros {
		if so.LacrosBrowser == nil {
			return nil, errors.New("Lacros not instantiated")
		}
		return so.LacrosBrowser, nil
	}

	if so.Chrome == nil {
		return nil, errors.New("Chrome not instantiated")
	}
	return so.Chrome.Browser(), nil
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
	return UseTconnMaybeLacros(ctx, so, fn, false)
}

// UseTconnMaybeLacros performs an action on tconn either on (Lacros) browser or (Ash) Chrome depending on the value of usingLacros.
// Read comment on UseTconn on how to use it.
func UseTconnMaybeLacros[T any](ctx context.Context, so *SharedObjectsForService, fn func(tconn *chrome.TestConn) (*T, error), usingLacros bool) (*T, error) {
	so.ChromeMutex.Lock()
	defer so.ChromeMutex.Unlock()

	br, err := so.Browser(usingLacros)
	if err != nil {
		return nil, err
	}

	// When in OOBE, use SigninProfileTestAPIConn to create the test connection.
	testAPIConn := br.TestAPIConn
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
