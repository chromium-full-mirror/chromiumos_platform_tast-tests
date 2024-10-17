// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package types provides interface types shared by healthd tast files.
package types

import (
	"go.chromium.org/tast-tests/cros/local/jsontypes"
)

// BusDevice represents the BusDevice in cros-healthd mojo interface.
type BusDevice struct {
	VendorName  string  `json:"vendor_name"`
	ProductName string  `json:"product_name"`
	DeviceClass string  `json:"device_class"`
	BusInfo     BusInfo `json:"bus_info"`
}

// BusInfo represents the BusInfo in cros-healthd mojo interface.
type BusInfo struct {
	PCIBusInfo         *PCIBusInfo         `json:"pci_bus_info"`
	USBBusInfo         *USBBusInfo         `json:"usb_bus_info"`
	ThunderboltBusInfo *ThunderboltBusInfo `json:"thunderbolt_bus_info"`
}

// PCIBusInfo represents the PciBusInfo in cros-healthd mojo interface.
type PCIBusInfo struct {
	ClassID     uint8   `json:"class_id"`
	SubClassID  uint8   `json:"subclass_id"`
	ProgIfID    uint8   `json:"prog_if_id"`
	VendorID    uint16  `json:"vendor_id"`
	DeviceID    uint16  `json:"device_id"`
	SubVendorID *uint16 `json:"sub_vendor_id"`
	SubDeviceID *uint16 `json:"sub_device_id"`
	Driver      *string `json:"driver"`
}

// USBBusInfo represents the UsbBusInfo in cros-healthd mojo interface.
type USBBusInfo struct {
	ClassID                  uint8                     `json:"class_id"`
	SubClassID               uint8                     `json:"subclass_id"`
	ProtocolID               uint8                     `json:"protocol_id"`
	VendorID                 uint16                    `json:"vendor_id"`
	ProductID                uint16                    `json:"product_id"`
	Interfaces               []USBInterfaceInfo        `json:"interfaces"`
	FwupdFirmwareVersionInfo *FwupdFirmwareVersionInfo `json:"fwupd_firmware_version_info"`
	Version                  string                    `json:"version"`
	SpecSpeed                string                    `json:"spec_speed"`
}

// USBInterfaceInfo represents the UsbInterfaceInfo in cros-healthd mojo
// interface.
type USBInterfaceInfo struct {
	InterfaceNumber uint8   `json:"interface_number"`
	ClassID         uint8   `json:"class_id"`
	SubClassID      uint8   `json:"subclass_id"`
	ProtocolID      uint8   `json:"protocol_id"`
	Driver          *string `json:"driver"`
}

// FwupdFirmwareVersionInfo represents the FwupdFirmwareVersionInfo in
// cros-healthd mojo interface.
type FwupdFirmwareVersionInfo struct {
	Version       string `json:"version"`
	VersionFormat string `json:"version_format"`
}

// ThunderboltInterfaceInfo represents the ThunderboltInterfaces in cros-healthd mojo
// interface.
type ThunderboltInterfaceInfo struct {
	Authorized      bool   `json:"authorized"`
	DeviceFwVersion string `json:"device_fw_version"`
	DeviceName      string `json:"device_name"`
	DeviceType      string `json:"device_type"`
	DeviceUUID      string `json:"device_uuid"`
	RxSpeedGbs      string `json:"rx_speed_gbs"`
	TxSpeedGbs      string `json:"tx_speed_gbs"`
	VendorName      string `json:"vendor_name"`
}

// ThunderboltBusInfo represents the ThunderboltBusInfo in cros-healthd mojo interface.
type ThunderboltBusInfo struct {
	SecurityLevel         string                     `json:"security_level"`
	ThunderboltInterfaces []ThunderboltInterfaceInfo `json:"thunderbolt_interfaces"`
}

// CPUInfo represents the CpuInfo in cros-healthd mojo interface.
type CPUInfo struct {
	Architecture        string                       `json:"architecture"`
	NumTotalThreads     jsontypes.Uint32             `json:"num_total_threads"`
	TemperatureChannels []TemperatureChannelInfo     `json:"temperature_channels"`
	PhysicalCPUs        []PhysicalCPUInfo            `json:"physical_cpus"`
	KeylockerInfo       *Keylockerinfo               `json:"keylocker_info"`
	Virtualization      VirtualizationInfo           `json:"virtualization"`
	Vulnerabilities     map[string]VulnerabilityInfo `json:"vulnerabilities"`
}

// TemperatureChannelInfo represents the CpuTemperatureChannel in cros-healthd mojo interface.
type TemperatureChannelInfo struct {
	Label              *string `json:"label"`
	TemperatureCelsius int32   `json:"temperature_celsius"`
}

// CStateInfo represents the CpuCStateInfo in cros-healthd mojo interface.
type CStateInfo struct {
	Name                       string           `json:"name"`
	TimeInStateSinceLastBootUs jsontypes.Uint64 `json:"time_in_state_since_last_boot_us"`
}

// LogicalCPUInfo represents the LogicalCpuInfo in cros-healthd mojo interface.
type LogicalCPUInfo struct {
	UserTimeUserHz             jsontypes.Uint64 `json:"user_time_user_hz"`
	SystemTimeUserHz           jsontypes.Uint64 `json:"system_time_user_hz"`
	MaxClockSpeedKhz           jsontypes.Uint32 `json:"max_clock_speed_khz"`
	ScalingMaxFrequencyKhz     jsontypes.Uint32 `json:"scaling_max_frequency_khz"`
	ScalingCurrentFrequencyKhz jsontypes.Uint32 `json:"scaling_current_frequency_khz"`
	IdleTimeUserHz             jsontypes.Uint64 `json:"idle_time_user_hz"`
	CStates                    []CStateInfo     `json:"c_states"`
	CoreID                     jsontypes.Uint32 `json:"core_id"`
}

// CPUVirtualizationInfo represents the CpuVirtualizationInfo in cros-healthd mojo interface.
type CPUVirtualizationInfo struct {
	Type      string `json:"type"`
	IsEnabled bool   `json:"is_enabled"`
	IsLocked  bool   `json:"is_locked"`
}

// PhysicalCPUInfo represents the PhysicalCpuInfo in cros-healthd mojo interface.
type PhysicalCPUInfo struct {
	ModelName         *string                `json:"model_name"`
	LogicalCPUs       []LogicalCPUInfo       `json:"logical_cpus"`
	Flags             []string               `json:"flags"`
	CPUVirtualization *CPUVirtualizationInfo `json:"cpu_virtualization"`
}

// Keylockerinfo represents the KeylockerInfo in cros-healthd mojo interface.
type Keylockerinfo struct {
	KeylockerConfigured bool `json:"keylocker_configured"`
}

// VirtualizationInfo represents the VirtualizationInfo in cros-healthd mojo interface.
type VirtualizationInfo struct {
	HasKvmDevice bool   `json:"has_kvm_device"`
	IsSmtActive  bool   `json:"is_smt_active"`
	SmtControl   string `json:"smt_control"`
}

// VulnerabilityInfo represents the VulnerabilityInfo in cros-healthd mojo interface.
type VulnerabilityInfo struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
