// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package uwb contains the dbus client that interacts with the UWB dbus service to facilitate usage of UWB.
package uwb

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/tast-tests/cros/common/uwb"
)

// DbusClient contains BusObject pointing to UWB D-Bus Service
type DbusClient struct {
	obj           dbusutil.DBusObject
	RangingSignal *dbusutil.SignalWatcher
}

// NewClient initializes a connection to UWB D-Bus and returns DbusClient with corresponding object
func NewClient(ctx context.Context) (*DbusClient, error) {
	obj, err := dbusutil.NewDBusObject(ctx, dbusService, dbusService, dbusPath)

	if err != nil {
		return nil, err
	}

	spec := dbusutil.MatchSpec{
		Type:      "signal",
		Path:      dbusPath,
		Interface: dbusService,
		Member:    dbusOnRangeDataReceived,
	}

	signal, err := dbusutil.NewSignalWatcher(ctx, obj.Conn(), spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to register for uwbd rangedata signals")
	}

	return &DbusClient{obj: *obj, RangingSignal: signal}, nil
}

// Enable calls UWB Dbus Enable method
func (d DbusClient) Enable(ctx context.Context) (*uwb.EnableResponse, error) {
	response := uwb.EnableResponse{}

	err := d.dbusCall(ctx, nil, dbusEnable, &response)
	if err != nil {
		return &response, errors.Wrap(err, "D-Bus Enable method call failed")
	}

	return &response, nil
}

// Disable calls UWB Dbus Disable method
func (d DbusClient) Disable(ctx context.Context) (*uwb.DisableResponse, error) {
	response := uwb.DisableResponse{}

	err := d.dbusCall(ctx, nil, dbusDisable, &response)
	if err != nil {
		return &response, errors.Wrap(err, "D-Bus Disable method call failed")
	}

	return &response, nil
}

// InitSession calls UWB Dbus InitSession method
func (d DbusClient) InitSession(ctx context.Context, sessionID uint32, params *uwb.FiraAppConfigParams) (*uwb.InitSessionResponse, error) {
	initSessionRequest := &uwb.InitSessionRequest{
		SessionId:   sessionID,
		SessionType: uwb.SessionType_FIRA_RANGING_SESSION,
		Params:      params,
	}

	response := uwb.InitSessionResponse{}

	if err := d.dbusCallWithParams(ctx, initSessionRequest, dbusInitSession, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus InitSession method call failed")
	}

	return &response, nil
}

// DeinitSession calls UWB Dbus DeinitSession method
func (d DbusClient) DeinitSession(ctx context.Context, sessionID uint32) (*uwb.DeinitSessionResponse, error) {
	deinitSessionRequest := &uwb.DeinitSessionRequest{
		SessionId: sessionID,
	}
	response := uwb.DeinitSessionResponse{}

	if err := d.dbusCallWithParams(ctx, deinitSessionRequest, dbusDeinitSession, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus DeinitSession method call failed")
	}

	return &response, nil
}

// StartRanging calls UWB Dbus StartRanging method
func (d DbusClient) StartRanging(ctx context.Context, sessionID uint32) (*uwb.StartRangingResponse, error) {
	startRangingRequest := &uwb.StartRangingRequest{
		SessionId: sessionID,
	}

	response := uwb.StartRangingResponse{}

	if err := d.dbusCallWithParams(ctx, startRangingRequest, dbusStartRanging, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus StartRanging method call failed")
	}

	return &response, nil
}

// StopRanging calls UWB Dbus StopRanging method
func (d DbusClient) StopRanging(ctx context.Context, sessionID uint32) (*uwb.StopRangingResponse, error) {
	stopRangingRequest := &uwb.StopRangingRequest{
		SessionId: sessionID,
	}

	response := uwb.StopRangingResponse{}

	if err := d.dbusCallWithParams(ctx, stopRangingRequest, dbusStopRanging, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus StopRanging method call failed")
	}

	return &response, nil
}

// SetLoggerMode calls UWB Dbus SetLoggerMode method
func (d DbusClient) SetLoggerMode(ctx context.Context, mode uwb.UciLoggerMode) (*uwb.SetLoggerModeResponse, error) {
	setLoggerModeRequest := &uwb.SetLoggerModeRequest{
		LoggerMode: mode,
	}

	response := uwb.SetLoggerModeResponse{}

	if err := d.dbusCallWithParams(ctx, setLoggerModeRequest, dbusSetLoggerMode, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus SetLoggerMode method call failed")
	}

	return &response, nil
}

// SessionParams calls UWB Dbus SessionParams method
func (d DbusClient) SessionParams(ctx context.Context, sessionID uint32) (*uwb.SessionParamsResponse, error) {
	sessionParamsRequest := &uwb.SessionParamsRequest{
		SessionId: sessionID,
	}

	response := uwb.SessionParamsResponse{}

	if err := d.dbusCallWithParams(ctx, sessionParamsRequest, dbusSessionParams, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus SessionParams method call failed")
	}

	return &response, nil
}

// Reconfigure calls UWB Dbus Reconfigure method
func (d DbusClient) Reconfigure(ctx context.Context, sessionID uint32, params *uwb.FiraAppConfigParams) (*uwb.ReconfigureResponse, error) {
	reconfigureRequest := &uwb.ReconfigureRequest{
		SessionId: sessionID,
		Params:    params,
	}

	response := uwb.ReconfigureResponse{}

	if err := d.dbusCallWithParams(ctx, reconfigureRequest, dbusReconfigure, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus Reconfigure method call failed")
	}

	return &response, nil
}

// UpdateControllerMulticastList calls UWB Dbus UpdateControllerMulticastList method
func (d DbusClient) UpdateControllerMulticastList(ctx context.Context, sessionID uint32, action uwb.UpdateMulticastListAction, controlees []*uwb.Controlee) (*uwb.UpdateControllerMulticastListResponse, error) {
	updateRequest := &uwb.UpdateControllerMulticastListRequest{
		SessionId:  sessionID,
		Action:     action,
		Controlees: controlees,
	}

	response := uwb.UpdateControllerMulticastListResponse{}

	if err := d.dbusCallWithParams(ctx, updateRequest, dbusUpdateControllerMulticastList, &response); err != nil {
		return &response, errors.Wrap(err, "D-Bus UpdateControllerMulticastList method call failed")
	}

	return &response, nil
}

// GetFiraRangingReports listens for OnRangedDataReceivedSignals for a given amount of time, and returns all the reports
// received for the given sessionID in the form of a map where the key is the MAC Address of the other peer device
func (d DbusClient) GetFiraRangingReports(ctx context.Context, sessionID uint32, duration time.Duration) (map[uint64]*uwb.RangingResult, error) {

	spec := dbusutil.MatchSpec{
		Type:      "signal",
		Path:      dbusPath,
		Interface: dbusService,
		Member:    dbusOnRangeDataReceived,
	}

	signal, err := dbusutil.NewSignalWatcher(ctx, d.obj.Conn(), spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to register for uwbd rangedata signals")
	}

	// bufferTimer & bufferCheck is used to wait for a short duration before collecting the results from the ranging signals as UWB on the peer devices may take a couple
	// seconds to begin and we do not want to collect the notifications prior to beginning
	bufferDuration := 5 * time.Second
	bufferTimer := time.NewTimer(bufferDuration)
	defer bufferTimer.Stop()
	bufferCheck := true

	// rangingTimer is the timer to collect ranging reports as results; We add bufferDuration to account for bufferTimer being allowed to Stop prior to receiving the first OK
	rangingTimer := time.NewTimer(duration + bufferDuration)
	results := make(map[uint64]*uwb.RangingResult)
	defer rangingTimer.Stop()

readSignalChannel:
	for {
		select {
		case sig := <-signal.Signals:
			var buf []byte
			if err := dbus.Store(sig.Body, &buf); err != nil {
				return nil, errors.Wrap(err, "failed to extract OnRangedDataReceivedSignal body")
			}
			signal := uwb.RangeDataReceivedSignal{}

			if err := proto.Unmarshal(buf, &signal); err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal OnRangeDataReceivedSignal")
			}

			if signal.RangeData.SessionId == sessionID {

				for _, report := range signal.RangeData.TwowayRangingMeasurements {
					// if buffer hasn't expired and bufferCheck is false, check if status_ok
					if !bufferTimer.Stop() && bufferCheck == false {
						// if status_ok, set boolean to true indicating that the buffer is no longer necessary and proceed
						if report.Status == uwb.StatusCode_UCI_STATUS_OK {
							bufferCheck = true
							// we also reset rangingTimer to start for given duration from this moment onwards
							rangingTimer = time.NewTimer(duration)
						} else {
							// otherwise, buffer is still necessary, continue to check subsequent iterations
							continue
						}
					}

					// check if there exists a RangingResult in the results map for this mac address, creating one if not
					_, ok := results[report.MacAddress]
					if !ok {
						results[report.MacAddress] = &uwb.RangingResult{
							TotalReportCount:      0,
							SuccessfulReportCount: 0,
							DistanceReadings:      []uint32{},
							AzimuthReadings:       []*uwb.AngleReading{},
							ElevationReadings:     []*uwb.AngleReading{},
							StatusCodes:           make(map[int32]uint64),
						}
					}

					// checks if status code is OK
					if report.Status == uwb.StatusCode_UCI_STATUS_OK {
						// Increment SuccessfulReportCount
						results[report.MacAddress].SuccessfulReportCount++
						// Append Distance reading
						results[report.MacAddress].DistanceReadings = append(
							results[report.MacAddress].DistanceReadings,
							report.Distance,
						)

						// Append Angle readings, after converting from q9.7 format to degrees
						results[report.MacAddress].AzimuthReadings = append(
							results[report.MacAddress].AzimuthReadings,
							&uwb.AngleReading{Reading: Q97ToDegrees(int16(report.AoaAzimuth)),
								Fom: report.AoaAzimuthFom,
							})

						results[report.MacAddress].ElevationReadings = append(
							results[report.MacAddress].ElevationReadings,
							&uwb.AngleReading{Reading: Q97ToDegrees(int16(report.AoaElevation)),
								Fom: report.AoaElevationFom,
							})

						results[report.MacAddress].DestinationAzimuthReadings = append(
							results[report.MacAddress].DestinationElevationReadings,
							&uwb.AngleReading{Reading: Q97ToDegrees(int16(report.AoaDestinationAzimuth)),
								Fom: report.AoaDestinationAzimuthFom,
							})

						results[report.MacAddress].DestinationElevationReadings = append(
							results[report.MacAddress].DestinationAzimuthReadings,
							&uwb.AngleReading{Reading: Q97ToDegrees(int16(report.AoaDestinationElevation)),
								Fom: report.AoaDestinationElevationFom,
							})
					}
					// Increment TotalReportCount
					results[report.MacAddress].TotalReportCount++
					// Update StatusCode field
					results[report.MacAddress].StatusCodes[int32(report.Status)]++
				}
			}

		case <-rangingTimer.C:
			break readSignalChannel

		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "reached test session timeout")

		}
	}
	return results, nil
}

// dbusCall is a helper function to make calls to uwb dbus service
// methodName should be method name defined in dbus_constants.go
// requestBytes should be the argument to dbus call marshalled into serialized/byte format. nil if method has no argument
// response should be corresponding response message generated in uwb_service.pb.go
func (d DbusClient) dbusCall(ctx context.Context, requestBytes []byte, methodName string, response proto.Message) error {
	call := d.obj.Call(ctx, methodName, requestBytes)

	if call.Err != nil {
		return errors.Wrap(call.Err, "D-Bus method call failed")
	}

	var responseBytes []uint8

	err := call.Store(&responseBytes)
	if err != nil {
		return errors.Wrap(err, "Call.Store failed")
	}

	err = proto.Unmarshal(responseBytes, response)
	if err != nil {
		return errors.Wrap(err, "could not unmarshal method response")
	}

	return nil
}

func (d DbusClient) dbusCallWithParams(ctx context.Context, request proto.Message, methodName string, response proto.Message) error {
	requestBytes, err := proto.Marshal(request)
	if err != nil {
		return err
	}

	err = d.dbusCall(ctx, requestBytes, methodName, response)
	if err != nil {
		return err
	}
	return nil
}

// Q97ToDegrees converts an angle in q9.7 format to degrees. Used for converting the angle data received which comes in q9.7
func Q97ToDegrees(q97 int16) float64 {
	sign := 1.0
	if q97 < 0 {
		sign = -1.0
		q97 = -q97 // negate number to clear sign bit
	}
	degrees := float64(q97 >> 7)            // get integer part
	fractional := float64(q97&0x7f) / 128.0 // get fractional part
	return (degrees + fractional) * sign    // add integer and fractional parts multiplied by sign
}

// DegreesToQ97 converts an angle in degrees to its q9.7 format
func DegreesToQ97(degrees float64) int16 {
	sign := int16(1.0)
	if degrees < 0 {
		sign = int16(-1.0)
		degrees = -degrees // negate degrees to clear sign bit
	}
	q97 := int16(degrees) << 7                              // get integer part with sign
	q97 += int16((degrees - float64(int16(degrees))) * 128) // get fractional part
	return q97 * sign                                       // apply the sign to q97
}
