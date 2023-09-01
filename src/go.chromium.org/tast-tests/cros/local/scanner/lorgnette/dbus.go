// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lorgnette provides an interface to talk to lorgnette over D-Bus.
package lorgnette

import (
	"context"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	lpb "chromiumos/system_api/lorgnette_proto"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

const (
	dbusName      = "org.chromium.lorgnette"
	dbusPath      = "/org/chromium/lorgnette/Manager"
	dbusInterface = "org.chromium.lorgnette.Manager"
)

// Lorgnette is used to interact with the lorgnette process over D-Bus.
// For detailed spec of each D-Bus method, please review
// src/platform2/lorgnette/dbus_bindings/org.chromium.lorgnette.Manager.xml
type Lorgnette struct {
	conn              *dbus.Conn
	obj               dbus.BusObject
	scanStatusWatcher *dbusutil.SignalWatcher
}

// New connects to lorgnette via D-Bus and returns a Lorgnette object.  The
// returned object will be registered for ScanStatusChanged signals.
func New(ctx context.Context) (*Lorgnette, error) {
	conn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, err
	}

	obj := conn.Object(dbusName, dbus.ObjectPath(dbusPath))

	spec := dbusutil.MatchSpec{
		Type:      "signal",
		Path:      dbusPath,
		Interface: dbusInterface,
		Member:    "ScanStatusChanged",
	}
	scanStatusWatcher, err := dbusutil.NewSignalWatcher(ctx, conn, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to register for ScanStatusChanged signals from lorgnette")
	}

	return &Lorgnette{conn, obj, scanStatusWatcher}, nil
}

// StartScannerDiscovery calls lorgnette's StartScannerDiscovery method and returns the remote response.
func (l *Lorgnette) StartScannerDiscovery(ctx context.Context, request *lpb.StartScannerDiscoveryRequest) (*lpb.StartScannerDiscoveryResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal StartScannerDiscoveryRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".StartScannerDiscovery", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call StartScannerDiscovery")
	}

	response := &lpb.StartScannerDiscoveryResponse{}
	if err = proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal StartScannerDiscoveryResponse")
	}

	return response, nil
}

// StopScannerDiscovery calls lorgnette's StopScannerDiscovery method and returns the remote response.
func (l *Lorgnette) StopScannerDiscovery(ctx context.Context, request *lpb.StopScannerDiscoveryRequest) (*lpb.StopScannerDiscoveryResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal StopScannerDiscoveryRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".StopScannerDiscovery", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call StopScannerDiscovery")
	}

	response := &lpb.StopScannerDiscoveryResponse{}
	if err = proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal StopScannerDiscoveryResponse")
	}

	return response, nil
}

// AccumulateDiscoveredScanners waits for ScannerListChangedSignal signals and accumulates a list of added scanners.  It returns when a ENUM_COMPLETE event is received.
func (l *Lorgnette) AccumulateDiscoveredScanners(ctx context.Context, session string) ([]*lpb.ScannerInfo, error) {
	spec := dbusutil.MatchSpec{
		Type:      "signal",
		Path:      dbusPath,
		Interface: dbusInterface,
		Member:    "ScannerListChanged",
	}
	scannerListWatcher, err := dbusutil.NewSignalWatcher(ctx, l.conn, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to register for ScannerListChanged signals from lorgnette")
	}

	var scanners []*lpb.ScannerInfo
	for {
		select {
		case sig := <-scannerListWatcher.Signals:
			var buf []byte
			if err := dbus.Store(sig.Body, &buf); err != nil {
				return nil, errors.Wrap(err, "failed to extract ScannerListChangedSignal body")
			}
			signal := lpb.ScannerListChangedSignal{}
			if err := proto.Unmarshal(buf, &signal); err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal ScannerListChangedSignal")
			}

			if signal.SessionId != session {
				continue
			}

			switch signal.EventType {
			case lpb.ScannerListChangedSignal_SCANNER_ADDED:
				scanners = append(scanners, signal.Scanner)
			case lpb.ScannerListChangedSignal_SESSION_ENDING:
				return nil, errors.New("discovery session ended before ENUM_COMPLETE received")
			case lpb.ScannerListChangedSignal_ENUM_COMPLETE:
				return scanners, nil
			}

		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "did not receive ENUM_COMPLETE event")

		}
	}
}

// StartScan calls lorgnette's StartScan method and returns the remote response.
func (l *Lorgnette) StartScan(ctx context.Context, request *lpb.StartScanRequest) (*lpb.StartScanResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal StartScanRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".StartScan", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call StartScan")
	}

	response := &lpb.StartScanResponse{}
	if err = proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal StartScanResponse")
	}

	return response, nil
}

// GetNextImage calls lorgnette's GetNextImage method and returns the remote response.
func (l *Lorgnette) GetNextImage(ctx context.Context, request *lpb.GetNextImageRequest, outFD uintptr) (*lpb.GetNextImageResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal GetNextImageRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".GetNextImage", 0, marshalled, dbus.UnixFD(outFD)).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call GetNextImage")
	}

	response := &lpb.GetNextImageResponse{}
	if err = proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal GetNextImageResponse")
	}

	return response, nil
}

// WaitForScanCompletion waits for a ScanStatusChanged signal that matches uuid
// and has a terminal status.  Intermediate statuses are ignored.
func (l *Lorgnette) WaitForScanCompletion(ctx context.Context, uuid string) error {
	for {
		select {
		case sig := <-l.scanStatusWatcher.Signals:
			var buf []byte
			if err := dbus.Store(sig.Body, &buf); err != nil {
				return errors.Wrap(err, "failed to extract ScanStatusChangedSignal body")
			}
			signal := lpb.ScanStatusChangedSignal{}
			if err := proto.Unmarshal(buf, &signal); err != nil {
				return errors.Wrap(err, "failed to unmarshal ScanStatusChangedSignal")
			}

			if signal.ScanUuid != uuid {
				continue
			}

			switch signal.State {
			case lpb.ScanState_SCAN_STATE_FAILED:
				return errors.Errorf("scan failed: %s", signal.FailureReason)
			case lpb.ScanState_SCAN_STATE_PAGE_COMPLETED:
				if signal.MorePages {
					return errors.New("did not expect additional pages for scan")
				}
			case lpb.ScanState_SCAN_STATE_COMPLETED:
				return nil
			}

		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "did not receive scan completion signal")

		}
	}
}

// StopService finds the running lorgnette process and kills it.
func StopService(ctx context.Context) error {
	return upstart.StopJob(ctx, "lorgnette")
}

// ListScanners calls lorgnette's ListScanners() method and returns the
// (possibly empty) list of ScannerInfo objects from the response.
func (l *Lorgnette) ListScanners(ctx context.Context) ([]*lpb.ScannerInfo, error) {
	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".ListScanners", 0).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call ListScanners")
	}

	response := &lpb.ListScannersResponse{}
	if err := proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal ListScannersResponse")
	}

	return response.Scanners, nil
}

// OpenScanner calls lorgnette's OpenScanner method and returns the remote response.
func (l *Lorgnette) OpenScanner(ctx context.Context, request *lpb.OpenScannerRequest) (*lpb.OpenScannerResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal OpenScannerRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".OpenScanner", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call OpenScanner")
	}

	response := &lpb.OpenScannerResponse{}
	if err := proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal OpenScannerResponse")
	}

	return response, nil
}

// CloseScanner calls lorgnette's CloseScanner method and returns the remote response.
func (l *Lorgnette) CloseScanner(ctx context.Context, request *lpb.CloseScannerRequest) (*lpb.CloseScannerResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal CloseScannerRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".CloseScanner", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call CloseScanner")
	}

	response := &lpb.CloseScannerResponse{}
	if err := proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal CloseScannerResponse")
	}

	return response, nil
}

// StartPreparedScan calls lorgnette's StartPreparedScan method and returns the remote response.
func (l *Lorgnette) StartPreparedScan(ctx context.Context, request *lpb.StartPreparedScanRequest) (*lpb.StartPreparedScanResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal StartPreparedScanRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".StartPreparedScan", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call StartPreparedScan")
	}

	response := &lpb.StartPreparedScanResponse{}
	if err := proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal StartPreparedScanResponse")
	}

	return response, nil
}

// CancelScan calls lorgnette's CancelScan method and returns the remote response.
func (l *Lorgnette) CancelScan(ctx context.Context, request *lpb.CancelScanRequest) (*lpb.CancelScanResponse, error) {
	marshalled, err := proto.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal CancelScanRequest")
	}

	var buf []byte
	if err := l.obj.CallWithContext(ctx, dbusInterface+".CancelScan", 0, marshalled).Store(&buf); err != nil {
		return nil, errors.Wrap(err, "failed to call CancelScan")
	}

	response := &lpb.CancelScanResponse{}
	if err := proto.Unmarshal(buf, response); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal CancelScanResponse")
	}

	return response, nil
}
