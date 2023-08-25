// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package manager

import (
	"encoding/json"
	"fmt"
)

// Addresses of callbox DAU web services.
const (
	DAUAddressIPv4 = "www.ipv4.dau.dau"
	DAUAddressIPv6 = "www.ipv6.dau.dau"
)

// Band represents a RAT frequency band.
type Band string

// NewBand returns a Band given an integer band number.
func NewBand(band int) Band {
	return Band(fmt.Sprintf("%d", band))
}

// NewNBand returns a NR Band given an integer band number.
func NewNBand(band int) Band {
	return Band(fmt.Sprintf("n%d", band))
}

// Bandwidth represents the carrier bandwidth to set on the callbox.
type Bandwidth string

// Supported LTE only bandwidths.
const (
	Bandwidth1MHz = "1.4"
	Bandwidth2MHz = "2"
)

// Supported LTE & 5G bandwidths.
const (
	Bandwidth5Mhz  = "5"
	Bandwidth10MHz = "10"
	Bandwidth15MHz = "15"
	Bandwidth20MHz = "20"
)

// 5G only bandwidths.
const (
	Bandwidth25MHz  = "25"
	Bandwidth30MHz  = "30"
	Bandwidth35MHz  = "35"
	Bandwidth40MHz  = "40"
	Bandwidth45MHz  = "45"
	Bandwidth50MHz  = "50"
	Bandwidth60MHz  = "60"
	Bandwidth70MHz  = "70"
	Bandwidth80MHz  = "80"
	Bandwidth90MHz  = "90"
	Bandwidth100MHz = "100"
)

// CallboxHardware represents the type of callbox that the DUT is connected to.
type CallboxHardware string

// Supported callbox hardware types.
const (
	CallboxHardwareCMW = "CMW"
	CallboxHardwareCMX = "CMX"
)

// CellularTechnology represents cellular network technology.
type CellularTechnology string

const (
	// CellularTechnologyLTE represents an LTE cellular network.
	CellularTechnologyLTE CellularTechnology = "LTE"
	// CellularTechnologyWCDMA represents a WCDMA cellular network.
	CellularTechnologyWCDMA CellularTechnology = "WCDMA"
	// CellularTechnologyNR5GNSA represents a 5G NSA cellular network.
	CellularTechnologyNR5GNSA CellularTechnology = "NR5G_NSA"
	// CellularTechnologyNR5GSA represents a 5G SA cellular network.
	CellularTechnologyNR5GSA CellularTechnology = "NR5G_SA"
)

// IPAddressType represents a network IP type.
type IPAddressType string

// Supported IPAddressTypes
const (
	IPV4   = "IPV4"
	IPV6   = "IPV6"
	IPV4V6 = "IPV4V6"
)

// MimoMode represents a cellular MIMO configuration.
type MimoMode string

// Supported MIMO modes.
const (
	MimoMode1x1 MimoMode = "1x1"
	MimoMode2x2 MimoMode = "2x2"
	MimoMode4x4 MimoMode = "4x4"
)

// RxPower is a predefined callbox Rx (downlink) power level in dBm.
// Note: the CallboxManager expects RxPower to be passed as a string.
type RxPower string

const (
	// LteRxPowerExcellent is a predefined excellent downlink power level.
	LteRxPowerExcellent RxPower = "-88"
	// LteRxPowerHigh is a predefined high downlink power level.
	LteRxPowerHigh = "-98"
	// LteRxPowerMedium is a predefined medium downlink power level.
	LteRxPowerMedium = "-108"
	// LteRxPowerWeak is a predefined weak downlink power level.
	LteRxPowerWeak = "-118"
	// LteRxPowerDisconnected is a predefined disconnected downlink power level.
	LteRxPowerDisconnected = "-170"
)

// NewRxPower returns a RxPower from an exact value in dBm.
func NewRxPower(power float64) RxPower {
	return RxPower(fmt.Sprintf("%f", power))
}

// TransmissionMode represents the MIMO transmission scheme to use in the downlink.
type TransmissionMode string

// Supported transmission modes.
const (
	TransmissionMode1 = "1"
	TransmissionMode2 = "2"
	TransmissionMode3 = "3"
	TransmissionMode4 = "4"
	TransmissionMode5 = "5"
	TransmissionMode6 = "6"
	TransmissionMode7 = "7"
)

// TxPower is a predefined callbox Tx (uplink) power level in dBm.
// Note: the CallboxManager expects TxPower to be passed as a string.
type TxPower string

const (
	// LteTxPowerMax is a predefined max uplink power level.
	LteTxPowerMax TxPower = "24"
	// LteTxPowerHigh is a predefined high uplink power level.
	LteTxPowerHigh = "13"
	// LteTxPowerMedium is a predefined medium uplink power level.
	LteTxPowerMedium = "3"
	// LteTxPowerLow is a predefined low uplink power level.
	LteTxPowerLow = "-20"
)

// NewTxPower returns a TxPower from an exact value in dBm.
func NewTxPower(power float64) TxPower {
	return TxPower(fmt.Sprintf("%f", power))
}

// DRXConfiguration represents a cell's discontinuous reception settings.
type DRXConfiguration struct {
	OnDuration             int
	InactiveDuration       int
	RetransmissionDuration int
	LongCycle              int
	LongCycleOffset        int
}

// MarshalJSON marshalls DRXConfiguration as a JSON byte array.
func (d DRXConfiguration) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		// callbox controllers expect DRX to be passed as an array of values.
		[]int{
			d.OnDuration,
			d.InactiveDuration,
			d.RetransmissionDuration,
			d.LongCycle,
			d.LongCycleOffset,
		},
	)
}

// SchedulingMode represents the type of UE scheduling to use.
type SchedulingMode string

// Supported SchedulingModes.
const (
	SchedulingModeStatic  = "static"
	SchedulingModeDynamic = "dynamic"
)

// CellConfiguration represents the configuration options for a cellular base station.
// If multiple are provided, each configuration will be applied to the primary and secondary
// carriers in a carrier aggregation scenario, respectively.
type CellConfiguration struct {
	Band                   Band              `json:"band,omitempty"`
	Bandwidth              Bandwidth         `json:"bw,omitempty"`
	NRARFCN                int               `json:"nr_arfcn,omitempty"`
	Mimo                   MimoMode          `json:"mimo,omitempty"`
	RxPower                RxPower           `json:"pdl,omitempty"`
	TxPower                TxPower           `json:"pul,omitempty"`
	TransmissionMode       TransmissionMode  `json:"tm,omitempty"`
	DRX                    *DRXConfiguration `json:"drx,omitempty"`
	Downlink256QAM         bool              `json:"dl_256_qam_enabled,omitempty"`
	Scheduling             SchedulingMode    `json:"scheduling,omitempty"`
	DownlinkResourceBlocks int               `json:"dl_rbs,omitempty"`
	UplinkResourceBlocks   int               `json:"ul_rbs,omitempty"`
	DownlinkMCS            int               `json:"dlmcs,omitempty"`
	UplinkMCS              int               `json:"ulmcs,omitempty"`
	TrackingAreaCode       int               `json:"tracking_area,omitempty"`
}

// CellOption represents a configuration option for a base station cell/component carrier.
type CellOption func(opt *CellConfiguration)

// BandOption configures the cell frequency band.
func BandOption(band int) CellOption {
	return func(opt *CellConfiguration) {
		opt.Band = NewBand(band)
	}
}

// NBandOption configures the cell frequency band as a NR band.
func NBandOption(band int) CellOption {
	return func(opt *CellConfiguration) {
		opt.Band = NewNBand(band)
	}
}

// BandwidthOption configures the cell bandwidth.
func BandwidthOption(bw Bandwidth) CellOption {
	return func(opt *CellConfiguration) {
		opt.Bandwidth = bw
	}
}

// AntennaOption configures the cell MIMO and transmission mode.
func AntennaOption(mimo MimoMode, tm TransmissionMode) CellOption {
	return func(opt *CellConfiguration) {
		opt.Mimo = mimo
		opt.TransmissionMode = tm
	}
}

// RxPowerOption configures the cell downlink power.
func RxPowerOption(power RxPower) CellOption {
	return func(opt *CellConfiguration) {
		opt.RxPower = power
	}
}

// TxPowerOption configures the cell uplink power.
func TxPowerOption(power TxPower) CellOption {
	return func(opt *CellConfiguration) {
		opt.TxPower = power
	}
}

// DRXOption configures the cell discontinuous reception settings.
func DRXOption(DRX *DRXConfiguration) CellOption {
	return func(opt *CellConfiguration) {
		opt.DRX = DRX
	}
}

// NRARFCNOption configures the cell NR ARFCN (frequency channel).
func NRARFCNOption(nrARFCN int) CellOption {
	return func(opt *CellConfiguration) {
		opt.NRARFCN = nrARFCN
	}
}

// SchedulingOption configures the cells scheduling settings.
func SchedulingOption(dlRB, ulRB, dlMCS, ulMCS int) CellOption {
	return func(opt *CellConfiguration) {
		opt.Scheduling = SchedulingModeStatic
		opt.DownlinkResourceBlocks = dlRB
		opt.UplinkResourceBlocks = ulRB
		opt.DownlinkMCS = dlMCS
		opt.UplinkMCS = ulMCS
	}
}

// TrackingAreaOption configures the cells tracking area.
func TrackingAreaOption(tac int) CellOption {
	return func(opt *CellConfiguration) {
		opt.TrackingAreaCode = tac
	}
}

// NewLteCellConfiguration returns a default configuration for an LTE cell with the optional overrides.
func NewLteCellConfiguration(options ...CellOption) CellConfiguration {
	config := CellConfiguration{
		Band:                   NewBand(3),
		Bandwidth:              Bandwidth20MHz,
		Mimo:                   MimoMode2x2,
		TransmissionMode:       TransmissionMode3,
		RxPower:                LteRxPowerExcellent,
		TxPower:                LteTxPowerMax,
		Scheduling:             SchedulingModeStatic,
		DownlinkResourceBlocks: 100,
		UplinkResourceBlocks:   100,
		DownlinkMCS:            28,
		UplinkMCS:              23,
	}

	for _, opt := range options {
		opt(&config)
	}

	return config
}

// New5GNSACellConfiguration returns a default configuration for an LTE cell with the optional overrides.
func New5GNSACellConfiguration(options ...CellOption) CellConfiguration {
	config := CellConfiguration{
		Band:                   NewNBand(78),
		Bandwidth:              Bandwidth100MHz,
		Mimo:                   MimoMode2x2,
		TransmissionMode:       TransmissionMode3,
		TxPower:                LteTxPowerMax,
		RxPower:                LteRxPowerExcellent,
		Scheduling:             SchedulingModeStatic,
		DownlinkResourceBlocks: 273,
		UplinkResourceBlocks:   273,
		DownlinkMCS:            28,
		UplinkMCS:              23,
	}

	for _, opt := range options {
		opt(&config)
	}

	return config
}

// NetworkConfiguration represents a set of additional callbox network options.
type NetworkConfiguration struct {
	APN    string        `json:"apn,omitempty"`
	MTU    int           `json:"mtu,omitempty"`
	IPType IPAddressType `json:"ip_type,omitempty"`
}

// NewNetworkConfiguration a NetworkConfiguration with the provided settings.
func NewNetworkConfiguration(apn string, mtu int, ipType IPAddressType) *NetworkConfiguration {
	return &NetworkConfiguration{
		APN:    apn,
		MTU:    mtu,
		IPType: ipType,
	}
}

// ConfigureCallboxRequestBody is the request body for ConfigureCallbox requests.
type ConfigureCallboxRequestBody struct {
	Callbox      string                `json:"callbox,omitempty"`
	Hardware     CallboxHardware       `json:"hardware,omitempty"`
	CellularType CellularTechnology    `json:"cellular_type,omitempty"`
	Parameters   []CellConfiguration   `json:"configuration,omitempty"`
	Network      *NetworkConfiguration `json:"network,omitempty"`
}

// BeginSimulationRequestBody is the request body for BeginSimulation requests.
type BeginSimulationRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// ConfigureRxPowerRequestBody is the request body for ConfigureRxPower requests.
type ConfigureRxPowerRequestBody struct {
	Callbox string  `json:"callbox,omitempty"`
	Power   RxPower `json:"pdl,omitempty"`
}

// ConfigureTxPowerRequestBody is the request body for ConfigureTxPower requests.
type ConfigureTxPowerRequestBody struct {
	Callbox string  `json:"callbox,omitempty"`
	Power   TxPower `json:"pul,omitempty"`
}

// FetchTxPowerRequestBody is the request body for FetchTxPower requests.
type FetchTxPowerRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchTxPowerResponseBody is the response body for FetchTRxPower requests.
type FetchTxPowerResponseBody struct {
	Power float64 `json:"pul,omitempty"`
}

// FetchRxPowerRequestBody is the request body for FetchRxPower requests.
type FetchRxPowerRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchRxPowerResponseBody is the response body for FetchRxPower requests.
type FetchRxPowerResponseBody struct {
	Power float64 `json:"pdl,omitempty"`
}

// SendSmsRequestBody is the request body for SendSms requests.
type SendSmsRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
	Message string `json:"sms,omitempty"`
}

// ConfigureIperfRequestBody is the request body for iperf configuration requests.
type ConfigureIperfRequestBody struct {
	Callbox    string              `json:"callbox,omitempty"`
	Time       int                 `json:"time,omitempty"`
	PacketSize int                 `json:"psize,omitempty"`
	Clients    []IperfClientConfig `json:"clients,omitempty"`
	Servers    []IperfServerConfig `json:"servers,omitempty"`
}

// IperfClientConfig is a configuration to use with a callbox Iperf client instance.
type IperfClientConfig struct {
	IP                  string  `json:"ip,omitempty"`
	Port                int     `json:"port,omitempty"`
	Protocol            string  `json:"proto,omitempty"`
	WindowSize          int64   `json:"wsize,omitempty"`
	ParallelConnections int     `json:"pconnections,omitempty"`
	MaxBitRate          float64 `json:"mbitrate,omitempty"`
}

// IperfServerConfig is a configuration to use with a callbox Iperf server instance.
type IperfServerConfig struct {
	IP         string `json:"ip,omitempty"`
	Port       int    `json:"port,omitempty"`
	Protocol   string `json:"proto,omitempty"`
	WindowSize int64  `json:"wsize,omitempty"`
}

// StartIperfRequestBody is the request body for iperf start requests.
type StartIperfRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// StopIperfRequestBody is the request body for Iperf stop requests.
type StopIperfRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// CloseIperfRequestBody is the request body for Iperf close requests.
type CloseIperfRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchIperfResultRequestBody is the request body for Iperf results query requests.
type FetchIperfResultRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchIperfResultResponseBody is the response body for an Iperf result query requests.
type FetchIperfResultResponseBody struct {
	Clients []*IperfClientResult `json:"clients"`
	Servers []*IperfServerResult `json:"servers"`
}

// IperfServerResult is the current Iperf measurement for an Iperf server instance.
type IperfServerResult struct {
	ID          int     `json:"counter"`
	Throughput  float64 `json:"throughput"`
	PercentLoss float64 `json:"loss"`
}

// IperfClientResult is the current Iperf measurement for an Iperf client instance.
type IperfClientResult struct {
	ID         int     `json:"counter"`
	Throughput float64 `json:"throughput"`
}

// FetchIperfIPRequestBody is the request body for an Iperf IP query request.
type FetchIperfIPRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchIperfIPResponseBody is the response body for an Iperf IP query request.
type FetchIperfIPResponseBody struct {
	IP string `json:"ip"`
}

// FetchMaxThroughputRequestBody is the request body for a maximum throughput request.
type FetchMaxThroughputRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchMaxThroughputResponseBody is the request body for a maximum throughput request.
type FetchMaxThroughputResponseBody struct {
	Uplink   float64 `json:"uplink"`
	Downlink float64 `json:"downlink"`
}

// ConfigureTxMeasurementRequestBody is the request body for a Tx measurement configuration request.
type ConfigureTxMeasurementRequestBody struct {
	Callbox     string `json:"callbox,omitempty"`
	SampleCount int32  `json:"sample_count,omitempty"`
}

// RunTxMeasurementRequestBody is the request body for a Tx measurement run request.
type RunTxMeasurementRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchTxMeasurementRequestBody is the request body for a Tx measurement result query requests.
type FetchTxMeasurementRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// FetchTxMeasurementResponseBody is the response body for a Tx measurement result query requests.
type FetchTxMeasurementResponseBody struct {
	Min               float64 `json:"min"`
	Max               float64 `json:"max"`
	Average           float64 `json:"average"`
	StandardDeviation float64 `json:"stdev"`
}

// StopTxMeasurementRequestBody is the request body for Tx measurement stop requests.
type StopTxMeasurementRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// CloseTxMeasurementRequestBody is the request body for Tx measurement close requests.
type CloseTxMeasurementRequestBody struct {
	Callbox string `json:"callbox,omitempty"`
}

// HandoverRequestBody is the request body for an inter/intra-RAT handover.
type HandoverRequestBody struct {
	Callbox            string             `json:"callbox,omitempty"`
	Band               int                `json:"band,omitempty"`
	Channel            int                `json:"channel,omitempty"`
	Bandwidth          float64            `json:"bw,omitempty"`
	SecondaryBand      int                `json:"secondary_band,omitempty"`
	SecondaryChannel   int                `json:"secondary_channel,omitempty"`
	SecondaryBandwidth float64            `json:"secondary_bw,omitempty"`
	Destination        CellularTechnology `json:"technology,omitempty"`
}
