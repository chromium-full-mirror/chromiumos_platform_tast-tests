// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mmconst defines the constants for ModemManager1,
// This is defined under common/ as they might be used in both
// local and remote tests.
package mmconst

import "time"

// ModemManager1.Modem properties
const (
	ModemPropertyBearers                     = "Bearers"
	ModemPropertyDevice                      = "Device"
	ModemPropertyEquipmentIdentifier         = "EquipmentIdentifier"
	ModemPropertyManufacturer                = "Manufacturer"
	ModemPropertyMaxActiveMultiplexedBearers = "MaxActiveMultiplexedBearers"
	ModemPropertyOwnNumbers                  = "OwnNumbers"
	ModemPropertyPowered                     = "PowerState"
	ModemPropertyPrimarySimSlot              = "PrimarySimSlot"
	ModemPropertyRevision                    = "Revision"
	ModemPropertySim                         = "Sim"
	ModemPropertySimSlots                    = "SimSlots"
	ModemPropertyState                       = "State"
	ModemPropertyMessages                    = "Messages"
)

// ModemManager1.Modem.Modem3gpp properties
const (
	ModemModem3gppPropertyInitialEpsBearer = "InitialEpsBearer"
	ModemModem3gppPropertyOperatorCode     = "OperatorCode"
)

// ModemManager1.Modem.Modem3gpp.ProfileManager properties
const (
	ModemProfileManagerList = "List"
)

// ModemManager1.Modem.Simple properties
const (
	SimpleModemPropertyState    = "state"
	SimpleModemPropertyRegState = "m3gpp-registration-state"
)

// ModemManager1.Modem.Signal properties
const (
	SignalPropertyLte     = "Lte"
	SignalPropertyLteRsrp = "rsrp"
	SignalPropertyLteRsrq = "rsrq"
	SignalPropertyLteSnr  = "snr"
)

// ModemManager1.Sim properties
const (
	SimPropertySimIMSI               = "Imsi"
	SimPropertySimIdentifier         = "SimIdentifier"
	SimPropertySimEid                = "Eid"
	SimPropertyESimStatus            = "EsimStatus"
	SimPropertySimOperatorIdentifier = "OperatorIdentifier"
)

// ModemManager1.Bearer properties
const (
	BearerPropertyBearerType      = "BearerType"
	BearerPropertyConnected       = "Connected"
	BearerPropertyConnectionError = "ConnectionError"
	BearerPropertyInterface       = "Interface"
	BearerPropertyIP4Config       = "Ip4Config"
	BearerPropertyIP6Config       = "Ip6Config"
	BearerPropertyMultiplexed     = "Multiplexed"
	BearerPropertyProperties      = "Properties"
	// APN related properties
	BearerPropertyAllowRoaming   = "allow-roaming"
	BearerPropertyAllowedAuth    = "allowed-auth"
	BearerPropertyApn            = "apn"
	BearerPropertyApnType        = "apn-type"
	BearerPropertyIPType         = "ip-type"
	BearerPropertyMultiplex      = "multiplex"
	BearerPropertyPassword       = "password"
	BearerPropertyUser           = "user"
	BearerPropertyProfileEnabled = "profile-enabled"
	BearerPropertyProfileID      = "profile-id"
	BearerPropertyProfileName    = "profile-name"
	BearerPropertyProfileSource  = "profile-source"
	// IPConfig related properties
	BearerPropertyIPMethod  = "method"
	BearerPropertyIPAddress = "address"
	BearerPropertyIPPrefix  = "prefix"
	BearerPropertyIPDns1    = "dns1"
	BearerPropertyIPDns2    = "dns2"
	BearerPropertyIPDns3    = "dns3"
	BearerPropertyIPGateway = "gateway"
	BearerPropertyIPMtu     = "mtu"
)

// BearerIPMethod IP method types from ModemManager-enums.h
type BearerIPMethod uint32

// All the bearer IP method types
const (
	BearerIPMethodNone   BearerIPMethod = 0
	BearerIPMethodPPP    BearerIPMethod = 1
	BearerIPMethodStatic BearerIPMethod = 2
	BearerIPMethodDHCP   BearerIPMethod = 3
)

// BearerAllowedAuth Allowed authentication types from Modemmanager-enums.h
type BearerAllowedAuth uint32

// All the bearer authentication types
const (
	BearerAllowedAuthUnknown  BearerAllowedAuth = 0
	BearerAllowedAuthNone     BearerAllowedAuth = 1 << 0
	BearerAllowedAuthPAP      BearerAllowedAuth = 1 << 1
	BearerAllowedAuthCHAP     BearerAllowedAuth = 1 << 2
	BearerAllowedAuthMSChap   BearerAllowedAuth = 1 << 3
	BearerAllowedAuthMSChapV2 BearerAllowedAuth = 1 << 4
	BearerAllowedAuthEAP      BearerAllowedAuth = 1 << 5
)

// BearerIPFamily IP families from Modemmanager-enums.h
type BearerIPFamily uint32

// All the bearer IP families
const (
	BearerIPFamilyNone   BearerIPFamily = 0
	BearerIPFamilyIPv4   BearerIPFamily = 1 << 0
	BearerIPFamilyIPv6   BearerIPFamily = 1 << 1
	BearerIPFamilyIPv4v6 BearerIPFamily = 1 << 2
	BearerIPFamilyAny    BearerIPFamily = 0xFFFFFFFF
)

// BearerMultiplexSupport Multiplex support options from Modemmanager-enums.h
type BearerMultiplexSupport uint32

// All the bearer multiplex options
const (
	BearerMultiplexSupportUnknown   BearerMultiplexSupport = 0
	BearerMultiplexSupportNone      BearerMultiplexSupport = 1
	BearerMultiplexSupportRequested BearerMultiplexSupport = 2
	BearerMultiplexSupportRequired  BearerMultiplexSupport = 3
)

// BearerProfileSource Profile source from Modemmanager-enums.h
type BearerProfileSource uint32

// All the bearer profile sources
const (
	BearerProfileSourceUnknown  BearerProfileSource = 0
	BearerProfileSourceAdmin    BearerProfileSource = 1
	BearerProfileSourceUser     BearerProfileSource = 2
	BearerProfileSourceOperator BearerProfileSource = 3
	BearerProfileSourceModem    BearerProfileSource = 4
	BearerProfileSourceDevice   BearerProfileSource = 5
)

// BearerAPNType APN types options from Modemmanager-enums.h
type BearerAPNType uint32

// Most common bearer apn types
const (
	BearerAPNTypeNone      BearerAPNType = 0
	BearerAPNTypeInitial   BearerAPNType = 1 << 0
	BearerAPNTypeDefault   BearerAPNType = 1 << 1
	BearerAPNTypeIMS       BearerAPNType = 1 << 2
	BearerAPNTypeTethering BearerAPNType = 1 << 13
)

// Wait times for modem at Modemmanager operations
const (
	ModemPollTime = 1 * time.Minute
)

// D-Bus path for empty sim slots
const (
	EmptySlotPath = "/"
)

// ModemState states from Modemmanager-enums.h
type ModemState int32

// All the modem states
const (
	ModemStateFailed        ModemState = -1
	ModemStateUnknown       ModemState = 0
	ModemStateInitializing  ModemState = 1
	ModemStateLocked        ModemState = 2
	ModemStateDisabled      ModemState = 3
	ModemStateDisabling     ModemState = 4
	ModemStateEnabling      ModemState = 5
	ModemStateEnabled       ModemState = 6
	ModemStateSearching     ModemState = 7
	ModemStateRegistered    ModemState = 8
	ModemStateDisconnecting ModemState = 9
	ModemStateConnecting    ModemState = 10
	ModemStateConnected     ModemState = 11
)

// ModemRegState for registration states
type ModemRegState uint32

// All the 3gpp registration states
const (
	ModemRegStateIdle      ModemRegState = 0
	ModemRegStateHome      ModemRegState = 1
	ModemRegStateSearching ModemRegState = 2
	ModemRegStateDenied    ModemRegState = 3
	ModemRegStateUnknown   ModemRegState = 4
	ModemRegStateRoaming   ModemRegState = 5
)

// ModemPowerState is states of MMModemPowerState
type ModemPowerState uint32

// All the modem power states
const (
	ModemPowerStateUnknown ModemPowerState = 0
	ModemPowerStateOff     ModemPowerState = 1
	ModemPowerStateLow     ModemPowerState = 2
	ModemPowerStateOn      ModemPowerState = 3
)

// Modem DBus methods
const (
	// Modem interface methods
	ModemDeleteBearer = "DeleteBearer"
	ModemEnable       = "Enable"

	// Modem.Simple interface methods
	ModemConnect    = "Connect"
	ModemDisconnect = "Disconnect"
	ModemSetLogging = "SetLogging"
)

// Modem Sar DBus methods
const (
	ModemSAREnable        = "Enable"
	ModemSARSetPowerLevel = "SetPowerLevel"
	ModemSARState         = "State"
	ModemSARPowerLevel    = "PowerLevel"
)

// Modem Messages DBus methods
const (
	MessagesDelete = "Delete"
)

// Modem Carrier Lock DBus method
const (
	ModemSetCarrierLock = "SetCarrierLock"
)

// Default SIM pin
const (
	DefaultSimPin = "1111"
	TempSimPin    = "1600"
)

// Possible values for ESimStatus
const (
	ESimStatusUnknown      = 0
	ESimStatusNoProfile    = 1
	ESimStatusWithProfiles = 2
)
