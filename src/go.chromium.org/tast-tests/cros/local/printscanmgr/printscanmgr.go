// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package printscanmgr provides a client library for the printscanmgr D-Bus
// service.
package printscanmgr

import (
	"context"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	ppb "go.chromium.org/chromiumos/system_api/printscanmgr_proto"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

const (
	dbusName      = "org.chromium.printscanmgr"
	dbusPath      = "/org/chromium/printscanmgr"
	dbusInterface = "org.chromium.printscanmgr"
)

// Printscanmgr is used to interact with the printscanmgr process over D-Bus.
// For a detailed specification of each D-Bus method, see
// src/platform2/printscanmgr/dbus_bindings/org.chromium.printscanmgr.xml
type Printscanmgr struct {
	obj dbus.BusObject
}

// New connects to printscanmgr via D-Bus and returns a Printscanmgr object.
func New(ctx context.Context) (*Printscanmgr, error) {
	conn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, err
	}

	obj := conn.Object(dbusName, dbus.ObjectPath(dbusPath))

	return &Printscanmgr{obj}, nil
}

// CupsAddAutoConfiguredPrinter calls the
// printscanmgr.CupsAddAutoConfiguredPrinter D-Bus method.
func (p *Printscanmgr) CupsAddAutoConfiguredPrinter(ctx context.Context, request *ppb.CupsAddAutoConfiguredPrinterRequest) (*ppb.CupsAddAutoConfiguredPrinterResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal CupsAddAutoConfiguredPrinterRequest")
	}

	var buf []byte
	if err := p.obj.CallWithContext(ctx, dbusInterface+".CupsAddAutoConfiguredPrinter", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call CupsAddAutoConfiguredPrinter")
	}

	response := &ppb.CupsAddAutoConfiguredPrinterResponse{}
	if err = proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal CupsAddAutoConfiguredPrinterResponse")
	}

	return response, nil
}

// CupsAddManuallyConfiguredPrinter calls the
// printscanmgr.CupsAddManuallyConfiguredPrinter D-Bus method.
func (p *Printscanmgr) CupsAddManuallyConfiguredPrinter(ctx context.Context, request *ppb.CupsAddManuallyConfiguredPrinterRequest) (*ppb.CupsAddManuallyConfiguredPrinterResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal CupsAddManuallyConfiguredPrinterRequest")
	}

	var buf []byte
	if err := p.obj.CallWithContext(ctx, dbusInterface+".CupsAddManuallyConfiguredPrinter", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call CupsAddManuallyConfiguredPrinter")
	}

	response := &ppb.CupsAddManuallyConfiguredPrinterResponse{}
	if err = proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal CupsAddManuallyConfiguredPrinterResponse")
	}

	return response, nil
}

// PrintscanDebugSetCategories calls the
// printscanmgr.PrintscanDebugSetCategories D-Bus method.
func (p *Printscanmgr) PrintscanDebugSetCategories(ctx context.Context, request *ppb.PrintscanDebugSetCategoriesRequest) (*ppb.PrintscanDebugSetCategoriesResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal PrintscanDebugSetCategoriesRequest")
	}

	var buf []byte
	if err := p.obj.CallWithContext(ctx, dbusInterface+".PrintscanDebugSetCategories", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call PrintscanDebugSetCategories")
	}

	response := &ppb.PrintscanDebugSetCategoriesResponse{}
	if err = proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal PrintscanDebugSetCategoriesResponse")
	}

	return response, nil
}
