// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shillconst defines the constants of shill service.
// This is defined under common/ as they might be used in both
// local and remote tests.
package shillconst

import (
	"time"

	"github.com/godbus/dbus/v5"
)

// Timeout constants for shill tests.
const (
	DefaultTimeout = 30 * time.Second
)

// ServiceProviderOverridePath isth path of the modb file to override serviceproviders.pbf
const ServiceProviderOverridePath = "/var/cache/shill/serviceproviders-exclusive-override.pbf"

// Type values defined in dbus-constants.h
// The values are used both for Service type and Technology type.
const (
	TypeEthernet = "ethernet"
	TypeWifi     = "wifi"
	TypeCellular = "cellular"
	TypeVPN      = "vpn"
	TypePPPoE    = "pppoe"
)

// Device property names defined in dbus-constants.h .
const (
	// Device property names.
	DevicePropertyAddress         = "Address"
	DevicePropertyDBusObject      = "DBus.Object"
	DevicePropertyInhibited       = "Inhibited"
	DevicePropertyInterface       = "Interface"
	DevicePropertyIPConfigs       = "IPConfigs"
	DevicePropertyName            = "Name"
	DevicePropertyPowered         = "Powered"
	DevicePropertyType            = "Type"
	DevicePropertySelectedService = "SelectedService"

	// Cellular device property names.
	DevicePropertyCellularAPNList                     = "Cellular.APNList"
	DevicePropertyCellularFirmwareRevision            = "Cellular.FirmwareRevision"
	DevicePropertyCellularHomeProvider                = "Cellular.HomeProvider"
	DevicePropertyCellularICCID                       = "Cellular.ICCID"
	DevicePropertyCellularIMEI                        = "Cellular.IMEI"
	DevicePropertyCellularIMSI                        = "Cellular.IMSI"
	DevicePropertyCellularMDN                         = "Cellular.MDN"
	DevicePropertyCellularPolicyAllowRoaming          = "Cellular.PolicyAllowRoaming"
	DevicePropertyCellularSIMPresent                  = "Cellular.SIMPresent"
	DevicePropertyCellularSIMSlotInfo                 = "Cellular.SIMSlotInfo"
	DevicePropertyCellularSIMLockStatus               = "Cellular.SIMLockStatus"
	DevicePropertyCellularProviderRequiresRoaming     = "Cellular.ProviderRequiresRoaming"
	DevicePropertyCellularPrimaryMultiplexedInterface = "Cellular.PrimaryMultiplexedInterface"

	// Keys into the dictionaries exposed as properties
	DevicePropertyCellularSIMLockStatusLockType    = "LockType"
	DevicePropertyCellularSIMLockStatusLockEnabled = "LockEnabled"
	DevicePropertyCellularSIMLockStatusRetriesLeft = "RetriesLeft"

	// Valid values taken by properties exposed by shill.
	DevicePropertyValueSIMLockTypePIN    = "sim-pin"
	DevicePropertyValueSIMLockTypePUK    = "sim-puk"
	DevicePropertyValueSIMLockTypeNetPIN = "network-pin"

	// Ethernet device property names.
	DevicePropertyEthernetBusType   = "Ethernet.DeviceBusType"
	DevicePropertyEthernetLinkUp    = "Ethernet.LinkUp"
	DevicePropertyEthernetMACSource = "Ethernet.UsbEthernetMacAddressSource"
	DevicePropertyEapDetected       = "EapAuthenticatorDetected"
	DevicePropertyEapCompleted      = "EapAuthenticationCompleted"

	// WiFi device property names.
	DevicePropertyLastWakeReason             = "LastWakeReason"
	DevicePropertyMACAddrRandomEnabled       = "MACAddressRandomizationEnabled"
	DevicePropertyMACAddrRandomSupported     = "MACAddressRandomizationSupported"
	DevicePropertyNetDetectScanPeriodSeconds = "NetDetectScanPeriodSeconds"
	DevicePropertyScanning                   = "Scanning" // Also for cellular.
	DevicePropertyWakeOnWiFiAllowed          = "WakeOnWiFiAllowed"
	DevicePropertyWakeOnWiFiFeaturesEnabled  = "WakeOnWiFiFeaturesEnabled"
	DevicePropertyWiFiBgscanMethod           = "BgscanMethod"
	DevicePropertyWiFiBgscanShortInterval    = "BgscanShortInterval"
	DevicePropertyWiFiScanInterval           = "ScanInterval"
)

// IPConfig property names.
const (
	IPConfigPropertyAddress                   = "Address"
	IPConfigPropertyNameServers               = "NameServers"
	IPConfigPropertyBroadcast                 = "Broadcast"
	IPConfigPropertyDomainName                = "DomainName"
	IPConfigPropertyGateway                   = "Gateway"
	IPConfigPropertyMethod                    = "Method"
	IPConfigPropertyMtu                       = "Mtu"
	IPConfigPropertyPeerAddress               = "PeerAddress"
	IPConfigPropertyPrefixlen                 = "Prefixlen"
	IPConfigPropertySearchDomains             = "SearchDomains"
	IPConfigPropertyVendorEncapsulatedOptions = "VendorEncapsulatedOptions"
	IPConfigPropertyWebProxyAutoDiscoveryURL  = "WebProxyAutoDiscoveryUrl"
	IPConfigPropertyiSNSOptionData            = "iSNSOptionData"
)

// Manager property names.
const (
	ManagerPropertyActiveProfile                      = "ActiveProfile"
	ManagerPropertyAvailableTechnologies              = "AvailableTechnologies"
	ManagerPropertyDevices                            = "Devices"
	ManagerPropertyDHCPHostname                       = "DHCPProperty.Hostname"
	ManagerPropertyEnabledTechnologies                = "EnabledTechnologies"
	ManagerPropertyExperimentalTetheringFunctionality = "ExperimentalTetheringFunctionality"
	ManagerPropertyProfiles                           = "Profiles"
	ManagerPropertyProhibitedTechnologies             = "ProhibitedTechnologies"
	ManagerPropertyServices                           = "Services"
	ManagerTetheringAllowed                           = "TetheringAllowed"
	ManagerPropertyServiceCompleteList                = "ServiceCompleteList"
	ManagerPropertyTetheringAllowed                   = "TetheringAllowed"
	ManagerPropertyTetheringCapabilities              = "TetheringCapabilities"
	ManagerPropertyGlobalFTEnabled                    = "WiFi.GlobalFTEnabled"
	ManagerPropertyRequestScanType                    = "WiFi.RequestScanType"
	ManagerPropertyScanAllowRoam                      = "WiFi.ScanAllowRoam"
	ManagerPropertyDOHProviders                       = "DNSProxyDOHProviders"
	ManagerPropertyPortalFallbackHTTPSURLs            = "PortalFallbackHttpsUrls"
	ManagerPropertyPortalHTTPSURL                     = "PortalHttpsUrl"
	ManagerPropertyDefaultService                     = "DefaultService"
	ManagerPropertyDefaultTechnology                  = "DefaultTechnology"
	ManagerPropertyTetheringConfig                    = "TetheringConfig"
	ManagerPropertyEnableDHCPQos                      = "EnableDHCPQoS"
	ManagerPropertyP2PAllowed                         = "P2PAllowed"
	ManagerPropertyP2PCapabilities                    = "P2PCapabilities"
	ManagerPropertyP2PGroupInfos                      = "P2PGroupInfos"
	ManagerPropertyP2PClientInfos                     = "P2PClientInfos"
)

// Service property names defined in dbus-constants.h .
const (
	// Service property names.
	ServicePropertyConnectable       = "Connectable"
	ServicePropertyDevice            = "Device"
	ServicePropertyError             = "Error"
	ServicePropertyName              = "Name"
	ServicePropertyType              = "Type"
	ServicePropertyIPConfig          = "IPConfig"
	ServicePropertyIsConnected       = "IsConnected"
	ServicePropertyMode              = "Mode"
	ServicePropertyState             = "State"
	ServicePropertyStaticIPConfig    = "StaticIPConfig"
	ServicePropertyStrength          = "Strength"
	ServicePropertyVisible           = "Visible"
	ServicePropertyAutoConnect       = "AutoConnect"
	ServicePropertyGUID              = "GUID"
	ServicePropertyProvider          = "Provider"
	ServicePropertyPriority          = "Priority"
	ServicePropertyProxyConfig       = "ProxyConfig"
	ServicePropertyEphemeralPriority = "EphemeralPriority"
	ServicePropertyCheckPortal       = "CheckPortal"
	ServicePropertyONCSource         = "ONCSource"

	// Cellular service property names.
	ServicePropertyCellularEID               = "Cellular.EID"
	ServicePropertyCellularICCID             = "Cellular.ICCID"
	ServicePropertyCellularAllowRoaming      = "Cellular.AllowRoaming"
	ServicePropertyCellularAPN               = "Cellular.APN"
	ServicePropertyCellularLastGoodAPN       = "Cellular.LastGoodAPN"
	ServicePropertyCellularLastAttachAPN     = "Cellular.LastAttachAPN"
	ServicePropertyCellularNetworkTechnology = "Cellular.NetworkTechnology"
	ServicePropertyCellularRoamingState      = "Cellular.RoamingState"
	ServicePropertyCellularServingOperator   = "Cellular.ServingOperator"
	ServicePropertyCellularCustomAPNList     = "Cellular.CustomAPNList"
	ServicePropertyCellularActivationState   = "Cellular.ActivationState"

	// Keys into the dictionaries exposed as properties for LastAttachAPN and LastGoodAPN
	DevicePropertyCellularAPNInfoApnName             = "apn"
	DevicePropertyCellularAPNInfoApnSource           = "apn_source"
	DevicePropertyCellularAPNInfoApnTypes            = "apn_types"
	DevicePropertyCellularAPNInfoApnAttach           = "attach"
	DevicePropertyCellularAPNInfoApnIPType           = "ip_type"
	DevicePropertyCellularAPNInfoApnAuthentication   = "authentication"
	DevicePropertyCellularAPNInfoApnUsername         = "username"
	DevicePropertyCellularAPNInfoApnPassword         = "password"
	DevicePropertyCellularAPNInfoUserFriendlyApnName = "name"

	// APN related enums
	DevicePropertyCellularAPNInfoApnIPTypeIPv4         = "ipv4"
	DevicePropertyCellularAPNInfoApnIPTypeIPv4v6       = "ipv4v6"
	DevicePropertyCellularAPNInfoApnIPTypeIPv6         = "ipv6"
	DevicePropertyCellularAPNInfoApnAttachTrue         = "attach"
	DevicePropertyCellularAPNInfoApnAuthenticationChap = "chap"
	DevicePropertyCellularAPNInfoApnAuthenticationPap  = "pap"
	DevicePropertyCellularAPNInfoApnTypeDefault        = "DEFAULT"
	DevicePropertyCellularAPNInfoApnTypeIA             = "IA"

	// WiFi service property names.
	ServicePropertyPassphrase          = "Passphrase"
	ServicePropertySecurity            = "Security"
	ServicePropertySecurityClass       = "SecurityClass"
	ServicePropertySSID                = "SSID"
	ServicePropertyWiFiBSSID           = "WiFi.BSSID"
	ServicePropertyWiFiBSSIDAllowlist  = "WiFi.BSSIDAllowlist"
	ServicePropertyWiFiBSSIDRequested  = "WiFi.BSSIDRequested"
	ServicePropertyWiFiFrequency       = "WiFi.Frequency"
	ServicePropertyWiFiFrequencyList   = "WiFi.FrequencyList"
	ServicePropertyWiFiHexSSID         = "WiFi.HexSSID"
	ServicePropertyWiFiHiddenSSID      = "WiFi.HiddenSSID"
	ServicePropertyWiFiRandomMACPolicy = "WiFi.RandomMACPolicy"
	ServicePropertyWiFiPhyMode         = "WiFi.PhyMode"
	ServicePropertyWiFiRekeyInProgress = "WiFi.RekeyInProgress"
	ServicePropertyWiFiRoamState       = "WiFi.RoamState"

	// EAP service property names.
	ServicePropertyEAPCACertPEM                   = "EAP.CACertPEM"
	ServicePropertyEAPMethod                      = "EAP.EAP"
	ServicePropertyEAPInnerEAP                    = "EAP.InnerEAP"
	ServicePropertyEAPIdentity                    = "EAP.Identity"
	ServicePropertyEAPPassword                    = "EAP.Password"
	ServicePropertyEAPPin                         = "EAP.PIN"
	ServicePropertyEAPCertID                      = "EAP.CertID"
	ServicePropertyEAPKeyID                       = "EAP.KeyID"
	ServicePropertyEAPKeyMgmt                     = "EAP.KeyMgmt"
	ServicePropertyEAPUseSystemCAs                = "EAP.UseSystemCAs"
	ServicePropertyEAPSubjectAlternativeNameMatch = "EAP.SubjectAlternativeNameMatch"
	ServicePropertyEAPDomainSuffixMatch           = "EAP.DomainSuffixMatch"
)

// Service Error values
const (
	ServiceErrorNoFailure = "no-failure"
)

// Service state values defined in dbus-constants.h
const (
	ServiceStateIdle              = "idle"
	ServiceStateCarrier           = "carrier"
	ServiceStateAssociation       = "association"
	ServiceStateConfiguration     = "configuration"
	ServiceStateReady             = "ready"
	ServiceStatePortal            = "portal"
	ServiceStateNoConnectivity    = "no-connectivity"
	ServiceStateRedirectFound     = "redirect-found"
	ServiceStateOffline           = "offline"
	ServiceStateOnline            = "online"
	ServiceStateDisconnect        = "disconnecting"
	ServiceStateFailure           = "failure"
	ServiceStateActivationFailure = "activation-failure"
)

// Service ONC Source values defined in dbus-constants.h
const (
	ServiceONCSourceUnknown      = "Unknown"
	ServiceONCSourceNone         = "None"
	ServiceONCSourceUserImport   = "UserImport"
	ServiceONCSourceDevicePolicy = "DevicePolicy"
	ServiceONCSourceUserPolicy   = "UserPolicy"
)

// Cellular network technology values defined in dbus-constants.h
const (
	CellularNetworkTechnologyLTE  = "LTE"
	CellularNetworkTechnologyUMTS = "UMTS"
	CellularNetworkTechnology5GNR = "5GNR"
)

// Roam state values defined in dbus-constants.h
const (
	RoamStateIdle          = "idle"
	RoamStateAssociation   = "association"
	RoamStateConfiguration = "configuration"
	RoamStateReady         = "ready"
)

// ServiceConnectedStates is a list of service states that are considered connected.
var ServiceConnectedStates = []interface{}{
	ServiceStatePortal,
	ServiceStateNoConnectivity,
	ServiceStateRedirectFound,
	ServiceStateOnline,
	ServiceStateReady,
}

// Portal Detector default values defined in portal_detector.h
const (
	PortalDetectorDefaultCheckPortalList = "ethernet,wifi,cellular"
)

// Security options defined in dbus-constants.h
const (
	SecurityNone               = "none"
	SecurityTransOWE           = "trans-owe"
	SecurityOWE                = "owe"
	SecurityWEP                = "wep"
	SecurityWPA                = "wpa"
	SecurityWPAWPA2            = "wpa+wpa2"
	SecurityWPAAll             = "wpa-all"
	SecurityWPA2               = "wpa2"
	SecurityWPA2WPA3           = "wpa2+wpa3"
	SecurityWPA3               = "wpa3"
	SecurityWPAEnterprise      = "wpa-ent"
	SecurityWPAWPA2Enterprise  = "wpa+wpa2-ent"
	SecurityWPAAllEnterprise   = "wpa-all-ent"
	SecurityWPA2Enterprise     = "wpa2-ent"
	SecurityWPA2WPA3Enterprise = "wpa2+wpa3-ent"
	SecurityWPA3Enterprise     = "wpa3-ent"
	SecurityClassNone          = "none"
	SecurityClassWEP           = "wep"
	SecurityClassPSK           = "psk"
	SecurityClass8021x         = "802_1x"
)

// MAC randomization policy constants defined in dbus-constants.h
const (
	MacPolicyHardware            = "Hardware"
	MacPolicyFullRandom          = "FullRandom"
	MacPolicyOUIRandom           = "OUIRandom"
	MacPolicyPersistentRandom    = "PersistentRandom"
	MacPolicyNonPersistentRandom = "NonPersistentRandom"
)

// ServiceKeyMgmtIEEE8021X is a value of EAPKeyMgmt.
const ServiceKeyMgmtIEEE8021X = "IEEE8021X"

const defaultStorageDir = "/var/cache/shill/"

// ApInterfaceName is the default interface name used to bring up AP on.
const ApInterfaceName = "ap0"

const (
	// DefaultProfileName is the name of default profile.
	DefaultProfileName = "default"
	// DefaultProfileObjectPath is the dbus object path of default profile.
	DefaultProfileObjectPath dbus.ObjectPath = "/profile/" + DefaultProfileName
	// DefaultProfilePath is the path of default profile.
	DefaultProfilePath = defaultStorageDir + DefaultProfileName + ".profile"
)

// Cellular Operator info values defined in dbus-constants.h
const (
	// OperatorUUIDKey is the unique identifier of the carrier in the shill DB.
	OperatorUUIDKey = "uuid"
	OperatorCode    = "code"
)

// Profile property names.
const (
	ProfilePropertyCheckPortalList           = "CheckPortalList"
	ProfilePropertyEntries                   = "Entries"
	ProfilePropertyName                      = "Name"
	ProfilePropertyPortalURL                 = "PortalURL"
	ProfilePropertyPortalCheckInterval       = "PortalCheckInterval"
	ProfilePropertyServices                  = "Services"
	ProfilePropertyUserHash                  = "UserHash"
	ProfilePropertyProhibitedTechnologies    = "ProhibitedTechnologies"
	ProfilePropertyArpGateway                = "ArpGateway"
	ProfilePropertyNoAutoConnectTechnologies = "NoAutoConnectTechnologies"
	ProfilePropertyAlwaysOnVPNService        = "AlwaysOnVpnService"
	ProfilePropertyAlwaysOnVPNMode           = "AlwaysOnVpnMode"
)

// Profile entry property names.
const (
	ProfileEntryPropertyName = "Name"
	ProfileEntryPropertyType = "Type"
)

// Device background scan methods.
// The values are from wpa_supplicant + "none" for no background scan.
// See:
//
//	https://w1.fi/cgit/hostap/plain/wpa_supplicant/wpa_supplicant.conf
//	platform2/shill/supplicant/wpa_supplicant.cc
const (
	DeviceBgscanMethodSimple = "simple"
	DeviceBgscanMethodLearn  = "learn"
	DeviceBgscanMethodNone   = "none"
)

// WakeOnWiFi features.
const (
	WakeOnWiFiFeaturesDarkConnect = "darkconnect"
	WakeOnWiFiFeaturesNone        = "none"
)

// LastWakeReason values.
const (
	WakeOnWiFiReasonDisconnect = "WiFi.Disconnect"
	WakeOnWiFiReasonPattern    = "WiFi.Pattern"
	WakeOnWiFiReasonSSID       = "WiFi.SSID"
	WakeOnWiFiReasonUnknown    = "Unknown"
)

// DBus Errors
const (
	ErrorMatchingServiceNotFound = "Matching service was not found"
	ErrorModemNotStarted         = "Modem not started"
	// For invalid pin errors without error code
	// QMI : failed to enter pin /device/rmnet_data0: Couldn't enable PIN...
	// MBIM: failed to enter pin /device/wwan0: Failure
	ErrorPinFailure = "failed to enter pin"
	// For error org.freedesktop.ModemManager1.Error.MobileEquipment.IncorrectPassword
	// and org.chromium.flimflam.Error.IncorrectPin
	// QMI : Message=Couldn't verify PIN: QMI protocol error (12): 'IncorrectPin'
	// MBIM: Message=Incorrect password
	ErrorIncorrectPin      = "IncorrectPin"
	ErrorIncorrectPassword = "Incorrect password"
	// For error org.freedesktop.ModemManager1.Error.MobileEquipment.SimPuk
	// and org.chromium.flimflam.Error.PinBlocked
	// QMI : Message=Couldn't verify PIN: QMI protocol error (35): 'PinBlocked'
	// MBIM: Message=SIM PUK required
	ErrorPinBlocked  = "PinBlocked"
	ErrorPukRequired = "SIM PUK required"
)

// Passpoint credentials property names.
const (
	PasspointCredentialsPropertyDomains            = "Domains"
	PasspointCredentialsPropertyRealm              = "Realm"
	PasspointCredentialsPropertyHomeOIs            = "HomeOIs"
	PasspointCredentialsPropertyRequiredHomeOIs    = "RequiredHomeOIs"
	PasspointCredentialsPropertyRoamingConsortia   = "RoamingConsortia"
	PasspointCredentialsPropertyMeteredOverride    = "MeteredOverride"
	PasspointCredentialsPropertyAndroidPackageName = "AndroidPackageName"
	PasspointCredentialsPropertyExpirationTime     = "ExpirationTimeMilliseconds"
)

// Default ICCID when ICCID is unknown. Defined in dbus-constants.h
const (
	UnknownICCID = "unknown-iccid"
)

// Scaled signal quality constant for shill cellular service.
// Set to 10 = less than 2 bars in the UI
const (
	CellularServiceMinSignalStrength = 10
)

// AlwaysOnVPNMode represents a value to configure always-on VPN.
type AlwaysOnVPNMode string

// Shill always on VPN values.
const (
	AlwaysOnVPNModeOff        AlwaysOnVPNMode = "off"
	AlwaysOnVPNModeStrict     AlwaysOnVPNMode = "strict"
	AlwaysOnVPNModeBestEffort AlwaysOnVPNMode = "best-effort"
)

// Tethering-related constants, as defined in dbus-constants.h
const (
	// Manager TetheringConfig dictionary key names.
	TetheringConfAutoDisable               = "auto_disable"
	TetheringConfBand                      = "band"
	TetheringConfMAR                       = "randomize_mac_address"
	TetheringConfPassphrase                = "passphrase"
	TetheringConfSecurity                  = "security"
	TetheringConfSSID                      = "ssid"
	TetheringConfUpstreamTech              = "upstream_technology"
	TetheringConfDownstreamDeviceForTest   = "downstream_device_for_test"
	TetheringConfDownstreamPhyIndexForTest = "downstream_phy_index_for_test"

	// Manager TetheringCapabilities dictionary key names.
	TetheringCapDownstream = "downstream_technologies"
	TetheringCapSecurity   = "wifi_security_modes"
	TetheringCapUpstream   = "upstream_technologies"

	// Manager TetheringStatus dictionary key names.
	TetheringStatusClientHostname  = "hostname"
	TetheringStatusClientIPv4      = "IPv4"
	TetheringStatusClientIPv6      = "IPv6"
	TetheringStatusClientMAC       = "MAC"
	TetheringStatusClients         = "active_clients"
	TetheringStatusDownstreamTech  = "downstream_technology"
	TetheringStatusIdleReason      = "idle_reason"
	TetheringStatusState           = "state"
	TetheringStatusUpstreamTech    = "upstream_technology"
	TetheringStatusUpstreamService = "upstream_service"

	// TetheringStatusIdleReason values.
	TetheringIdleReasonClientStop         = "client_stop"
	TetheringIdleReasonError              = "error"
	TetheringIdleReasonInactive           = "inactive"
	TetheringIdleReasonInitialState       = "initial_state"
	TetheringIdleReasonSuspend            = "suspend"
	TetheringIdleReasonUpstreamDisconnect = "upstream_disconnect"
	TetheringIdleReasonUserExit           = "user_exit"

	// TetheringStatusState values.
	TetheringStateActive   = "active"
	TetheringStateIdle     = "idle"
	TetheringStateStarting = "starting"

	// SetTetheringEnabled result values.
	TetheringEnableResultFailure              = "failure"
	TetheringEnableResultInvalidProperties    = "invalid_properties"
	TetheringEnableResultNotAllowed           = "not_allowed"
	TetheringEnableResultSuccess              = "success"
	TetheringEnableResultUpstreamNotAvailable = "upstream_not_available"

	// CheckTetheringReadinessFunction return status.
	TetheringReadinessNotAllowed                  = "not_allowed"
	TetheringReadinessNotAllowedByCarrier         = "not_allowed_by_carrier"
	TetheringReadinessNotAllowedOnFW              = "not_allowed_on_fw"
	TetheringReadinessNotAllowedOnVariant         = "not_allowed_on_variant"
	TetheringReadinessNotAllowedUserNotEntitled   = "not_allowed_user_not_entitled"
	TetheringReadinessReady                       = "ready"
	TetheringReadinessUpstreamNetworkNotAvailable = "upstream_network_not_available"

	// WiFi Band options.
	Band2GHz    = "2.4GHz"
	Band5GHz    = "5GHz"
	BandAll     = "all-bands"
	BandUnknown = "unknown"
)

// Top level properties for a Geolocation request.
const (
	GeoHomeMobileCountryCodeProperty = "homeMobileCountryCode"
	GeoHomeMobileNetworkCodeProperty = "homeMobileNetworkCode"
	GeoRadioTypePropertyProperty     = "radioType"
	GeoCellTowersProperty            = "cellTowers"
	GeoWifiAccessPointsProperty      = "wifiAccessPoints"
)

// WiFi access point property names.
const (
	GeoInfoGeoMacAddressProperty         = "macAddress"
	GeoInfoGeoChannelProperty            = "channel"
	GeoInfoGeoSignalToNoiseRatioProperty = "signalToNoiseRatio"
)

// Common property names for geolocation objects.
const (
	GeoInfoGeoAgeProperty = "age"
)

// WiFi RequestScan type constants defined in dbus-constants.h
const (
	WiFiRequestScanTypeActive  = "active"
	WiFiRequestScanTypeDefault = "default"
	WiFiRequestScanTypePassive = "passive"
)

// P2P device state values
const (
	P2PDeviceStateUninitialized       = "uninitialized"
	P2PDeviceStateReady               = "ready"
	P2PDeviceStateClientAssociating   = "client_associating"
	P2PDeviceStateClientConfiguring   = "client_configuring"
	P2PDeviceStateClientConnected     = "client_connected"
	P2PDeviceStateClientDisconnecting = "client_disconnecting"
	P2PDeviceStateGOStarting          = "go_starting"
	P2PDeviceStateGOConfiguring       = "go_configuring"
	P2PDeviceStateGOActive            = "go_active"
	P2PDeviceStateGOStopping          = "go_stopping"
)

// Manager kP2PCapabilitiesProperty dictionary key names.
const (
	P2PCapabilitiesP2PSupportedProperty      = "P2PSupported"
	P2PCapabilitiesGroupReadinessProperty    = "GroupReadiness"
	P2PCapabilitiesClientReadinessProperty   = "ClientReadiness"
	P2PCapabilitiesSupportedChannelsProperty = "SupportedChannels"
	P2PCapabilitiesPreferredChannelsProperty = "PreferredChannels"
)

// kP2PCapabilitiesGroupReadinessProperty values
const (
	P2PCapabilitiesGroupReadinessReady    = "ready"
	P2PCapabilitiesGroupReadinessNotReady = "not_ready"
)

// kP2PCapabilitiesClientReadinessProperty values
const (
	P2PCapabilitiesClientReadinessReady    = "ready"
	P2PCapabilitiesClientReadinessNotReady = "not_ready"
)

// Manager kP2PGroupInfosProperty dictionary key names.
const (
	P2PGroupInfoShillIDProperty     = "shill_id"
	P2PGroupInfoStateProperty       = "state"
	P2PGroupInfoSSIDProperty        = "ssid"
	P2PGroupInfoPassphraseProperty  = "passphrase"
	P2PGroupInfoBSSIDProperty       = "bssid"
	P2PGroupInfoIPv4AddressProperty = "ipv4_address"
	P2PGroupInfoIPv6AddressProperty = "ipv6_address"
	P2PGroupInfoMACAddressProperty  = "mac_address"
	P2PGroupInfoFrequencyProperty   = "frequency"
	P2PGroupInfoClientsProperty     = "clients"
)

// kP2PGroupInfoStateProperty values
const (
	P2PGroupInfoStateIdle        = "idle"
	P2PGroupInfoStateStarting    = "starting"
	P2PGroupInfoStateConfiguring = "configuring"
	P2PGroupInfoStateActive      = "active"
	P2PGroupInfoStateStopping    = "stopping"
)

// Manager kP2PGroupInfoClientsProperty dictionary key names
const (
	P2PGroupInfoClientMACAddressProperty  = "mac_address"
	P2PGroupInfoClientIPv4AddressProperty = "ipv4_address"
	P2PGroupInfoClientIPv6AddressProperty = "ipv6_address"
	P2PGroupInfoClientHostnameProperty    = "hostname"
	P2PGroupInfoClientVendorClassProperty = "vendor_class"
)

// Manager kP2PClientInfoProperty dictionary key names.
const (
	P2PClientInfoShillIDProperty     = "shill_id"
	P2PClientInfoStateProperty       = "state"
	P2PClientInfoSSIDProperty        = "ssid"
	P2PClientInfoPassphraseProperty  = "passphrase"
	P2PClientInfoGroupBSSIDProperty  = "group_bssid"
	P2PClientInfoIPv4AddressProperty = "ipv4_address"
	P2PClientInfoIPv6AddressProperty = "ipv6_address"
	P2PClientInfoMACAddressProperty  = "mac_address"
	P2PClientInfoFrequencyProperty   = "frequency"
	P2PClientInfoGroupOwnerProperty  = "group_owner"
)

// kP2PClientInfoStateProperty values
const (
	P2PClientInfoStateIdle          = "idle"
	P2PClientInfoStateAssociating   = "associating"
	P2PClientInfoStateConfiguring   = "configuring"
	P2PClientInfoStateConnected     = "connected"
	P2PClientInfoStateDisconnecting = "disconnecting"
)

// Manager kP2PClientInfoGroupOwnerProperty dictionary key names
const (
	P2PClientInfoGroupOwnerMACAddressProperty  = "mac_address"
	P2PClientInfoGroupOwnerIPv4AddressProperty = "ipv4_address"
	P2PClientInfoGroupOwnerIPv6AddressProperty = "ipv6_address"
	P2PClientInfoGroupOwnerHostnameProperty    = "hostname"
	P2PClientInfoGroupOwnerVendorClassProperty = "vendor_class"
)

// Manager CreateP2PGroup and ConnectToP2PGroup dictionary argument key names
const (
	P2PDeviceSSID       = "ssid"
	P2PDevicePassphrase = "passphrase"
	P2PDeviceFrequency  = "frequency"
	P2PDeviceBSSID      = "bssid"
	P2PDevicePriority   = "priority"
)

// P2P result dictionary key names
const (
	P2PDeviceShillID = "shill_id"
	P2PResultCode    = "result_code"
)

// Manager CreateP2PGroup result values
const (
	CreateP2PGroupResultSuccess                 = "success"
	CreateP2PGroupResultNotAllowed              = "not_allowed"
	CreateP2PGroupResultNotSupported            = "not_supported"
	CreateP2PGroupResultConcurrencyNotSupported = "concurrency_not_supported"
	CreateP2PGroupResultTimeout                 = "timeout"
	CreateP2PGroupResultFrequencyNotSupported   = "frequency_not_supported"
	CreateP2PGroupResultBadSSID                 = "bad_ssid"
	CreateP2PGroupResultOperationInProgress     = "operation_in_progress"
	CreateP2PGroupResultOperationFailed         = "operation_failed"
)

// Manager ConnectToP2PGroup result values
const (
	ConnectToP2PGroupResultSuccess                 = "success"
	ConnectToP2PGroupResultNotAllowed              = "not_allowed"
	ConnectToP2PGroupResultNotSupported            = "not_supported"
	ConnectToP2PGroupResultConcurrencyNotSupported = "concurrency_not_supported"
	ConnectToP2PGroupResultTimeout                 = "timeout"
	ConnectToP2PGroupResultAuthFailure             = "auth_failure"
	ConnectToP2PGroupResultFrequencyNotSupported   = "frequency_not_supported"
	ConnectToP2PGroupResultGroupNotFound           = "group_not_found"
	ConnectToP2PGroupResultAlreadyConnected        = "already_connected"
	ConnectToP2PGroupResultInvalidArguments        = "invalid_arguments"
	ConnectToP2PGroupResultOperationInProgress     = "operation_in_progress"
	ConnectToP2PGroupResultOperationFailed         = "operation_failed"
)

// Manager DestroyP2PGroup result values
const (
	DestroyP2PGroupResultSuccess             = "success"
	DestroyP2PGroupResultNotAllowed          = "not_allowed"
	DestroyP2PGroupResultNotSupported        = "not_supported"
	DestroyP2PGroupResultTimeout             = "timeout"
	DestroyP2PGroupResultNoGroup             = "no_group"
	DestroyP2PGroupResultOperationInProgress = "operation_in_progress"
	DestroyP2PGroupResultOperationFailed     = "operation_failed"
)

// Manager DisconnectFromP2PGroup result values
const (
	DisconnectFromP2PGroupResultSuccess             = "success"
	DisconnectFromP2PGroupResultNotAllowed          = "not_allowed"
	DisconnectFromP2PGroupResultNotSupported        = "not_supported"
	DisconnectFromP2PGroupResultTimeout             = "timeout"
	DisconnectFromP2PGroupResultNotConnected        = "not_connected"
	DisconnectFromP2PGroupResultOperationInProgress = "operation_in_progress"
	DisconnectFromP2PGroupResultOperationFailed     = "operation_failed"
)
