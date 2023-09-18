// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

import "encoding/binary"

var defaultVendorID []byte = []byte{0, 0}
var defaultStaticStsIv []byte = []byte{0, 0, 0, 0, 0, 0}
var resultReportConfigAzimuthOn = ResultReportConfig{
	Tof:          true,
	AoaAzimuth:   true,
	AoaElevation: true,
	AoaFom:       true,
}
var resultReportConfigAzimuthOff = ResultReportConfig{
	Tof:          true,
	AoaAzimuth:   false,
	AoaElevation: false,
	AoaFom:       false,
}

var defaultRangingRoundControl = RangingRoundControl{
	RangingResultReportMessage: true,
	ControlMessage:             true,
	MeasurementReportMessage:   false,
}

// FiraDefaultControllerSet returns the default fira parameters for a controller and initiator
func FiraDefaultControllerSet() *FiraAppConfigParams {
	return &FiraAppConfigParams{
		DeviceType:                    DeviceType_CONTROLLER,
		RangingRoundUsage:             RangingRoundUsage_DS_TWR,
		StsConfig:                     StsConfig_STATIC,
		MultiNodeMode:                 MultiNodeMode_UNICAST,
		ChannelNumber:                 UwbChannel_CHANNEL_9,
		DeviceMacAddress:              []byte{8, 0},
		DstMacAddress:                 [][]byte{{4, 0}},
		SlotDurationRstu:              2400,
		RangingIntervalMs:             200,
		MacFcsType:                    MacFcsType_CRC_16,
		RangingRoundControl:           &defaultRangingRoundControl,
		AoaResultRequest:              AoaResultRequest_REQ_AOA_RESULTS,
		RangeDataNtfConfig:            RangeDataNtfConfig_RANGE_DATA_NTF_CONFIG_ENABLE,
		RangeDataNtfProximityNearCm:   0,
		RangeDataNtfProximityFarCm:    20000,
		DeviceRole:                    DeviceRole_INITIATOR,
		RframeConfig:                  RframeConfig_SP3,
		PreambleCodeIndex:             9,
		SfdId:                         2,
		PsduDataRate:                  PsduDataRate_RATE_6M_81,
		PreambleDuration:              PreambleDuration_T64_SYMBOLS,
		RangingTimeStruct:             RangingTimeStruct_BLOCK_BASED_SCHEDULING,
		SlotsPerRr:                    25,
		TxAdaptivePayloadPower:        TxAdaptivePayloadPower_TX_ADAPTIVE_PAYLOAD_POWER_DISABLE,
		ResponderSlotIndex:            1,
		PrfMode:                       PrfMode_BPRF,
		ScheduledMode:                 ScheduledMode_TIME_SCHEDULED_RANGING,
		KeyRotation:                   KeyRotation_KEY_ROTATION_DISABLE,
		KeyRotationRate:               0,
		SessionPriority:               70,
		MacAddressMode:                MacAddressMode_MAC_ADDRESS_2_BYTES,
		VendorId:                      defaultVendorID,
		StaticStsIv:                   defaultStaticStsIv,
		NumberOfStsSegments:           1,
		MaxRrRetry:                    0,
		UwbInitiationTimeMs:           0,
		HoppingMode:                   HoppingMode_HOPPING_MODE_DISABLE,
		BlockStrideLength:             0,
		ResultReportConfig:            &resultReportConfigAzimuthOn,
		InBandTerminationAttemptCount: 1,
		SubSessionId:                  0,
		BprfPhrDataRate:               BprfPhrDataRate_BPRF_PHR_DATA_RATE_850K,
		MaxNumberOfMeasurements:       0,
		StsLength:                     StsLength_LENGTH_64,
	}
}

// FiraDefaultControleeSet returns the controlee counterpart parameters to the default controller set
func FiraDefaultControleeSet() *FiraAppConfigParams {
	params := FiraDefaultControllerSet()
	temp := make([]byte, len(params.DeviceMacAddress))
	copy(temp, params.DeviceMacAddress)
	copy(params.DeviceMacAddress, params.DstMacAddress[0])
	copy(params.DstMacAddress[0], temp)
	params.DeviceRole = DeviceRole_RESPONDER
	params.DeviceType = DeviceType_CONTROLEE
	return params
}

// ByteArrayToInt returns the uint64 that is represented by the given byte array. Used to convert MAC address to uint64
func ByteArrayToInt(byteArray []byte) uint64 {
	paddedData := make([]byte, 8)
	copy(paddedData, byteArray)

	// Convert the byte array to an uint64 using little-endian byte order
	intValue := binary.LittleEndian.Uint64(paddedData)

	return intValue
}
