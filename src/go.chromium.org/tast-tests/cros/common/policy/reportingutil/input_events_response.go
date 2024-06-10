// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reportingutil

// InputEventsResponse represents the response from Reporting API. It
// consists of a list of InputEvent json objects.
type InputEventsResponse struct {
	Event []InputEvent `json:"event"`
}

// InputEvent is the model for the response from Reporting API. Add to this
// when you want to query for new fields.
type InputEvent struct {
	APIEvent *struct {
		ReportingRecordEvent *struct {
			Destination string `json:"destination"`
			Time        string `json:"timestampUs"`
		} `json:"reportingRecordEvent"`
	} `json:"apiEvent"`
	ObfuscatedCustomerID string                `json:"obfuscatedCustomerID"`
	ObfuscatedGaiaID     string                `json:"obfuscatedGaiaID"`
	ClientID             string                `json:"clientId"`
	WrappedEncryptedData *WrappedEncryptedData `json:"wrappedEncryptedData"`
}

// WrappedEncryptedData mirrors the wrappedEncryptedData JSON field.
type WrappedEncryptedData struct {
	MetricData         *MetricData         `json:"metricData"`
	DlpPolicyEvent     *DlpPolicyEvent     `json:"dlpPolicyEvent"`
	LockUnlockEvent    *LockUnlockEvent    `json:"lockUnlockEvent"`
	LoginLogoutEvent   *LoginLogoutEvent   `json:"loginLogoutEvent"`
	AddRemoveUserEvent *AddRemoveUserEvent `json:"addRemoveUserEvent"`
	LogUploadEvent     *LogUploadEvent     `json:"logUploadEvent"`
}

// DlpPolicyEvent mirrors the dlpPolicyEvent JSON field.
type DlpPolicyEvent struct {
	Restriction string `json:"restriction"`
	Mode        string `json:"mode"`
}

// LockUnlockEvent mirrors the lockUnlockEvent JSON field.
type LockUnlockEvent struct {
	Time           string          `json:"eventTimestampSec"`
	AffiliatedUser *AffiliatedUser `json:"affiliatedUser"`
	LockEvent      *LockEvent      `json:"lockEvent"`
	UnlockEvent    *UnlockEvent    `json:"unlockEvent"`
}

// LoginLogoutEvent mirrors the loginLogoutEvent JSON field.
type LoginLogoutEvent struct {
	Time           string          `json:"eventTimestampSec"`
	AffiliatedUser *AffiliatedUser `json:"affiliatedUser"`
	LoginEvent     *LoginEvent     `json:"loginEvent"`
	LogoutEvent    *LogoutEvent    `json:"logoutEvent"`
	SessionType    string          `json:"sessionType"`
}

// AddRemoveUserEvent mirrors the addRemoveUserEvent JSON field.
type AddRemoveUserEvent struct {
	Time             string            `json:"timestampMs"`
	AffiliatedUser   *AffiliatedUser   `json:"affiliatedUser"`
	UserAddedEvent   *UserAddedEvent   `json:"userAddedEvent"`
	UserRemovedEvent *UserRemovedEvent `json:"userRemovedEvent"`
}

// AffiliatedUser mirrors the affiliatedUser JSON field.
type AffiliatedUser struct {
	UserEmail string `json:"userEmail"`
}

// LockEvent mirrors the lockEvent JSON field.
type LockEvent struct{}

// UnlockEvent mirrors the unlockEvent JSON field.
type UnlockEvent struct {
	Success    bool   `json:"success"`
	UnlockType string `json:"unlockType"`
}

// UserAddedEvent mirrors the userAddedEvent JSON field.
type UserAddedEvent struct{}

// UserRemovedEvent mirrors the userRemovedEvent JSON field.
type UserRemovedEvent struct {
	Reason string `json:"reason"`
}

// LoginEvent mirrors the loginEvent JSON field.
type LoginEvent struct{}

// LogoutEvent mirrors the logoutEvent JSON field.
type LogoutEvent struct{}

// LogUploadEvent mirrors the logUploadEvent JSON field.
type LogUploadEvent struct {
	UploadSettings       *UploadSettings       `json:"uploadSettings"`
	UploadTracker        *UploadTracker        `json:"uploadTracker"`
	RemoteCommandDetails *RemoteCommandDetails `json:"remoteCommandDetails"`
}

// RemoteCommandDetails is a struct representation of message based on chrome/browser/policy/messaging_layer/proto/synced/log_upload_event.proto.
type RemoteCommandDetails struct {
	CommandID            *string `json:"commandId"`
	CommandResultPayload *string `json:"commandResultPayload"`
}

// UploadSettings is a struct representation of message based on components/reporting/proto/synced/upload_tracker.proto.
type UploadSettings struct {
	OriginPath       *string `json:"originPath"`
	RetryCount       *string `json:"retryCount"`
	UploadParameters *string `json:"uploadParameters"`
}

// UploadTracker is a struct representation of message based on components/reporting/proto/synced/upload_tracker.proto.
type UploadTracker struct {
	Status           *StatusProto `json:"status"`
	Total            *string      `json:"total"`
	Uploaded         *string      `json:"uploaded"`
	SessionToken     *string      `json:"sessionToken"`
	AccessParameters *string      `json:"accessParameters"`
}

// StatusProto is a struct representation of components/reporting/proto/synced/status.proto.
type StatusProto struct {
	Code         *string `json:"code"`
	ErrorMessage *string `json:"errorMessage"`
}

// MetricData mirrors the metricData JSON field.
type MetricData struct {
	Time          string         `json:"timestampMs"`
	InfoData      *InfoData      `json:"infoData"`
	TelemetryData *TelemetryData `json:"telemetryData"`
}

// InfoData mirrors the infoData JSON field.
type InfoData struct {
	MemoryInfo        *MemoryInfo        `json:"memoryInfo"`
	NetworkInfo       *NetworkInfo       `json:"networksInfo"`
	CPUInfo           *CPUInfo           `json:"cpuInfo"`
	DisplayInfo       *DisplayInfo       `json:"displayInfo"`
	PrivacyScreenInfo *PrivacyScreenInfo `json:"privacyScreenInfo"`
	TouchScreenInfo   *TouchScreenInfo   `json:"touchScreenInfo"`
}

// TelemetryData mirrors the telemetryData JSON field.
type TelemetryData struct {
	AudioTelemetry       *AudioTelemetry       `json:"audioTelemetry"`
	NetworksTelemetry    *NetworksTelemetry    `json:"networksTelemetry"`
	PeripheralsTelemetry *PeripheralsTelemetry `json:"peripheralsTelemetry"`
	DisplaysTelemetry    *DisplaysTelemetry    `json:"displaysTelemetry"`
}

// MemoryInfo mirrors the memoryInfo JSON field.
type MemoryInfo struct {
	TMEInfo *TMEInfo `json:"tmeInfo"`
}

// TMEInfo mirrors the TMEInfo JSON field.
type TMEInfo struct {
	MemoryEncryptionState     string `json:"encryptionState"`
	MaxKeys                   string `json:"maxKeys"`
	KeyLength                 string `json:"keyLength"`
	MemoryEncryptionAlgorithm string `json:"encryptionAlgorithm"`
}

// NetworkInfo represents a list of NetworkInterfaces.
type NetworkInfo struct {
	NetworkInterfaces []NetworkInterfaces `json:"networkInterfaces"`
}

// NetworkInterfaces mirrors the networkInterfaces JSON field.
type NetworkInterfaces struct {
	Type       string `json:"type"`
	MacAddress string `json:"macAddress"`
	DevicePath string `json:"devicePath"`
}

// CPUInfo mirrors the CPUInfo JSON field.
type CPUInfo struct {
	KeyLockerInfo *KeyLockerInfo `json:"keyLockerInfo"`
}

// KeyLockerInfo mirrors the KeyLockerInfo JSON field.
type KeyLockerInfo struct {
	Supported  bool `json:"supported"`
	Configured bool `json:"configured"`
}

// DisplayInfo lists the displayDevice JSON field.
type DisplayInfo struct {
	DisplayDevice []DisplayDevice `json:"displayDevice"`
}

// DisplayDevice mirrors the displayDevice json field.
type DisplayDevice struct {
	DisplayName     string `json:"displayName"`
	DisplayWidth    int32  `json:"displayWidth"`
	DisplayHeight   int32  `json:"displayHeight"`
	IsInternal      bool   `json:"isInternal"`
	Manufacturer    string `json:"manufacturer"`
	ModelID         int32  `json:"modelId"`
	ManufactureYear int32  `json:"manufactureYear"`
}

// TouchScreenInfo mirrors the touchScreenInfo JSON field.
type TouchScreenInfo struct {
	LibraryName        string               `json:"libraryName"`
	TouchScreenDevices []TouchScreenDevices `json:"touchScreenDevices"`
}

// TouchScreenDevices mirrors the touchScreenDevices JSON field.
type TouchScreenDevices struct {
	DisplayName string `json:"displayName"`
	TouchPoints int    `json:"touchPoints"`
	HasStylus   bool   `json:"hasStylus"`
}

// PrivacyScreenInfo mirrors the privacyScreenInfo field.
type PrivacyScreenInfo struct {
	Supported bool `json:"supported"`
}

// AudioTelemetry mirrors the audioTelemetry JSON field.
type AudioTelemetry struct {
	OutputMute       bool   `json:"outputMute"`
	InputMute        bool   `json:"inputMute"`
	OutputVolume     int32  `json:"outputVolume"`
	OutputDeviceName string `json:"outputDeviceName"`
	InputGain        int32  `json:"inputGain"`
	InputDeviceName  string `json:"inputDeviceName"`
}

// NetworksTelemetry mirrors the networksTelemetry JSON field.
type NetworksTelemetry struct {
	NetworkTelemetry []NetworkTelemetry `json:"networkTelemetry"`
	HTTPSLatencyData *HTTPSLatencyData  `json:"httpsLatencyData"`
}

// NetworkTelemetry mirrors the networkTelemetry JSON field.
type NetworkTelemetry struct {
	GUID            string `json:"guid"`
	ConnectionState string `json:"connectionState"`
	DevicePath      string `json:"devicePath"`
	IPAddress       string `json:"ipAddress"`
	Gateway         string `json:"gateway"`
	Type            string `json:"type"`
}

// HTTPSLatencyData mirrors httpsLatencyData JSON field.
type HTTPSLatencyData struct {
	Verdict   string `json:"verdict"`
	LatencyMs string `json:"latencyMs"`
}

// BandwidthData mirrors the bandwidthData JSON field.
type BandwidthData struct {
	DownloadSpeedKbps string `json:"downloadSpeedKbps"`
}

// PeripheralsTelemetry mirrors the peripheralsTelemetry JSON field.
type PeripheralsTelemetry struct {
	UsbTelemetry *UsbTelemetry `json:"usbTelemetry"`
}

// UsbTelemetry mirrors the usbTelemetry JSON field.
type UsbTelemetry struct {
	Vendor     string `json:"vendor"`
	Name       string `json:"name"`
	Vid        int32  `json:"vid"`
	Pid        int32  `json:"pid"`
	ClassID    int32  `json:"classId"`
	SubclassDd int32  `json:"subclassId"`
}

// DisplaysTelemetry mirrors the displaysTelemetry JSON field.
type DisplaysTelemetry struct {
	DisplayStatus []DisplayStatus `json:"displayStatus"`
}

// DisplayStatus mirrors the displayStatus JSON field.
type DisplayStatus struct {
	DisplayName          string `json:"displayName"`
	ResolutionVertical   int32  `json:"resolutionVertical"`
	ResolutionHorizontal int32  `json:"resolutionHorizontal"`
	RefreshRate          string `json:"refreshRate"`
}
