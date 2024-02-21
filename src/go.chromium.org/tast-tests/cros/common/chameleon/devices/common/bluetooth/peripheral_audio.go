// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/xmlrpc"
	"go.chromium.org/tast/core/errors"
)

// audioRecordingEntity will be the test data key Chameleond uses to get the
// filename for audio file capturing. This was for the Tauto client
// implementation, which is abstracted out in this implementation. Filenames
// instead are just specified in passed AudioFile data.
const audioRecordingEntity = "recorded_by_peer"

// AudioConfig stores audio configuration values as used by SetAudioConfig.
type AudioConfig struct {
	// AudioServer is the audio server to use.
	AudioServer AudioServer

	// A2DPCodec is the A2DP audio codec to use.
	A2DPCodec A2DPCodec

	// HPCodec is the HFP audio codec to use.
	HFPCodec HFPCodec
}

// Update will overwrite this config's values with any non-empty values in newConfig.
func (ac *AudioConfig) Update(newConfig *AudioConfig) {
	if newConfig.AudioServer != "" {
		ac.AudioServer = newConfig.AudioServer
	}
	if newConfig.A2DPCodec != "" {
		ac.A2DPCodec = newConfig.A2DPCodec
	}
	if newConfig.HFPCodec != "" {
		ac.HFPCodec = newConfig.HFPCodec
	}
}

// marshall returns the AudioConfig in the format Chameleond expects in method calls.
func (ac *AudioConfig) marshall() map[string]string {
	configMap := make(map[string]string)
	if ac.AudioServer != "" {
		configMap["audio_server"] = ac.AudioServer.String()
	}
	if ac.A2DPCodec != "" {
		configMap["a2dp_codec"] = ac.A2DPCodec.String()
	}
	if ac.HFPCodec != "" {
		configMap["hfp_codec"] = ac.HFPCodec.String()
	}
	return configMap
}

// String returns this audio config as a string for logging purposes.
func (ac *AudioConfig) String() string {
	configJSON, err := json.Marshal(ac.marshall())
	if err != nil {
		panic("failed to marshall AudioConfig map to JSON")
	}
	return string(configJSON)
}

// AudioFile describes an audio file on the chameleond host.
type AudioFile struct {
	// DeviceFile is the path to the file on the chameleond host.
	DeviceFile string

	// SampleFormat is the sample format to use with the audio file during
	// playback or capturing.
	//
	// The value expected to be in the format pulseaudio (pacat) command or
	// pipewire (pw-record) command expects. The value will be converted to
	// lowercase and underscores will be replaced by spaces before using it on the
	// chameleond server.
	SampleFormat string

	// SampleRate is the sample rate to use with the audio file during
	// playback or capturing.
	SampleRate int

	// Channels is the amount of channels to use with the audio file during
	// playback or capturing.
	Channels int
}

// marshall returns the AudioFile in the format Chameleond expects in method calls.
func (af *AudioFile) marshall() map[string]interface{} {
	audioFileMap := make(map[string]interface{})
	if af.DeviceFile != "" {
		audioFileMap["device_file"] = af.DeviceFile
	}
	if af.SampleFormat != "" {
		audioFileMap["format"] = af.SampleFormat
	}
	if af.SampleRate != 0 {
		audioFileMap["rate"] = af.SampleRate
	}
	if af.Channels != 0 {
		audioFileMap["channels"] = af.Channels
	}
	return audioFileMap
}

// String returns this audio file as a string for logging purposes.
func (af *AudioFile) String() string {
	audioFileJSON, err := json.Marshal(af.marshall())
	if err != nil {
		panic("failed to marshall AudioFile map to JSON")
	}
	return string(audioFileJSON)
}

// AudioPeripheral is an interface for making RPC calls to a chameleond daemon
// targeting a specific bluetooth audio peripheral chameleon device flow.
//
// This is based off of the Python class "chameleond.utils.bluetooth_audio.BluetoothAudio"
// from the chameleon source. Refer to that source for more complete
// documentation.
type AudioPeripheral interface {
	BluezPeripheral

	// GetUserIDOfPi calls the "GetUserIdOfPi" Chameleond RPC method.
	// Gets the user ID for the user pi.
	// Returns the pi's user ID.
	//
	// Note that the casing of the Chameleond RPC method name differs from this
	// method name slightly to meet golang standards.
	GetUserIDOfPi(ctx context.Context) (string, error)

	// GetPipewireBluezID calls the "GetPipewireBluezId" Chameleond RPC method.
	// Gets the pipewire bluez id.
	//
	// Uses `wpctl status` to get the id of the bluez audio device.
	//
	// Returns the audio device ID if available, or -1 if not available.
	//
	// Note that the casing of the Chameleond RPC method name differs from this
	// method name slightly to meet golang standards.
	GetPipewireBluezID(ctx context.Context) (int, error)

	// StartPipewire calls the Chameleond RPC method of the same name.
	// Starts the pipewire processes.
	//
	// May need to retry a few times as sometimes they may not start correctly.
	StartPipewire(ctx context.Context) error

	// StopPipewire calls the Chameleond RPC method of the same name.
	// Stops the Pipewire processes.
	StopPipewire(ctx context.Context) error

	// GetAudioServerName calls the Chameleond RPC method of the same name.
	// Gets the audio server name.
	GetAudioServerName(ctx context.Context) (AudioServer, error)

	// StartAudioServer calls the Chameleond RPC method of the same name.
	// Starts the audio server.
	//
	// There are multiple audio servers including pulseaudio and pipewire.
	// Starts the specified audio server after stopping the other non-used one.
	StartAudioServer(ctx context.Context, audioProfile AudioProfile) error

	// StopAudioServer calls the Chameleond RPC method of the same name.
	// Stops the configured audio server (pulseaudio or pipewire).
	StopAudioServer(ctx context.Context) error

	// SetAudioConfig calls the Chameleond RPC method of the same name.
	// Sets the audio configuration.
	SetAudioConfig(ctx context.Context, audioConfig *AudioConfig) error

	// StartPulseaudio calls the Chameleond RPC method of the same name.
	// Starts the pulseaudio process.
	StartPulseaudio(ctx context.Context, audioProfile AudioProfile) error

	// StopPulseaudio calls the Chameleond RPC method of the same name.
	// Stops the pulseaudio process.
	StopPulseaudio(ctx context.Context) error

	// StartOfono calls the Chameleond RPC method of the same name.
	// Starts/restarts the Ofono process.
	StartOfono(ctx context.Context) error

	// StopOfono calls the Chameleond RPC method of the same name.
	// Stops the Ofono process.
	StopOfono(ctx context.Context) error

	// PlayAudio calls the Chameleond RPC method of the same name.
	// Plays the audio file located at audioFile on the chameleon device.
	PlayAudio(ctx context.Context, audioFile string) error

	// StartPlayingAudioSubprocess calls the Chameleond RPC method of the same name.
	// Starts playing the audio file in a subprocess.
	StartPlayingAudioSubprocess(ctx context.Context, audioProfile AudioProfile, srcAudioFile *AudioFile, waitSecs int) error

	// StopPlayingAudioSubprocess calls the Chameleond RPC method of the same name.
	// Stops playing the audio file in the subprocess.
	StopPlayingAudioSubprocess(ctx context.Context) error

	// ListCards calls the Chameleond RPC method of the same name.
	// Lists all sound cards.
	// Returns the pactl command call output as a string.
	ListCards(ctx context.Context, audioProfile AudioProfile) (string, error)

	// ListSources calls the Chameleond RPC method of the same name.
	// Lists all audio source cards.
	// Returns the pactl command call output as a string.
	ListSources(ctx context.Context, audioProfile AudioProfile) (string, error)

	// ListSinks calls the Chameleond RPC method of the same name.
	// Lists all audio sinks.
	// Returns the pactl command call output as a string.
	ListSinks(ctx context.Context, audioProfile AudioProfile) (string, error)

	// GetBluezSourceDevice calls the Chameleond RPC method of the same name.
	// Gets the number of the bluez source device.
	GetBluezSourceDevice(ctx context.Context, audioProfile AudioProfile) (string, error)

	// GetBluezSourceHFPDevice calls the Chameleond RPC method of the same name.
	// Gets the number of the HFP bluez source device.
	GetBluezSourceHFPDevice(ctx context.Context, audioProfile AudioProfile) (int, error)

	// GetBluezSinkHFPDevice calls the Chameleond RPC method of the same name.
	// Gets the number of the HFB bluez sink device.
	GetBluezSinkHFPDevice(ctx context.Context, audioProfile AudioProfile) (int, error)

	// GetBluezSourceA2DPDevice calls the Chameleond RPC method of the same name.
	// Gets the number of the A2DP bluez sink device.
	GetBluezSourceA2DPDevice(ctx context.Context, audioProfile AudioProfile) (int, error)

	// StartRecordingAudioSubprocessPulseaudio calls the Chameleond RPC method of
	// the same name.
	// Starts recording audio in a subprocess using pulseaudio.
	//
	// Note: The testData and recordingEntity params are restructured in this client
	// as a single AudioFile parameter for ease of use.
	StartRecordingAudioSubprocessPulseaudio(ctx context.Context, audioProfile AudioProfile, dstAudioFile *AudioFile) error

	// StartRecordingAudioSubprocessPipewire calls the Chameleond RPC method of
	// the same name.
	// Starts recording audio in a subprocess using pipewire.
	//
	// Note: The testData and recordingEntity params are restructured in this client
	// as a single AudioFile parameter for ease of use.
	StartRecordingAudioSubprocessPipewire(ctx context.Context, audioProfile AudioProfile, dstAudioFile *AudioFile) error

	// StartRecordingAudioSubprocess calls the Chameleond RPC method of the same name.
	// Starts recording audio in a subprocess.
	//
	// Note: The testData and recordingEntity params are restructured in this client
	// as a single AudioFile parameter for ease of use.
	StartRecordingAudioSubprocess(ctx context.Context, audioProfile AudioProfile, dstAudioFile *AudioFile) error

	// HandleOneChunk calls the Chameleond RPC method of the same name.
	// Saves one chunk of data into a file and remote copies it to the DUT.
	// Returns the chuck filename if successful.
	HandleOneChunk(ctx context.Context, chunkInSecs, index int, dutIP string) (string, error)

	// StopRecordingAudioSubprocess calls the "StopRecordingingAudioSubprocess"
	// Chameleond RPC method.
	// Stops the recording subprocess.
	//
	// Note that the Chameleond RPC method name has a typo which is fixed in this
	// method name.
	StopRecordingAudioSubprocess(ctx context.Context) error

	// ScpToDut calls the Chameleond RPC method of the same name.
	// Copies the srcFile to the dstFile via scp at dutIP.
	ScpToDut(ctx context.Context, srcFile, dstFile, dutIP string) error

	// ExportMediaPlayer calls the Chameleond RPC method of the same name.
	// Exports the Bluetooth media player.
	// Returns true if one and only one mpris-proxy is running.
	ExportMediaPlayer(ctx context.Context) (bool, error)

	// UnexportMediaPlayer calls the Chameleond RPC method of the same name.
	// Stops all mpris-proxy processes.
	UnexportMediaPlayer(ctx context.Context) error

	// GetExportedMediaPlayer calls the Chameleond RPC method of the same name.
	// Gets the exported media player's name with playerctl.
	// Returns the player name if only one player found, empty string otherwise.
	GetExportedMediaPlayer(ctx context.Context) (string, error)

	// SendMediaPlayerCommand calls the Chameleond RPC method of the same name.
	// Executes the command towards the given player.
	SendMediaPlayerCommand(ctx context.Context, command string) error

	// GetMediaPlayerMediaInfo calls the Chameleond RPC method of the same name.
	// Retrieves media information through playerctl calls.
	GetMediaPlayerMediaInfo(ctx context.Context) (map[string]string, error)
}

// CommonAudioPeripheral is a base implementation of AudioPeripheral that
// provides methods for making XMLRPC calls to a chameleond daemon.
// See the AudioPeripheral interface for more detailed documentation.
type CommonAudioPeripheral struct {
	CommonBluezPeripheral
}

// NewCommonAudioPeripheral creates a new instance of CommonAudioPeripheral.
func NewCommonAudioPeripheral(xmlrpcClient *xmlrpc.XMLRpc, methodNamePrefix string) *CommonAudioPeripheral {
	return &CommonAudioPeripheral{
		CommonBluezPeripheral: *NewCommonBluezPeripheral(xmlrpcClient, methodNamePrefix),
	}
}

// GetUserIDOfPi calls the "GetUserIdOfPi" Chameleond RPC method.
// This implements AudioPeripheral.GetUserIDOfPi, see that for more details.
//
// Note that the casing of the Chameleond RPC method name differs from this
// method name slightly to meet golang standards.
func (c *CommonAudioPeripheral) GetUserIDOfPi(ctx context.Context) (string, error) {
	return c.RPC("GetUserIdOfPi").CallForString(ctx)
}

// callForIntOrNil calls the method for an int return value, but will return -1
// if the method returns nil instead. This works around missing support for nil
// values in the XMLRPC client library.
func (c *CommonAudioPeripheral) callForIntOrNil(ctx context.Context, callBuilder *xmlrpc.CallBuilder) (int, error) {
	result := -1
	err := callBuilder.Returns(&result).Call(ctx)
	if err != nil {
		// Will return nil if no ID present, but the XMLRPC client does not support
		// this, so parse the error.
		if strings.Contains(err.Error(), "value <empty> is not an int value") {
			return -1, nil
		}
		return 0, err
	}
	return result, nil
}

// GetPipewireBluezID calls the "GetPipewireBluezId" Chameleond RPC method.
// This implements AudioPeripheral.GetPipewireBluezID, see that for more details.
//
// Note that the casing of the Chameleond RPC method name differs from this
// method name slightly to meet golang standards.
//
// Returns -1 if GetPipewireBluezId returned no results.
//
// Will attempt to get the ID for up to 12s.
func (c *CommonAudioPeripheral) GetPipewireBluezID(ctx context.Context) (int, error) {
	return c.callForIntOrNil(ctx, c.RPC("GetPipewireBluezId").Timeout(12*time.Second))
}

// StartPipewire calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StartPipewire, see that for more details.
func (c *CommonAudioPeripheral) StartPipewire(ctx context.Context) error {
	return c.RPC("StartPipewire").CallForBoolSuccess(ctx)
}

// StopPipewire calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StopPipewire, see that for more details.
func (c *CommonAudioPeripheral) StopPipewire(ctx context.Context) error {
	return c.RPC("StopPipewire").CallForBoolSuccess(ctx)
}

// GetAudioServerName calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.GetAudioServerName, see that for more details.
func (c *CommonAudioPeripheral) GetAudioServerName(ctx context.Context) (AudioServer, error) {
	audioServer, err := c.RPC("GetAudioServerName").CallForString(ctx)
	if err != nil {
		return "", err
	}
	return AudioServer(audioServer), err
}

// StartAudioServer calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StartAudioServer, see that for more details.
func (c *CommonAudioPeripheral) StartAudioServer(ctx context.Context, audioProfile AudioProfile) error {
	return c.RPC("StartAudioServer").Args(audioProfile.String()).Timeout(30 * time.Second).CallForBoolSuccess(ctx)
}

// StopAudioServer calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StopAudioServer, see that for more details.
func (c *CommonAudioPeripheral) StopAudioServer(ctx context.Context) error {
	return c.RPC("StopAudioServer").CallForBoolSuccess(ctx)
}

// SetAudioConfig calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.SetAudioConfig, see that for more details.
func (c *CommonAudioPeripheral) SetAudioConfig(ctx context.Context, audioConfig *AudioConfig) error {
	return c.RPC("SetAudioConfig").Args(audioConfig.marshall()).CallForBoolSuccess(ctx)
}

// StartPulseaudio calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StartPulseaudio, see that for more details.
func (c *CommonAudioPeripheral) StartPulseaudio(ctx context.Context, audioProfile AudioProfile) error {
	return c.RPC("StartPulseaudio").Timeout(30 * time.Second).Args(audioProfile.String()).CallForBoolSuccess(ctx)
}

// StopPulseaudio calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StopPulseaudio, see that for more details.
func (c *CommonAudioPeripheral) StopPulseaudio(ctx context.Context) error {
	return c.RPC("StopPulseaudio").CallForBoolSuccess(ctx)
}

// StartOfono calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StartOfono, see that for more details.
func (c *CommonAudioPeripheral) StartOfono(ctx context.Context) error {
	return c.RPC("StartOfono").CallForBoolSuccess(ctx)
}

// StopOfono calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StopOfono, see that for more details.
func (c *CommonAudioPeripheral) StopOfono(ctx context.Context) error {
	return c.RPC("StopOfono").CallForBoolSuccess(ctx)
}

// PlayAudio calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.PlayAudio, see that for more details.
func (c *CommonAudioPeripheral) PlayAudio(ctx context.Context, audioFile string) error {
	return c.RPC("PlayAudio").Args(audioFile).CallForBoolSuccess(ctx)
}

// StartPlayingAudioSubprocess calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StartPlayingAudioSubprocess, see that for
// more details.
func (c *CommonAudioPeripheral) StartPlayingAudioSubprocess(ctx context.Context, audioProfile AudioProfile, srcAudioFile *AudioFile, waitSecs int) error {
	return c.RPC("StartPlayingAudioSubprocess").Args(audioProfile.String(), srcAudioFile.marshall(), waitSecs).CallForBoolSuccess(ctx)
}

// StopPlayingAudioSubprocess calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.StopPlayingAudioSubprocess, see that for more
// details.
func (c *CommonAudioPeripheral) StopPlayingAudioSubprocess(ctx context.Context) error {
	return c.RPC("StopPlayingAudioSubprocess").CallForBoolSuccess(ctx)
}

// ListCards calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.ListCards, see that for more details.
func (c *CommonAudioPeripheral) ListCards(ctx context.Context, audioProfile AudioProfile) (string, error) {
	return c.RPC("ListCards").Args(audioProfile.String()).CallForString(ctx)
}

// ListSources calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.ListSources, see that for more details.
func (c *CommonAudioPeripheral) ListSources(ctx context.Context, audioProfile AudioProfile) (string, error) {
	return c.RPC("ListSources").Args(audioProfile.String()).CallForString(ctx)
}

// ListSinks calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.ListSinks, see that for more details.
func (c *CommonAudioPeripheral) ListSinks(ctx context.Context, audioProfile AudioProfile) (string, error) {
	return c.RPC("ListSinks").Args(audioProfile.String()).CallForString(ctx)
}

// GetBluezSourceDevice calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.GetBluezSourceDevice, see that for more
// details.
func (c *CommonAudioPeripheral) GetBluezSourceDevice(ctx context.Context, audioProfile AudioProfile) (string, error) {
	return c.RPC("GetBluezSourceDevice").Args(audioProfile.String()).CallForString(ctx)
}

// GetBluezSourceHFPDevice calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.GetBluezSourceHFPDevice, see that for more
// details.
func (c *CommonAudioPeripheral) GetBluezSourceHFPDevice(ctx context.Context, audioProfile AudioProfile) (int, error) {
	return c.callForIntOrNil(ctx, c.RPC("GetBluezSourceHFPDevice").Args(audioProfile))
}

// GetBluezSinkHFPDevice calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.GetBluezSinkHFPDevice, see that for more
// details.
func (c *CommonAudioPeripheral) GetBluezSinkHFPDevice(ctx context.Context, audioProfile AudioProfile) (int, error) {
	return c.callForIntOrNil(ctx, c.RPC("GetBluezSinkHFPDevice").Args(audioProfile))
}

// GetBluezSourceA2DPDevice calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.GetBluezSourceA2DPDevice, see that for more
// details.
func (c *CommonAudioPeripheral) GetBluezSourceA2DPDevice(ctx context.Context, audioProfile AudioProfile) (int, error) {
	val, err := c.RPC("GetBluezSourceA2DPDevice").Args(audioProfile.String()).CallForString(ctx)
	if err != nil {
		return 0, err
	}
	sourceDeviceInt, err := strconv.Atoi(val)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse result of successful GetBluezSourceA2DPDevice RPC call %q as an int", val)
	}
	return sourceDeviceInt, nil
}

// StartRecordingAudioSubprocessPulseaudio calls the Chameleond RPC method of
// the same name.
// This implements AudioPeripheral.StartRecordingAudioSubprocessPulseaudio, see
// that for more details.
func (c *CommonAudioPeripheral) StartRecordingAudioSubprocessPulseaudio(ctx context.Context, audioProfile AudioProfile, dstAudioFile *AudioFile) error {
	testData := dstAudioFile.marshall()
	testData[audioRecordingEntity] = dstAudioFile.DeviceFile
	return c.RPC("StartRecordingAudioSubprocessPulseaudio").Args(audioProfile.String(), dstAudioFile.marshall(), audioRecordingEntity).CallForBoolSuccess(ctx)
}

// StartRecordingAudioSubprocessPipewire calls the Chameleond RPC method of the
// same name.
// This implements AudioPeripheral.StartRecordingAudioSubprocessPipewire, see
// that for more details.
func (c *CommonAudioPeripheral) StartRecordingAudioSubprocessPipewire(ctx context.Context, audioProfile AudioProfile, dstAudioFile *AudioFile) error {
	testData := dstAudioFile.marshall()
	testData[audioRecordingEntity] = dstAudioFile.DeviceFile
	return c.RPC("StartRecordingAudioSubprocessPipewire").Args(audioProfile.String(), testData).CallForBoolSuccess(ctx)
}

// StartRecordingAudioSubprocess calls the Chameleond RPC method of the same
// name. This implements AudioPeripheral.StartRecordingAudioSubprocess, see that
// for more details.
func (c *CommonAudioPeripheral) StartRecordingAudioSubprocess(ctx context.Context, audioProfile AudioProfile, dstAudioFile *AudioFile) error {
	testData := dstAudioFile.marshall()
	testData[audioRecordingEntity] = dstAudioFile.DeviceFile
	return c.RPC("StartRecordingAudioSubprocess").Args(audioProfile.String(), testData, audioRecordingEntity).CallForBoolSuccess(ctx)
}

// HandleOneChunk calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.HandleOneChunk, see that for more details.
func (c *CommonAudioPeripheral) HandleOneChunk(ctx context.Context, chunkInSecs, index int, dutIP string) (string, error) {
	return c.RPC("HandleOneChunk").Args(chunkInSecs, index, dutIP).CallForString(ctx)
}

// StopRecordingAudioSubprocess calls the "StopRecordingingAudioSubprocess"
// Chameleond RPC method.
// This implements AudioPeripheral.StopRecordingAudioSubprocess, see
// that for more details.
//
// Note that the Chameleond RPC method name has a typo which is fixed in this
// method name.
func (c *CommonAudioPeripheral) StopRecordingAudioSubprocess(ctx context.Context) error {
	return c.RPC("StopRecordingingAudioSubprocess").CallForBoolSuccess(ctx)
}

// ScpToDut calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.ScpToDut, see that for more details.
func (c *CommonAudioPeripheral) ScpToDut(ctx context.Context, srcFile, dstFile, dutIP string) error {
	return c.RPC("ScpToDut").Args(srcFile, dstFile, dutIP).Call(ctx)
}

// ExportMediaPlayer calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.ExportMediaPlayer, see that for more details.
func (c *CommonAudioPeripheral) ExportMediaPlayer(ctx context.Context) (bool, error) {
	return c.RPC("ExportMediaPlayer").CallForBool(ctx)
}

// UnexportMediaPlayer calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.UnexportMediaPlayer, see that for more
// details.
func (c *CommonAudioPeripheral) UnexportMediaPlayer(ctx context.Context) error {
	return c.RPC("UnexportMediaPlayer").Call(ctx)
}

// GetExportedMediaPlayer calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.GetExportedMediaPlayer, see that for more
// details.
func (c *CommonAudioPeripheral) GetExportedMediaPlayer(ctx context.Context) (string, error) {
	return c.RPC("GetExportedMediaPlayer").CallForString(ctx)
}

// SendMediaPlayerCommand calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.SendMediaPlayerCommand, see that for more
// details.
func (c *CommonAudioPeripheral) SendMediaPlayerCommand(ctx context.Context, command string) error {
	return c.RPC("SendMediaPlayerCommand").Args(command).CallForBoolSuccess(ctx)
}

// GetMediaPlayerMediaInfo calls the Chameleond RPC method of the same name.
// This implements AudioPeripheral.GetMediaPlayerMediaInfo, see that for more
// details.
func (c *CommonAudioPeripheral) GetMediaPlayerMediaInfo(ctx context.Context) (map[string]string, error) {
	var mediaPlayerInfo map[string]string
	err := c.RPC("GetMediaPlayerMediaInfo").Returns(&mediaPlayerInfo).Call(ctx)
	if err != nil {
		return nil, err
	}
	return mediaPlayerInfo, nil
}
