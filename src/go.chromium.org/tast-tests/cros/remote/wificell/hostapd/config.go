// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hostapd

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/hostap"
	"go.chromium.org/tast-tests/cros/common/wifi/iw"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/base"
	"go.chromium.org/tast/core/errors"
)

// ModeEnum is the type for specifying hostap mode.
type ModeEnum string

// Mode enums.
const (
	Mode80211a       ModeEnum = "a"
	Mode80211b       ModeEnum = "b"
	Mode80211g       ModeEnum = "g"
	Mode80211nMixed  ModeEnum = "n-mixed"
	Mode80211nPure   ModeEnum = "n-only"
	Mode80211acMixed ModeEnum = "ac-mixed"
	Mode80211acPure  ModeEnum = "ac-only"
	Mode80211axMixed ModeEnum = "ax-mixed"
	Mode80211axPure  ModeEnum = "ax-only"
	Mode80211beMixed ModeEnum = "be-mixed"
	Mode80211bePure  ModeEnum = "be-only"
)

// HTCap is the type for specifying HT capabilities in hostapd config (ht_capab=).
type HTCap int

// HTCap enums, use bitmask for ease of checking existence.
const (
	HTCapHT20      HTCap = 1 << iota // HTCaps string "" means HT20.
	HTCapHT40                        // auto-detect supported "[HT40-]" or "[HT40+]"
	HTCapHT40Minus                   // "[HT40-]"
	HTCapHT40Plus                    // "[HT40+]"
	HTCapSGI20                       // "[SHORT-GI-20]"
	HTCapSGI40                       // "[SHORT-GI-40]"
	HTCapLDPC                        // "[LDPC]"
	// The test APs don't support Greenfield now. Comment out the option to avoid usage.
	// (The capability can be shown with `iw phy`)
	// HTCapGreenfield                   // "[GF]"
)

// VHTCap is the type for specifying VHT capabilities in hostapd config (vht_capab=).
type VHTCap string

// Each capability can be simply mapped to a string.
const (
	VHTCapVHT160             VHTCap = "[VHT160]"
	VHTCapVHT16080Plus80     VHTCap = "[VHT160-80PLUS80]"
	VHTCapRXLDPC             VHTCap = "[RXLDPC]"
	VHTCapSGI80              VHTCap = "[SHORT-GI-80]"
	VHTCapSGI160             VHTCap = "[SHORT-GI-160]"
	VHTCapTxSTBC2BY1         VHTCap = "[TX-STBC-2BY1]"
	VHTCapRxSTBC1            VHTCap = "[RX-STBC-1]"
	VHTCapRxSTBC12           VHTCap = "[RX-STBC-12]"
	VHTCapRxSTBC123          VHTCap = "[RX-STBC-123]"
	VHTCapRxSTBC1234         VHTCap = "[RX-STBC-1234]"
	VHTCapSUBeamformer       VHTCap = "[SU-BEAMFORMER]"
	VHTCapSUBeamformee       VHTCap = "[SU-BEAMFORMEE]"
	VHTCapBFAntenna2         VHTCap = "[BF-ANTENNA-2]"
	VHTCapSoundingDimension2 VHTCap = "[SOUNDING-DIMENSION-2]"
	VHTCapMUBeamformer       VHTCap = "[MU-BEAMFORMER]"
	VHTCapMUBeamformee       VHTCap = "[MU-BEAMFORMEE]"
	VHTCapVHTTXOPPS          VHTCap = "[VHT-TXOP-PS]"
	VHTCapHTCVHT             VHTCap = "[HTC-VHT]"
	VHTCapMaxAMPDULenExp0    VHTCap = "[MAX-A-MPDU-LEN-EXP0]"
	VHTCapMaxAMPDULenExp1    VHTCap = "[MAX-A-MPDU-LEN-EXP1]"
	VHTCapMaxAMPDULenExp2    VHTCap = "[MAX-A-MPDU-LEN-EXP2]"
	VHTCapMaxAMPDULenExp3    VHTCap = "[MAX-A-MPDU-LEN-EXP3]"
	VHTCapMaxAMPDULenExp4    VHTCap = "[MAX-A-MPDU-LEN-EXP4]"
	VHTCapMaxAMPDULenExp5    VHTCap = "[MAX-A-MPDU-LEN-EXP5]"
	VHTCapMaxAMPDULenExp6    VHTCap = "[MAX-A-MPDU-LEN-EXP6]"
	VHTCapMaxAMPDULenExp7    VHTCap = "[MAX-A-MPDU-LEN-EXP7]"
	VHTCapVHTLinkADAPT2      VHTCap = "[VHT-LINK-ADAPT2]"
	VHTCapVHTLinkADAPT3      VHTCap = "[VHT-LINK-ADAPT3]"
	VHTCapRxAntennaPattern   VHTCap = "[RX-ANTENNA-PATTERN]"
	VHTCapTxAntennaPattern   VHTCap = "[TX-ANTENNA-PATTERN]"
)

// ChWidthEnum is the type for specifying operating channel width in hostapd config.
type ChWidthEnum int

// ChWidth enums.
const (
	ChWidth20 ChWidthEnum = iota
	ChWidth40
	ChWidth40Minus
	ChWidth40Plus
	ChWidth80
	ChWidth160
	ChWidth80Plus80
	ChWidth160Plus80
	ChWidth160Plus160
	ChWidthUnknown
)

// String returns ChWidthEnum as a string.
func (cw ChWidthEnum) String() string {
	var typeStr string
	switch cw {
	case ChWidth20:
		typeStr = "20"
	case ChWidth40:
		typeStr = "40"
	case ChWidth40Minus:
		typeStr = "40m"
	case ChWidth40Plus:
		typeStr = "40p"
	case ChWidth80:
		typeStr = "80"
	case ChWidth160:
		typeStr = "160"
	case ChWidth80Plus80:
		typeStr = "80+80"
	case ChWidth160Plus80:
		typeStr = "160+80"
	case ChWidth160Plus160:
		typeStr = "160+160"
	case ChWidthUnknown:
		typeStr = ""
	default:
		typeStr = string(rune(cw))
	}
	return typeStr
}

// BandEnum is the type for the WiFi band.
type BandEnum int

// Band enums.
const (
	Band2Ghz BandEnum = iota
	Band5Ghz
	Band6Ghz
	BandUnknown
)

// String returns BandEnum as a string.
func (b BandEnum) String() string {
	var typeStr string
	switch b {
	case Band2Ghz:
		typeStr = "2.4Ghz"
	case Band5Ghz:
		typeStr = "5Ghz"
	case Band6Ghz:
		typeStr = "6Ghz"
	case BandUnknown:
		typeStr = ""
	default:
		typeStr = string(rune(b))
	}
	return typeStr
}

// OpModeEnum is the type for specifying operating mode in hostapd config.
type OpModeEnum int

// Mode enums.
const (
	ModeVHT OpModeEnum = iota
	ModeHT
	ModeHE
	ModeEHT
)

// String returns OpMode as a string.
func (om OpModeEnum) String() string {
	var typeStr string
	switch om {
	case ModeVHT:
		typeStr = "VHT"
	case ModeHT:
		typeStr = "HT"
	case ModeHE:
		typeStr = "HE"
	case ModeEHT:
		typeStr = "EHT"
	default:
		typeStr = string(rune(om))
	}
	return typeStr
}

// VHTChWidthEnum is the type for specifying operating channel width in hostapd config (vht_oper_chwidth=).
type VHTChWidthEnum int

// VHTChWidth enums.
const (
	// VHTChWidth20Or40 is the default value when none of VHTChWidth* specified.
	VHTChWidth20Or40 VHTChWidthEnum = iota
	VHTChWidth80
	VHTChWidth160
	VHTChWidth80Plus80
)

// HEChWidthEnum is the type for specifying operating channel width in hostapd config (he_oper_chwidth=).
type HEChWidthEnum int

// HEChWidth enums.
const (
	// HEChWidth20Or40 is the default value when none of HEChWidth* specified.
	HEChWidth20Or40 HEChWidthEnum = iota
	HEChWidth80
	HEChWidth160
	HEChWidth80Plus80
)

// HECap is the type for specifying HE capabilities in hostapd config (he_capab=).
// This capability is currently not supported in hostapd.
type HECap string

// EHTChWidthEnum is the type for specifying operating channel width in hostapd config (eht_oper_chwidth=).
type EHTChWidthEnum int

// EHTChWidth enums.
const (
	// EHTChWidth20Or40 is the default value when none of EHTChWidth* specified.
	EHTChWidth20Or40 EHTChWidthEnum = iota
	EHTChWidth80
	EHTChWidth160
	EHTChWidth80Plus80
	EHTChWidth160Plus80
	EHTChWidth160Plus160
)

// EHTCap is the type for specifying EHT capabilities in hostapd config (eht_capab=).
// This capability is currently not supported in hostapd.
type EHTCap string

// PMFEnum is the type for specifying the setting of "Protected Management Frames" (IEEE802.11w).
type PMFEnum int

// PMF enums.
const (
	PMFDisabled PMFEnum = iota
	PMFOptional
	PMFRequired
)

// AdditionalBSS is the type for specifying parameters of additional BSSs to be advertised on the same phy.
// All fields are required, and must be distinct from the corresponding fields of the primary network.
type AdditionalBSS struct {
	IfaceName string
	SSID      string
	BSSID     string
}

// VenueInfo is the type for specifying the kind of venue the network is deployed.
type VenueInfo struct {
	Group byte
	Type  byte
}

// VenueName is the type for specifying the name of the venue for a given language.
type VenueName struct {
	Lang string
	Name string
}

// VenueInfo values as described by IEEE 802.11u std 7.3.1.
var (
	VenueInfoUnspecified VenueInfo = VenueInfo{0, 0}

	VenueInfoUnspecifiedAssembly         VenueInfo = VenueInfo{1, 0}
	VenueInfoArena                       VenueInfo = VenueInfo{1, 1}
	VenueInfoStadium                     VenueInfo = VenueInfo{1, 2}
	VenueInfoPassengerTerminal           VenueInfo = VenueInfo{1, 3}
	VenueInfoAmphitheater                VenueInfo = VenueInfo{1, 4}
	VenueInfoAmusementPark               VenueInfo = VenueInfo{1, 5}
	VenueInfoPlaceOfWorship              VenueInfo = VenueInfo{1, 6}
	VenueInfoConventionCenter            VenueInfo = VenueInfo{1, 7}
	VenueInfoLibrary                     VenueInfo = VenueInfo{1, 8}
	VenueInfoMuseum                      VenueInfo = VenueInfo{1, 9}
	VenueInfoRestaurant                  VenueInfo = VenueInfo{1, 10}
	VenueInfoTheater                     VenueInfo = VenueInfo{1, 11}
	VenueInfoBar                         VenueInfo = VenueInfo{1, 12}
	VenueInfoCoffeeShop                  VenueInfo = VenueInfo{1, 13}
	VenueInfoZooOrAquarium               VenueInfo = VenueInfo{1, 14}
	VenueInfoEmergencyCoordinationCenter VenueInfo = VenueInfo{1, 15}

	VenueInfoUnspecifiedBusiness            VenueInfo = VenueInfo{2, 1}
	VenueInfoDoctorOrDentist                VenueInfo = VenueInfo{2, 2}
	VenueInfoBank                           VenueInfo = VenueInfo{2, 3}
	VenueInfoFireStation                    VenueInfo = VenueInfo{2, 4}
	VenueInfoPoliceStation                  VenueInfo = VenueInfo{2, 5}
	VenueInfoPostOffice                     VenueInfo = VenueInfo{2, 6}
	VenueInfoProfessionalOffice             VenueInfo = VenueInfo{2, 7}
	VenueInfoResearchAndDevelopmentFacility VenueInfo = VenueInfo{2, 8}
	VenueInfoAttorneyOffice                 VenueInfo = VenueInfo{2, 9}

	VenueInfoUnspecifiedEducational VenueInfo = VenueInfo{3, 0}
	VenueInfoPrimarySchool          VenueInfo = VenueInfo{3, 1}
	VenueInfoSecondarySchool        VenueInfo = VenueInfo{3, 2}
	VenueInfoUniversityOrCollege    VenueInfo = VenueInfo{3, 3}

	VenueInfoUnspecifiedFactoryAndIndustrial VenueInfo = VenueInfo{4, 0}
	VenueInfoFactory                         VenueInfo = VenueInfo{4, 1}

	VenueInfoUnspecifiedInstitutional VenueInfo = VenueInfo{5, 0}
	VenueInfoHospital                 VenueInfo = VenueInfo{5, 1}
	VenueInfoLongTermCareFacility     VenueInfo = VenueInfo{5, 2}
	VenueInfoRehabilitationCenter     VenueInfo = VenueInfo{5, 3}
	VenueInfoGroupHome                VenueInfo = VenueInfo{5, 4}
	VenueInfoPrisonOrJail             VenueInfo = VenueInfo{5, 5}

	VenueInfoUnspecifiedMercantile    VenueInfo = VenueInfo{6, 0}
	VenueInfoRetailStore              VenueInfo = VenueInfo{6, 1}
	VenueInfoGroceryMarket            VenueInfo = VenueInfo{6, 2}
	VenueInfoAutomotiveServiceStation VenueInfo = VenueInfo{6, 3}
	VenueInfoShoppingMall             VenueInfo = VenueInfo{6, 4}
	VenueInfoGasStation               VenueInfo = VenueInfo{6, 5}

	VenueInfoUnspecifiedResidential VenueInfo = VenueInfo{7, 0}
	VenueInfoHotelOrMotel           VenueInfo = VenueInfo{7, 1}
	VenueInfoDormitory              VenueInfo = VenueInfo{7, 2}
	VenueInfoBoardingHouse          VenueInfo = VenueInfo{7, 3}

	VenueInfoUnspecifiedVehicular VenueInfo = VenueInfo{10, 0}
	VenueInfoAutomobileOrTruck    VenueInfo = VenueInfo{10, 1}
	VenueInfoAirplane             VenueInfo = VenueInfo{10, 2}
	VenueInfoBus                  VenueInfo = VenueInfo{10, 3}
	VenueInfoFerry                VenueInfo = VenueInfo{10, 4}
	VenueInfoShipOrBoat           VenueInfo = VenueInfo{10, 5}
	VenueInfoTrain                VenueInfo = VenueInfo{10, 6}
	VenueInfoMotorBike            VenueInfo = VenueInfo{10, 7}

	VenueInfoUnspecifiedOutdoor VenueInfo = VenueInfo{11, 0}
	VenueInfoMultimeshNetwork   VenueInfo = VenueInfo{11, 1}
	VenueInfoCityPark           VenueInfo = VenueInfo{11, 2}
	VenueInfoRestArea           VenueInfo = VenueInfo{11, 3}
	VenueInfoTrafficControl     VenueInfo = VenueInfo{11, 4}
)

// RealmEncoding is the type for specifying the encoding of the realm domain name.
type RealmEncoding byte

// Realm domain name encoding values as defined in IEEE 802.11u - 7.3.4.9.
const (
	RealmEncodingRFC4282 RealmEncoding = 0
	RealmEncodingUTF8    RealmEncoding = 1
)

// NAIRealm is the type for specifying the
type NAIRealm struct {
	Domains  []string
	Encoding RealmEncoding
	Methods  []EAPMethod
}

// EAPMethodType is the type for specifying the EAP authentication method of a realm.
type EAPMethodType byte

// EAP Method types as defined by IANA (see http://www.iana.org/assignments/eap-numbers/eap-numbers.xhtml#eap-numbers-4)
const (
	EAPMethodTypeTLS  EAPMethodType = 13
	EAPMethodTypeTTLS EAPMethodType = 21
)

// EAPMethod contains a set of authentication parameters associated with a EAP authentication method.
type EAPMethod struct {
	Type   EAPMethodType
	Params []EAPAuthParam
}

// EAPAuthParamType is the type for specifying additional EAP authentication parameters.
type EAPAuthParamType byte

// EAP authentication parameter types as defined in IEEE 802.11u - Table 7-43bo.
const (
	AuthParamInnerNonEAP EAPAuthParamType = 2
	AuthParamInnerEAP    EAPAuthParamType = 3
	AuthParamCredential  EAPAuthParamType = 5
	AuthParamTunneledEAP EAPAuthParamType = 6
)

// EAP authentication values for AuthParamInnerNonEAP (see IEEE 802.11u - table 7-43bo).
const (
	AuthNonEAPAuthPAP      byte = 1
	AuthNonEAPAuthCHAP     byte = 2
	AuthNonEAPAuthMSCHAP   byte = 3
	AuthNonEAPAuthMSCHAPV2 byte = 4
)

// EAP authentication values for AuthParamCredential (see IEEE 802.11u - table 7-43bo).
const (
	AuthCredentialsSIM              byte = 1
	AuthCredentialsUSIM             byte = 2
	AuthCredentialsNFCSecureElement byte = 3
	AuthCredentialsHardwareToken    byte = 4
	AuthCredentialsSoftToken        byte = 5
	AuthCredentialsCertificate      byte = 6
	AuthCredentialsUsernamePassword byte = 7
	AuthCredentialsAnonymous        byte = 9
	AuthCredentialsVendorSpecific   byte = 10
)

// EAPAuthParam describes an EAP authentication parameter using a type and a value.
type EAPAuthParam struct {
	Type  EAPAuthParamType
	Value byte
}

// String provides a readable representation of the realm in the format of hostapd configuration.
func (r NAIRealm) String() string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%d,%s", r.Encoding, strings.Join(r.Domains, ";")))

	// Append the optional authentication methods.
	for _, m := range r.Methods {
		builder.WriteString(fmt.Sprintf(",%d", m.Type))
		for _, p := range m.Params {
			builder.WriteString(fmt.Sprintf("[%d:%d]", p.Type, p.Value))
		}
	}

	return builder.String()
}

// Option is the function signature used to specify options of Config.
type Option func(*Config)

// SSID returns an Option which sets ssid in hostapd config.
func SSID(ssid string) Option {
	return func(c *Config) {
		c.SSID = ssid
	}
}

// Mode returns an Option which sets mode in hostapd config.
func Mode(mode ModeEnum) Option {
	return func(c *Config) {
		c.Mode = mode
	}
}

// Channel returns an Option which sets channel in hostapd config.
func Channel(ch int) Option {
	return func(c *Config) {
		c.Channel = ch
	}
}

// OpClass returns an Option which sets the operating class in hostapd config.
// OpClass and Channel together uniquely identify channels across different
// bands including the 6GHz band. For backwards compatibility, if the OpClass is
// not specified, the channel is assumed to be in the 2.4GHz or 5GHz band.
// If the operating class is in the range [131, 137], the channel will be mapped
// to a frequency in the 6GHz band.
// Refer to Table E-4 in IEEE Std 802.11ax-2021 for valid
// channel-operating class pairs in the 6GHz band.
func OpClass(opClass int) Option {
	return func(c *Config) {
		c.OpClass = opClass
	}
}

// HTCaps returns an Option which sets HT capabilities in hostapd config.
func HTCaps(caps ...HTCap) Option {
	return func(c *Config) {
		for _, ca := range caps {
			c.HTCaps |= ca
		}
	}
}

// VHTCaps returns an Option which sets VHT capabilities in hostapd config.
func VHTCaps(caps ...VHTCap) Option {
	return func(c *Config) {
		c.VHTCaps = append(c.VHTCaps, caps...)
	}
}

// VHTCenterChannel returns an Option which sets VHT center channel in hostapd config.
func VHTCenterChannel(ch int) Option {
	return func(c *Config) {
		c.VHTCenterChannel = ch
	}
}

// VHTChWidth returns an Option which sets VHT operating channel width in hostapd config.
func VHTChWidth(chw VHTChWidthEnum) Option {
	return func(c *Config) {
		c.VHTChWidth = chw
	}
}

// HECenterChannel returns an Option which sets HE center channel in hostapd config.
func HECenterChannel(ch int) Option {
	return func(c *Config) {
		c.HECenterChannel = ch
	}
}

// HEChWidth returns an Option which sets HE operating channel width in hostapd config.
func HEChWidth(chw HEChWidthEnum) Option {
	return func(c *Config) {
		c.HEChWidth = chw
	}
}

// HECaps returns an Option which sets HE capabilities in hostapd config.
func HECaps(caps ...HECap) Option {
	return func(c *Config) {
		c.HECaps = append(c.HECaps, caps...)
	}
}

// EHTCenterChannel returns an Option which sets EHT center channel in hostapd config.
func EHTCenterChannel(ch int) Option {
	return func(c *Config) {
		c.EHTCenterChannel = ch
	}
}

// EHTChWidth returns an Option which sets EHT operating channel width in hostapd config.
func EHTChWidth(chw EHTChWidthEnum) Option {
	return func(c *Config) {
		c.EHTChWidth = chw
	}
}

// EHTCaps returns an Option which sets EHT capabilities in hostapd config.
func EHTCaps(caps ...EHTCap) Option {
	return func(c *Config) {
		c.EHTCaps = append(c.EHTCaps, caps...)
	}
}

// Hidden returns an Option which sets that it is a hidden network in hostapd config.
func Hidden() Option {
	return func(c *Config) {
		c.Hidden = true
	}
}

// SecurityConfig returns an Option which sets the security config in hostapd config.
func SecurityConfig(conf security.Config) Option {
	return func(c *Config) {
		c.SecurityConfig = conf
	}
}

// PMF returns an Options which sets whether protected management frame
// is enabled or required.
func PMF(p PMFEnum) Option {
	return func(c *Config) {
		c.PMF = p
	}
}

// SpectrumManagement returns an Option which enables spectrum management in hostapd config.
func SpectrumManagement() Option {
	return func(c *Config) {
		c.SpectrumManagement = true
	}
}

// DTIMPeriod returns an Option which sets the DTIM period in hostapd config.
func DTIMPeriod(period int) Option {
	return func(c *Config) {
		c.DTIMPeriod = period
	}
}

// BeaconInterval returns an Option which sets the beacon interval in hostapd config.
// The unit is 1kus = 1.024ms. The value should be in 15..65535.
func BeaconInterval(bi int) Option {
	return func(c *Config) {
		c.BeaconInterval = bi
	}
}

// BSSID returns an Option which sets bssid in hostapd config.
func BSSID(bssid string) Option {
	return func(c *Config) {
		c.BSSID = bssid
	}
}

// OBSSInterval returns an Option which sets the interval in seconds between
// overlapping BSS scans. Default value is 0 (disabled).
func OBSSInterval(interval uint16) Option {
	return func(c *Config) {
		c.OBSSInterval = interval
	}
}

// Bridge returns an Option which sets bridge in hostapd config.
func Bridge(br string) Option {
	return func(c *Config) {
		c.Bridge = br
	}
}

// MobilityDomain returns an Option which sets mobility domain in hostapd config.
func MobilityDomain(mdID string) Option {
	return func(c *Config) {
		c.MobilityDomain = mdID
	}
}

// NASIdentifier returns an Option which sets nas_identifier in hostapd config.
func NASIdentifier(id string) Option {
	return func(c *Config) {
		c.NASIdentifier = id
	}
}

// R1KeyHolder returns an Option which sets r1 key holder identifier in hostapd config.
func R1KeyHolder(r1khID string) Option {
	return func(c *Config) {
		c.R1KeyHolder = r1khID
	}
}

// R0KHs returns an Option which sets R0KHs in hostapd config. Each R0KH
// should be in format: <MAC address> <NAS Identifier> <256-bit key as hex string>
func R0KHs(r0KHs ...string) Option {
	return func(c *Config) {
		c.R0KHs = append([]string(nil), r0KHs...)
	}
}

// R1KHs returns an Option which sets R1KHs in hostapd config. Each R1KH
// should be in format: <MAC address> <R1KH-ID> <256-bit key as hex string>
func R1KHs(r1KHs ...string) Option {
	return func(c *Config) {
		c.R1KHs = append([]string(nil), r1KHs...)
	}
}

// MBO returns an Option which enables MBO in hostapd config.
func MBO() Option {
	return func(c *Config) {
		c.MBO = true
	}
}

// RRMBeaconReport returns an Option which enables RRM Beacon Report in hostapd config.
func RRMBeaconReport() Option {
	return func(c *Config) {
		c.RRMBeaconReport = true
	}
}

// APSD returns an Option which enables U-APSD advertisement in hostapd config.
func APSD() Option {
	return func(c *Config) {
		c.APSD = true
	}
}

// AdditionalBSSs returns an Option which sets AdditionalBSSs in hostapd config.
// Each AdditionalBSS should have a unique interface name, SSID, and BSSID. The
// number of AdditionalBSSs is limited by the phy. See the 'valid interface
// combinations' section of `iw phy` for more.
func AdditionalBSSs(bssids ...AdditionalBSS) Option {
	return func(c *Config) {
		c.AdditionalBSSs = append([]AdditionalBSS(nil), bssids...)
	}
}

// SupportedRates returns an Option which sets the supported rates in hostapd config.
func SupportedRates(r ...float32) Option {
	return func(c *Config) {
		c.SupportedRates = append([]float32(nil), r...)
	}
}

// BasicRates returns an Option which sets the basic rates in hostapd config.
func BasicRates(r ...float32) Option {
	return func(c *Config) {
		c.BasicRates = append([]float32(nil), r...)
	}
}

// Interworking returns an Option which enables interworking service in hostapd config.
func Interworking() Option {
	return func(c *Config) {
		c.Interworking = true
	}
}

// VenueInfos returns an Option which sets the venue group and type in hostapd config.
func VenueInfos(vi VenueInfo) Option {
	return func(c *Config) {
		c.VenueInfo = vi
	}
}

// VenueNames returns an Option which sets the venue names in hostapd config.
func VenueNames(vns ...VenueName) Option {
	return func(c *Config) {
		c.VenueNames = append([]VenueName(nil), vns...)
	}
}

// RoamingConsortiums returns an Option which sets the roaming consortium OI(s) in hostapd configuration.
func RoamingConsortiums(rc ...string) Option {
	return func(c *Config) {
		c.RoamingConsortiums = append([]string(nil), rc...)
	}
}

// DomainNames returns an Option which sets the interworking domain names in hostapd configuration.
func DomainNames(d ...string) Option {
	return func(c *Config) {
		c.DomainNames = append([]string(nil), d...)
	}
}

// Realms returns an Option which sets the Network Access Identifier Realms in hostapd configuration.
func Realms(r ...NAIRealm) Option {
	return func(c *Config) {
		c.Realms = append([]NAIRealm(nil), r...)
	}
}

// NewConfig creates a Config with given options.
// Default value of Ssid is a random generated string with prefix "TAST_TEST_" and total length 30.
func NewConfig(ops ...Option) (*Config, error) {
	// Default config.
	conf := &Config{
		SSID:           RandomSSID("TAST_TEST_"),
		SecurityConfig: &base.Config{},
	}
	for _, op := range ops {
		op(conf)
	}

	// Assign a random BSSID if not assigned to avoid a known issue of
	// reused BSSID in ath10k driver. For the BSSID re-used cases,
	// we'll have a specific test for that.
	if conf.BSSID == "" {
		randBSSID, err := RandomMAC()
		if err != nil {
			return nil, err
		}
		conf.BSSID = randBSSID.String()
	}

	if err := conf.validate(); err != nil {
		return nil, err
	}

	return conf, nil
}

// ApConfig is the configuration of hostapd options and security options.
type ApConfig struct {
	ApOpts []Option
	// If unassigned, use default security config: open network.
	SecConfFac security.ConfigFactory
}

// Config is the configuration to start hostapd on a router.
type Config struct {
	SSID               string
	Mode               ModeEnum
	Channel            int
	OpClass            int
	HTCaps             HTCap
	VHTCaps            []VHTCap
	VHTCenterChannel   int
	VHTChWidth         VHTChWidthEnum
	HEChWidth          HEChWidthEnum
	HECaps             []HECap
	HECenterChannel    int
	EHTChWidth         EHTChWidthEnum
	EHTCaps            []EHTCap
	EHTCenterChannel   int
	Hidden             bool
	SpectrumManagement bool
	BeaconInterval     int
	SecurityConfig     security.Config
	PMF                PMFEnum
	DTIMPeriod         int
	BSSID              string
	OBSSInterval       uint16
	Bridge             string
	MobilityDomain     string
	NASIdentifier      string
	R1KeyHolder        string
	R0KHs              []string
	R1KHs              []string
	MBO                bool
	RRMBeaconReport    bool
	APSD               bool
	AdditionalBSSs     []AdditionalBSS
	SupportedRates     []float32
	BasicRates         []float32
	Interworking       bool
	VenueInfo
	VenueNames         []VenueName
	RoamingConsortiums []string
	DomainNames        []string
	Realms             []NAIRealm
}

// Format composes a hostapd.conf based on the given Config, iface and ctrlPath.
// iface is the network interface for the hostapd to run. ctrlPath is the control
// file path for hostapd to communicate with hostapd_cli.
func (c *Config) Format(iface, ctrlPath string) (string, error) {
	var builder strings.Builder
	configure := func(k, v string) {
		fmt.Fprintf(&builder, "%s=%s\n", k, v)
	}

	configure("logger_syslog", "-1")
	configure("logger_syslog_level", "0")
	// Default RTS and frag threshold to "off".
	configure("rts_threshold", "-1")
	configure("fragm_threshold", "2346")
	configure("driver", "nl80211")

	// Configurable.
	configure("ctrl_interface", ctrlPath)
	// ssid2 for printf-escaped string, cf. https://w1.fi/cgit/hostap/plain/hostapd/hostapd.conf
	configure("ssid2", hostap.EncodeSSID(c.SSID))
	configure("interface", iface)
	configure("channel", strconv.Itoa(c.Channel))

	hwMode, err := c.hwMode()
	if err != nil {
		return "", err
	}
	configure("hw_mode", hwMode)

	if c.is80211n() || c.is80211ac() || c.is80211ax() || c.is80211be() {
		configure("ieee80211n", "1")
		configure("ht_capab", c.htCapsString())
		if c.Mode == Mode80211nPure {
			configure("require_ht", "1")
		}
	}
	if c.is80211ac() || c.is80211ax() || c.is80211be() {
		configure("ieee80211ac", "1")
		configure("vht_oper_chwidth", strconv.Itoa(int(c.VHTChWidth)))
		// If not set, ignore this field and use hostapd's default value.
		if c.VHTCenterChannel != 0 {
			configure("vht_oper_centr_freq_seg0_idx", strconv.Itoa(c.VHTCenterChannel))
		}
		configure("vht_capab", c.vhtCapsString())
		if c.Mode == Mode80211acPure {
			configure("require_vht", "1")
		}
	}
	if c.is80211ax() || c.is80211be() {
		configure("ieee80211ax", "1")
		configure("he_oper_chwidth", strconv.Itoa(int(c.HEChWidth)))
		configure("he_default_pe_duration", "0")    // 0us value in PE (packet extension) field
		configure("he_basic_mcs_nss_set", "0xaaaa") // Enable MCS index 0-11 on spatial streams 1-8
		configure("he_bss_color", "42")             // Set default bss color

		if c.HECenterChannel != 0 {
			configure("he_oper_centr_freq_seg0_idx", strconv.Itoa(c.HECenterChannel))
		}
		if c.Mode == Mode80211axPure {
			configure("require_he", "1")
		}
	}
	if c.is80211be() {
		configure("ieee80211be", "1")
		configure("eht_oper_chwidth", strconv.Itoa(int(c.EHTChWidth)))

		if c.EHTCenterChannel != 0 {
			configure("eht_oper_centr_freq_seg0_idx", strconv.Itoa(c.EHTCenterChannel))
		}
		if c.Mode == Mode80211bePure {
			configure("require_eht", "1")
		}
	}
	if c.HTCaps != 0 {
		configure("wmm_enabled", "1")
	}
	if c.Hidden {
		configure("ignore_broadcast_ssid", "1")
	}
	if c.SpectrumManagement {
		configure("country_code", "US")          // Required for ieee80211d
		configure("ieee80211d", "1")             // Required for local_pwr_constraint
		configure("local_pwr_constraint", "0")   // No local constraint
		configure("spectrum_mgmt_required", "1") // Requires local_pwr_constraint
		configure("ieee80211h", "1")             // Enables DFS
	}
	if c.BeaconInterval != 0 {
		configure("beacon_int", strconv.Itoa(c.BeaconInterval))
	}

	if c.DTIMPeriod != 0 {
		configure("dtim_period", strconv.Itoa(c.DTIMPeriod))
	}

	if c.BSSID != "" {
		configure("bssid", c.BSSID)
	}

	if c.OBSSInterval != 0 {
		configure("obss_interval", strconv.FormatUint(uint64(c.OBSSInterval), 10))
	}

	securityConf, err := c.SecurityConfig.HostapdConfig()
	if err != nil {
		return "", err
	}
	for k, v := range securityConf {
		configure(k, v)
	}

	if c.OpClass != 0 {
		if Is6GHzOpClass(c.OpClass) {
			// Only configure operating class in hostapd if it is needed to
			// disambiguate a 6GHz channel from a 2.4/5GHz channel.
			configure("op_class", strconv.Itoa(c.OpClass))
			// Set country code to US to enable the DUT to actively scan for the
			// AP on 6GHz.
			configure("country_code", "US")
			// Enable hash-to-element mechanism for 6GHz networks.
			configure("sae_pwe", "1")
		} else {
			return "", errors.New("operating class outside of [131, 137] is not handled in testing hostapd config")
		}
	}

	// If HostapdConfig has provided "ieee80211w" then do not overwrite it.
	if _, ok := securityConf["ieee80211w"]; !ok {
		configure("ieee80211w", strconv.Itoa(int(c.PMF)))
	}

	if c.Bridge != "" {
		configure("bridge", c.Bridge)
	}

	if c.MobilityDomain != "" {
		configure("mobility_domain", c.MobilityDomain)
	}
	if c.NASIdentifier != "" {
		configure("nas_identifier", c.NASIdentifier)
	}
	if c.R1KeyHolder != "" {
		configure("r1_key_holder", c.R1KeyHolder)
	}
	if len(c.R0KHs) != 0 {
		for _, r := range c.R0KHs {
			configure("r0kh", r)
		}
	}
	if len(c.R1KHs) != 0 {
		for _, r := range c.R1KHs {
			configure("r1kh", r)
		}
	}

	if c.MBO {
		configure("mbo", "1")
	}

	if c.RRMBeaconReport {
		configure("rrm_beacon_report", "1")
	}

	if c.APSD {
		configure("uapsd_advertisement_enabled", "1")
	}

	for _, bssid := range c.AdditionalBSSs {
		if bssid.IfaceName == iface {
			return "", errors.Errorf("found a duplicate interface name: %s", iface)
		}
		configure("bss", bssid.IfaceName)
		configure("ssid", bssid.SSID)
		configure("bssid", bssid.BSSID)
	}

	if len(c.SupportedRates) != 0 {
		// Convert from Mbps to 100Kbps.
		rates := make([]string, len(c.SupportedRates))
		for i, r := range c.SupportedRates {
			rates[i] = fmt.Sprintf("%d", int(r*10))
		}
		configure("supported_rates", strings.Join(rates, " "))
	}

	if len(c.BasicRates) != 0 {
		// Convert from Mbps to 100Kbps.
		rates := make([]string, len(c.BasicRates))
		for i, r := range c.BasicRates {
			rates[i] = fmt.Sprintf("%d", int(r*10))
		}
		configure("basic_rates", strings.Join(rates, " "))
	}

	if c.Interworking {
		configure("interworking", "1")
		configure("venue_group", strconv.Itoa(int(c.VenueInfo.Group)))
		configure("venue_type", strconv.Itoa(int(c.VenueInfo.Type)))
		for _, vn := range c.VenueNames {
			configure("venue_name", fmt.Sprintf("%s:%s", vn.Lang, vn.Name))
		}
		for _, rc := range c.RoamingConsortiums {
			configure("roaming_consortium", rc)
		}
		if len(c.DomainNames) > 0 {
			configure("domain_name", strings.Join(c.DomainNames, ","))
		}
		for _, r := range c.Realms {
			configure("nai_realm", r.String())
		}
	}

	return builder.String(), nil
}

// PcapFreqOptions returns the options for the caller to set frequency with iw for
// preparing interface for packet capturing.
func (c *Config) PcapFreqOptions() ([]iw.SetFreqOption, error) {
	if c.is80211be() {
		switch c.EHTChWidth {
		case EHTChWidth80:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidth80)}, nil
		case EHTChWidth160:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidth160)}, nil
		case EHTChWidth80Plus80:
			return nil, errors.New("unsupported 80+80 channel width")
		case EHTChWidth160Plus80:
			return nil, errors.New("unsupported 160+80 channel width")
		case EHTChWidth160Plus160:
			return nil, errors.New("unsupported 160+160 channel width")
		}
		// fallthrough EHTChWidth20Or40.
	}
	if c.is80211ax() {
		switch c.HEChWidth {
		case HEChWidth80:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidth80)}, nil
		case HEChWidth160:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidth160)}, nil
		case HEChWidth80Plus80:
			return nil, errors.New("unsupported 80+80 channel width")
		}
		// fallthrough HEChWidth20Or40.
	}
	if c.is80211ac() {
		switch c.VHTChWidth {
		case VHTChWidth80:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidth80)}, nil
		case VHTChWidth160:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidth160)}, nil
		case VHTChWidth80Plus80:
			return nil, errors.New("unsupported 80+80 channel width")
		}
		// fallthrough VHTChWidth20Or40.
	}
	if c.is80211n() || c.is80211ac() || c.is80211ax() || c.is80211be() {
		// 80211n or 80211ac or 80211ax or 80211be with VHTChWidth20Or40 or
		// HEChWidth20Or40 or EHTChWidth20Or40.
		ht := c.htMode()
		switch ht {
		case HTCapHT40Minus:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidthHT40Minus)}, nil
		case HTCapHT40Plus:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidthHT40Plus)}, nil
		default:
			return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidthHT20)}, nil
		}
	}
	return []iw.SetFreqOption{iw.SetFreqChWidth(iw.ChWidthNOHT)}, nil
}

// PerfDesc returns the description of this config.
// Useful for reporting perf metrics.
func (c *Config) PerfDesc() string {
	width, mode := c.ChannelWidthAndMode()
	return fmt.Sprintf("ch%03d_mode%s%s_%s", c.Channel, mode, width.String(), c.SecurityConfig.Class())
}

// ChannelWidthAndMode returns the channel width and the operation mode.
func (c *Config) ChannelWidthAndMode() (ChWidthEnum, string) {
	var width ChWidthEnum
	var mode OpModeEnum
	if c.is80211ac() {
		mode = ModeVHT
		switch c.VHTChWidth {
		case VHTChWidth80:
			width = ChWidth80
		case VHTChWidth160:
			width = ChWidth160
		case VHTChWidth80Plus80:
			width = ChWidth80Plus80
		default:
			if c.HTCaps&HTCapHT20 > 0 {
				width = ChWidth20
			} else {
				width = ChWidth40
			}
		}
	} else if c.is80211n() {
		mode = ModeHT
		switch c.htMode() {
		case HTCapHT40Minus:
			width = ChWidth40Minus
		case HTCapHT40Plus:
			width = ChWidth40Plus
		default:
			width = ChWidth20
		}
	} else if c.is80211ax() {
		mode = ModeHE
		switch c.HEChWidth {
		case HEChWidth80:
			width = ChWidth80
		case HEChWidth160:
			width = ChWidth160
		case HEChWidth80Plus80:
			width = ChWidth80Plus80
		default:
			if c.HTCaps&HTCapHT20 > 0 {
				width = ChWidth20
			} else {
				width = ChWidth40
			}
		}
	} else if c.is80211be() {
		mode = ModeEHT
		switch c.EHTChWidth {
		case EHTChWidth80:
			width = ChWidth80
		case EHTChWidth160:
			width = ChWidth160
		case EHTChWidth80Plus80:
			width = ChWidth80Plus80
		case EHTChWidth160Plus80:
			width = ChWidth160Plus80
		case EHTChWidth160Plus160:
			width = ChWidth160Plus160
		default:
			if c.HTCaps&HTCapHT20 > 0 {
				width = ChWidth20
			} else {
				width = ChWidth40
			}
		}
	} else {
		return ChWidthUnknown, "11" + string(c.Mode)
	}
	return width, mode.String()
}

// OperatingBandAndFreq returns the operating band and frequency.
func (c *Config) OperatingBandAndFreq() (BandEnum, int, error) {
	var band BandEnum
	freq, err := ChannelToFrequencyWithOpClass(c.Channel, c.OpClass)
	if err != nil {
		return BandUnknown, 0, err
	}
	if freq >= 2412 && freq <= 2484 {
		band = Band2Ghz
	} else if freq >= 5160 && freq <= 5885 {
		band = Band5Ghz
	} else if freq >= 5935 && freq <= 7115 {
		band = Band6Ghz
	} else {
		return BandUnknown, freq, errors.New("unknown band")
	}
	return band, freq, nil
}

// validate validates the Config, c.
func (c *Config) validate() error {
	if c.SSID == "" || len(c.SSID) > 32 {
		return errors.New("invalid SSID")
	}
	if c.BSSID != "" && len(c.BSSID) != 17 {
		return errors.New("invalid BSSID")
	}
	if c.Mode == "" {
		return errors.New("invalid mode")
	}
	if c.HTCaps > 0 && !c.is80211n() && !c.is80211ac() && !c.is80211ax() && !c.is80211be() {
		return errors.Errorf("HTCap is not supported by mode %s", c.Mode)
	}
	if c.HTCaps == 0 && (c.is80211n() || c.is80211ac() || c.is80211ax() || c.is80211be()) {
		return errors.New("HTCap should be set in mode 802.11n or 802.11ac or 802.11ax or 802.11be")
	}
	if c.is80211be() {
		if err := c.validateEHTChWidth(); err != nil {
			return err
		}
	} else {
		// We do not check EHTCap here (in contrast with the VHTCap check below)
		// because EHTCaps are not currently supported by hostapd.
		if c.EHTCenterChannel != 0 {
			return errors.Errorf("EHTCenterChannel is not supported by mode %s", c.Mode)
		}
		if c.EHTChWidth != EHTChWidth20Or40 {
			return errors.Errorf("EHTChWidth is not supported by mode %s", c.Mode)
		}
	}
	if c.is80211ax() {
		if err := c.validateHEChWidth(); err != nil {
			return err
		}
	} else {
		// We do not check HECap here (in contrast with the VHTCap check below)
		// because HECaps are not currently supported by hostapd.
		if c.HECenterChannel != 0 {
			return errors.Errorf("HECenterChannel is not supported by mode %s", c.Mode)
		}
		if c.HEChWidth != HEChWidth20Or40 {
			return errors.Errorf("HEChWidth is not supported by mode %s", c.Mode)
		}
	}

	if c.is80211ac() || c.is80211ax() || c.is80211be() {
		if err := c.validateVHTChWidth(); err != nil {
			return err
		}
	} else {
		if len(c.VHTCaps) != 0 {
			return errors.Errorf("VHTCap is not supported by mode %s", c.Mode)
		}
		if c.VHTCenterChannel != 0 {
			return errors.Errorf("VHTCenterChannel is not supported by mode %s", c.Mode)
		}
		if c.VHTChWidth != VHTChWidth20Or40 {
			return errors.Errorf("VHTChWidth is not supported by mode %s", c.Mode)
		}
	}

	if err := c.validateChannel(); err != nil {
		return err
	}
	if c.BeaconInterval != 0 && (c.BeaconInterval > 65535 || c.BeaconInterval < 15) {
		return errors.Errorf("invalid beacon interval setting %d", c.BeaconInterval)
	}
	if c.SecurityConfig == nil {
		return errors.New("no SecurityConfig set")
	}
	if err := c.validatePMF(); err != nil {
		return err
	}

	if c.DTIMPeriod != 0 {
		if c.DTIMPeriod > 255 || c.DTIMPeriod < 1 {
			return errors.Errorf("invalid DTIM period: got %d; want [1..255]", c.DTIMPeriod)
		}
	}

	if c.MobilityDomain != "" {
		if b, err := hex.DecodeString(c.MobilityDomain); err != nil {
			return errors.Wrap(err, "mobility domain should be a hex string")
		} else if len(b) != 2 {
			return errors.New("mobility domain should be 2-octet identifier as a hex string")
		}
	}

	if c.R1KeyHolder != "" {
		if b, err := hex.DecodeString(c.R1KeyHolder); err != nil {
			return errors.Wrap(err, "r1 key holder identifier should be a hex string")
		} else if len(b) != 6 {
			return errors.New("r1 key holder identifier should be 6-octet identifier as a hex string")
		}
	}

	for i, khs := range append(c.R0KHs, c.R1KHs...) {
		fields := strings.Fields(khs)
		if len(fields) != 3 {
			return errors.Errorf("key holder should have exactly three fields, got %q", khs)
		}
		if _, err := net.ParseMAC(fields[0]); err != nil {
			return errors.Wrap(err, "the first field of key holders should be a MAC address")
		}
		if i >= len(c.R0KHs) {
			if _, err := net.ParseMAC(fields[1]); err != nil {
				return errors.Wrap(err, "the second field of r1kh should be a MAC address")
			}
		}
		if b, err := hex.DecodeString(fields[2]); err != nil {
			return errors.Wrap(err, "the third field of key holders should be a hex string")
		} else if len(b) != 32 {
			return errors.Errorf("the third field of key holders should be a 256-bits hex string, got len=%d", len(b))
		}
	}

	ifaces := map[string]struct{}{}
	ssids := map[string]struct{}{c.SSID: {}}
	bssids := map[string]struct{}{c.BSSID: {}}
	for _, bssid := range c.AdditionalBSSs {
		if _, ok := ifaces[bssid.IfaceName]; ok {
			return errors.Errorf("duplicate interface name %s in additional BSSIDs", bssid.IfaceName)
		}
		ifaces[bssid.IfaceName] = struct{}{}
		if _, ok := ssids[bssid.SSID]; ok {
			return errors.Errorf("duplicate SSID %s in additional BSSIDs", bssid.SSID)
		}
		ssids[bssid.SSID] = struct{}{}
		if _, ok := bssids[bssid.BSSID]; ok {
			return errors.Errorf("duplicate BSSID %s in additional BSSIDs", bssid.BSSID)
		}
		bssids[bssid.BSSID] = struct{}{}
	}

	return nil
}

// Helpers for Config to validate.

func channelIn(ch int, list []int) bool {
	for _, c := range list {
		if c == ch {
			return true
		}
	}
	return false
}

var ht40MinusChannels = []int{5, 6, 7, 8, 9, 10, 11, 12, 13, 40, 48, 56, 64, 104, 112, 120, 128, 136, 144, 153, 161}
var ht40PlusChannels = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 36, 44, 52, 60, 100, 108, 116, 124, 132, 140, 149, 157}

func supportHT40Plus(ch int) bool {
	return channelIn(ch, ht40PlusChannels)
}

func supportHT40Minus(ch int) bool {
	return channelIn(ch, ht40MinusChannels)
}

func (c *Config) validateChannel() error {
	f, err := ChannelToFrequencyWithOpClass(c.Channel, c.OpClass)
	if err != nil {
		return errors.Errorf("invalid channel: %d", err)
	}

	modeErr := errors.Errorf("mode %s does not support ch%d", c.Mode, c.Channel)
	switch c.Mode {
	case Mode80211a:
		if f < 5000 {
			return modeErr
		}
	case Mode80211b, Mode80211g:
		if f > 5000 {
			return modeErr
		}
	}

	htPlus := supportHT40Plus(c.Channel)
	htMinus := supportHT40Minus(c.Channel)
	if Is6GHzOpClass(c.OpClass) && (c.HTCaps&HTCapHT40 > 0 || c.HTCaps&HTCapHT40Plus > 0 || c.HTCaps&HTCapHT40Minus > 0) {
		return errors.New("6GHz channels do not support HTCap40+/-")
	}
	if c.HTCaps&HTCapHT40 > 0 && !htPlus && !htMinus {
		return errors.Errorf("ch%d does not support HTCap40", c.Channel)
	}
	if c.HTCaps&HTCapHT40Plus > 0 && !htPlus {
		return errors.Errorf("ch%d does not support HT40+", c.Channel)
	}
	if c.HTCaps&HTCapHT40Minus > 0 && !htMinus {
		return errors.Errorf("ch%d does not support HT40-", c.Channel)
	}
	return nil
}

// Helpers for Config to generate config map.

func (c *Config) is80211n() bool {
	return c.Mode == Mode80211nMixed || c.Mode == Mode80211nPure
}

func (c *Config) is80211ac() bool {
	return c.Mode == Mode80211acMixed || c.Mode == Mode80211acPure
}

func (c *Config) is80211ax() bool {
	return c.Mode == Mode80211axMixed || c.Mode == Mode80211axPure
}

func (c *Config) is80211be() bool {
	return c.Mode == Mode80211beMixed || c.Mode == Mode80211bePure
}

func (c *Config) hwMode() (string, error) {
	if c.Mode == Mode80211a || c.Mode == Mode80211b || c.Mode == Mode80211g {
		return string(c.Mode), nil
	}
	if c.is80211n() || c.is80211ac() || c.is80211ax() || c.is80211be() {
		f, err := ChannelToFrequencyWithOpClass(c.Channel, c.OpClass)
		if err != nil {
			return "", err
		}
		if f > 5000 {
			return string(Mode80211a), nil
		} else if c.is80211ax() {
			// 80211ax on 2.4ghz operates with 80211b hwmode as fallback.
			return string(Mode80211b), nil
		} else if c.is80211be() {
			// 80211be on 2.4ghz operates with 80211b hwmode as fallback.
			return string(Mode80211b), nil
		}
		return string(Mode80211g), nil
	}
	return "", errors.Errorf("invalid mode %s", string(c.Mode))
}

// htMode returns which of HT20, HT40+ and HT40- is used or 0 otherwise.
func (c *Config) htMode() HTCap {
	if c.HTCaps&(HTCapHT40|HTCapHT40Minus) > 0 && supportHT40Minus(c.Channel) {
		return HTCapHT40Minus
	}
	if c.HTCaps&(HTCapHT40|HTCapHT40Plus) > 0 && supportHT40Plus(c.Channel) {
		return HTCapHT40Plus
	}
	if c.HTCaps&HTCapHT20 > 0 {
		return HTCapHT20
	}
	return 0
}

func (c *Config) htCapsString() string {
	var caps []string
	htMode := c.htMode()
	switch htMode {
	case HTCapHT40Minus:
		caps = append(caps, "[HT40-]")
	case HTCapHT40Plus:
		caps = append(caps, "[HT40+]")
	default:
		// HT20 is default and no config string needed.
	}
	if c.HTCaps&HTCapSGI20 > 0 {
		caps = append(caps, "[SHORT-GI-20]")
	}
	if c.HTCaps&HTCapSGI40 > 0 {
		caps = append(caps, "[SHORT-GI-40]")
	}
	if c.HTCaps&HTCapLDPC > 0 {
		caps = append(caps, "[LDPC]")
	}
	return strings.Join(caps, "")
}

func (c *Config) vhtCapsString() string {
	caps := make([]string, len(c.VHTCaps))
	for i, v := range c.VHTCaps {
		caps[i] = string(v)
	}
	return strings.Join(caps, "")
}

func (c *Config) heCapsString() string {
	caps := make([]string, len(c.HECaps))
	for i, v := range c.HECaps {
		caps[i] = string(v)
	}
	return strings.Join(caps, "")
}

func (c *Config) ehtCapsString() string {
	caps := make([]string, len(c.EHTCaps))
	for i, v := range c.EHTCaps {
		caps[i] = string(v)
	}
	return strings.Join(caps, "")
}

func (c *Config) validateVHTChWidth() error {
	switch c.VHTChWidth {
	case VHTChWidth20Or40, VHTChWidth80, VHTChWidth160, VHTChWidth80Plus80:
		return nil
	default:
		return errors.Errorf("invalid vht_oper_chwidth %d", int(c.VHTChWidth))
	}
}

func (c *Config) validateHEChWidth() error {
	switch c.HEChWidth {
	case HEChWidth20Or40, HEChWidth80, HEChWidth80Plus80, HEChWidth160:
		return nil
	default:
		return errors.Errorf("invalid he_oper_chwidth %d", int(c.HEChWidth))
	}
}
func (c *Config) validateEHTChWidth() error {
	switch c.EHTChWidth {
	case EHTChWidth20Or40, EHTChWidth80, EHTChWidth80Plus80, EHTChWidth160, EHTChWidth160Plus80, EHTChWidth160Plus160:
		return nil
	default:
		return errors.Errorf("invalid eht_oper_chwidth %d", int(c.EHTChWidth))
	}
}
func (c *Config) validatePMF() error {
	switch c.PMF {
	case PMFDisabled:
		return nil
	case PMFOptional, PMFRequired:
		sec, err := c.SecurityConfig.Security()
		if err != nil {
			return errors.Wrap(err, "failed to get Security of config")
		}
		if sec == shillconst.SecurityNone || sec == shillconst.SecurityWEP {
			return errors.Errorf("security %s does not support PMF", sec)
		}
		if sec == shillconst.SecurityTransOWE {
			h, err := c.SecurityConfig.HostapdConfig()
			if err != nil {
				return errors.Wrap(err, "failed to get HostapdConfig")
			}
			if h["wpa_key_mgmt"] != "OWE" {
				return errors.New("open BSS of trans-owe AP does not support PMF")
			}
		}
		return nil
	default:
		return errors.Errorf("invalid PMFEnum %d", int(c.PMF))
	}
}

// RandomSSID returns a random SSID of length 30 and given prefix.
func RandomSSID(prefix string) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// Generate 30-char SSID, including prefix
	n := 30 - len(prefix)
	s := make([]byte, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return prefix + string(s)
}

const macBitLocal = 0x2
const macBitMulticast = 0x1

// RandomMAC returns a random MAC address for WiFi device.
// The MAC address is a locally administered unicast address.
// This can also be used as BSSID.
func RandomMAC() (net.HardwareAddr, error) {
	randMAC := make(net.HardwareAddr, 6)
	if _, err := rand.Read(randMAC); err != nil {
		return nil, errors.Wrap(err, "failed to generate random MAC address")
	}
	randMAC[0] = (randMAC[0] &^ macBitMulticast) | macBitLocal
	return randMAC, nil
}
