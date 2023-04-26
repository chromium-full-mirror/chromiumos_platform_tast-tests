// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package printscanmgr provides a client library for the printscanmgr D-Bus
// service.
package printscanmgr

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	ppb "chromiumos/system_api/printscanmgr_proto"
	"chromiumos/tast/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

const (
	dbusName      = "org.chromium.printscanmgr"
	dbusPath      = "/org/chromium/printscanmgr"
	dbusInterface = "org.chromium.printscanmgr"
)

// CUPSResult is a status code for the CUPS related printscanmgr D-Bus methods.
// Values are from
// src/platform2/system_api/dbus/printscanmgr/printscanmgr_service.proto
type CUPSResult int32

const (
	// CUPSUnspecified indicates an error occurred.
	CUPSUnspecified CUPSResult = 0

	// CUPSSuccess indicates the operation succeeded.
	CUPSSuccess CUPSResult = 1

	// CUPSFatal indicates the operation failed for an unknown reason.
	CUPSFatal CUPSResult = 2

	// CUPSInvalidPPD indicates the operation failed because the given PPD is
	// invalid.
	CUPSInvalidPPD CUPSResult = 3

	// CUPSLPAdminFailure indicates the operation failed because the lpadmin
	// command failed.
	CUPSLPAdminFailure CUPSResult = 4

	// CUPSAutoconfFailure indicates the operation failed due to autoconf
	// failures.
	CUPSAutoconfFailure CUPSResult = 5

	// CUPSBadURI indicates that the operation failed because printscanmgr
	// rejected the printer URI.
	CUPSBadURI CUPSResult = 6

	// CUPSIOError indicates that the operation failed because of an I/O error.
	CUPSIOError CUPSResult = 7

	// CUPSMemoryAllocError indicates that the operation failed because of a
	// memory allocation error.
	CUPSMemoryAllocError CUPSResult = 8

	// CUPSPrinterUnreachable indicates that the printer did not respond.
	CUPSPrinterUnreachable CUPSResult = 9

	// CUPSPrinterWrongResponse indicates that the printer sent an unexpected
	// response.
	CUPSPrinterWrongResponse CUPSResult = 10

	// CUPSPrinterNotAutoconf indicates that the operation failed because the
	// printer is not autoconfigurable as it supposed to be.
	CUPSPrinterNotAutoconf CUPSResult = 11
)

func (r CUPSResult) String() string {
	switch r {
	case CUPSUnspecified:
		return fmt.Sprintf("CUPSUnspecified(%d)", r)
	case CUPSSuccess:
		return fmt.Sprintf("CUPSSuccess(%d)", r)
	case CUPSFatal:
		return fmt.Sprintf("CUPSFatal(%d)", r)
	case CUPSInvalidPPD:
		return fmt.Sprintf("CUPSInvalidPPD(%d)", r)
	case CUPSLPAdminFailure:
		return fmt.Sprintf("CUPSLPAdminFailure(%d)", r)
	case CUPSAutoconfFailure:
		return fmt.Sprintf("CUPSAutoconfFailure(%d)", r)
	case CUPSBadURI:
		return fmt.Sprintf("CUPSBadURI(%d)", r)
	case CUPSIOError:
		return fmt.Sprintf("CUPSIOError(%d)", r)
	case CUPSMemoryAllocError:
		return fmt.Sprintf("CUPSMemoryAllocError(%d)", r)
	case CUPSPrinterUnreachable:
		return fmt.Sprintf("CUPSPrinterUnreachable(%d)", r)
	case CUPSPrinterWrongResponse:
		return fmt.Sprintf("CUPSPrinterWrongResponse(%d)", r)
	case CUPSPrinterNotAutoconf:
		return fmt.Sprintf("CUPSPrinterNotAutoconf(%d)", r)
	default:
		return fmt.Sprintf("Unknown(%d)", r)
	}
}

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
