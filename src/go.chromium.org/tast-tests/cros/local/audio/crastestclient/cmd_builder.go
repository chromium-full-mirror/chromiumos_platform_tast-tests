// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crastestclient

import (
	"context"
	"fmt"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/audio/cras"
	"go.chromium.org/tast-tests/cros/common/testexec"
	audiopb "go.chromium.org/tast-tests/cros/services/cros/audio"
)

// SampleFormat is a sample format used for the cras_test_client --format
// argument.
type SampleFormat string

const cmdPath = "/usr/bin/cras_test_client"

// CmdBuilder is a utility for building cras_test_client commands, providing
// functions for supported arguments. Call Build to get the resultant Cmd.
//
// For further details on any of the arguments beyond what this provides, check
// the source directly. The help text for each argument is copied here for ease
// of use, but the cras_test_client itself is the main source of truth.
//
// Source: cras/src/tools/cras_test_client/cras_test_client.c::long_options
type CmdBuilder struct {
	args []string
}

// NewCmdBuilder creates a new CmdBuilder.
//
// Deprecated: Use cras_tests with testexec.CommandContext instead.
// See go/cras_test_client_deprecation.
func NewCmdBuilder() *CmdBuilder {
	return &CmdBuilder{}
}

// Build returns a new testexec.Cmd for the cras_audio_client cmd with all set
// arguments added.
//
// Note: If no arguments were added, the default action is the same as DumpServerInfo.
func (b *CmdBuilder) Build(ctx context.Context) *testexec.Cmd {
	return testexec.CommandContext(ctx, cmdPath, b.args...)
}

// intArgValue returns the int in the string format the cras_audio_client
// expects when parsing CLI arguments.
func (b *CmdBuilder) intArgValue(value int) string {
	return strconv.Itoa(value)
}

// boolArgValue returns the boolean in the string format the cras_audio_client
// expects when parsing CLI arguments.
func (b *CmdBuilder) boolArgValue(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

// ShowLatency adds the --show_latency argument.
//
// Display latency while playing or recording.
func (b *CmdBuilder) ShowLatency() *CmdBuilder {
	b.args = append(b.args, "--show_latency")
	return b
}

// ShowRMS adds the --show_rms argument.
//
// Display RMS value of loopback stream.
func (b *CmdBuilder) ShowRMS() *CmdBuilder {
	b.args = append(b.args, "--show_rms")
	return b
}

// ShowTotalRMS adds the --show_total_rms argument.
//
// Display total RMS value of loopback stream at the end.
func (b *CmdBuilder) ShowTotalRMS() *CmdBuilder {
	b.args = append(b.args, "--show_total_rms")
	return b
}

// SelectInput adds the --select_input argument.
// Select the ionode with the given id as preferred input.
func (b *CmdBuilder) SelectInput(nodeID *audiopb.CrasNodeID) *CmdBuilder {
	b.args = append(b.args, "--select_input", cras.MarshalNodeID(nodeID))
	return b
}

// BlockSize adds the --block_size argument.
//
// The number for frames per callback(dictates latency).
func (b *CmdBuilder) BlockSize(blockSize int) *CmdBuilder {
	b.args = append(b.args, "--block_size", b.intArgValue(blockSize))
	return b
}

// NumChannels adds the --num_channels argument.
//
// This specifies the number of audio channels. Two for stereo.
func (b *CmdBuilder) NumChannels(channels int) *CmdBuilder {
	b.args = append(b.args, "--num_channels", b.intArgValue(channels))
	return b
}

// DurationSeconds adds the --duration_seconds argument.
//
// This specifies the duration, in seconds, of audio to capture or playback.
// When left unset, the process continues until terminated.
func (b *CmdBuilder) DurationSeconds(durationSeconds int) *CmdBuilder {
	b.args = append(b.args, "--duration_seconds", b.intArgValue(durationSeconds))
	return b
}

// DumpEvents adds the --dump_events argument.
//
// Shows audio thread snapshots.
func (b *CmdBuilder) DumpEvents() *CmdBuilder {
	b.args = append(b.args, "--dump_events")
	return b
}

// Format adds the --format argument.
//
// This specifies the sampling format when capturing audio. Defaults to S16_LE.
//
// Supports S16_LE, S24_LE, and S32_LE formats.
func (b *CmdBuilder) Format(sampleFormat audiopb.SampleFormat) *CmdBuilder {
	sampleFormatArg, err := cras.MarshalSampleFormat(sampleFormat)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal SampleFormat: %v", err))
	}
	b.args = append(b.args, "--format", sampleFormatArg)
	return b
}

// CaptureGain adds the --capture_gain argument.
//
// Set system capture gain in dB*100 (100 = 1dB).
func (b *CmdBuilder) CaptureGain(captureGain int) *CmdBuilder {
	b.args = append(b.args, "--capture_gain", b.intArgValue(captureGain))
	return b
}

// DumpServerInfo adds the --dump_server_info argument.
//
// Print status of the server.
func (b *CmdBuilder) DumpServerInfo() *CmdBuilder {
	b.args = append(b.args, "--dump_server_info")
	return b
}

// CheckOutputPlugged adds the --check_output_plugged argument.
//
// Check if the output is plugged in.
func (b *CmdBuilder) CheckOutputPlugged(outputName string) *CmdBuilder {
	b.args = append(b.args, "--check_output_plugged", outputName)
	return b
}

// AddActiveInput adds the --add_active_input argument.
//
// Add the ionode with the given id to active input device list.
func (b *CmdBuilder) AddActiveInput(nodeID *audiopb.CrasNodeID) *CmdBuilder {
	b.args = append(b.args, "--add_active_input", cras.MarshalNodeID(nodeID))
	return b
}

// DumpDSP adds the --dump_dsp argument.
//
// Print status of dsp to syslog.
func (b *CmdBuilder) DumpDSP() *CmdBuilder {
	b.args = append(b.args, "--dump_dsp")
	return b
}

// DumpAudioThread adds the --dump_audio_thread argument.
//
// Dumps audio thread info.
func (b *CmdBuilder) DumpAudioThread() *CmdBuilder {
	b.args = append(b.args, "--dump_audio_thread")
	return b
}

// SyslogMask adds the --syslog_mask argument.
//
// Set the syslog mask to the given log level.
func (b *CmdBuilder) SyslogMask(logLevel int) *CmdBuilder {
	b.args = append(b.args, "--syslog_mask", b.intArgValue(logLevel))
	return b
}

// ChannelLayout adds the --channel_layout argument.
//
// Set multiple channel layout.
func (b *CmdBuilder) ChannelLayout(layout string) *CmdBuilder {
	b.args = append(b.args, "--channel_layout", layout)
	return b
}

// GetAECGroupID adds the --get_aec_group_id argument.
//
// Print the AEC group ID.
func (b *CmdBuilder) GetAECGroupID() *CmdBuilder {
	b.args = append(b.args, "--get_aec_group_id")
	return b
}

// UserMute adds the --user_mute argument.
//
// Set user mute state.
func (b *CmdBuilder) UserMute(userMute bool) *CmdBuilder {
	b.args = append(b.args, "--user_mute", b.boolArgValue(userMute))
	return b
}

// Rate adds the --rate argument.
//
// Specifies the sample rate in Hz.
func (b *CmdBuilder) Rate(sampleRate int) *CmdBuilder {
	b.args = append(b.args, "--rate", b.intArgValue(sampleRate))
	return b
}

// ReloadDSP adds the --reload_dsp argument.
//
// Reload dsp configuration from the ini file.
func (b *CmdBuilder) ReloadDSP() *CmdBuilder {
	b.args = append(b.args, "--reload_dsp")
	return b
}

// AddActiveOutput adds the --add_active_output argument.
//
// Add the ionode with the given id to active output device list.
func (b *CmdBuilder) AddActiveOutput(nodeID *audiopb.CrasNodeID) *CmdBuilder {
	b.args = append(b.args, "--add_active_output", cras.MarshalNodeID(nodeID))
	return b
}

// Mute adds the --mute argument.
//
// Set system mute state.
func (b *CmdBuilder) Mute(mute bool) *CmdBuilder {
	b.args = append(b.args, "--mute", b.boolArgValue(mute))
	return b
}

// Volume adds the --volume argument.
//
// Set system output volume (0-100).
func (b *CmdBuilder) Volume(volume int) *CmdBuilder {
	b.args = append(b.args, "--volume", b.intArgValue(volume))
	return b
}

// SetNodeVolume adds the --set_node_volume argument.
//
// Set the volume (0-100) of the ionode with the given ID.
func (b *CmdBuilder) SetNodeVolume(nodeID *audiopb.CrasNodeID, volume int) *CmdBuilder {
	argValue := fmt.Sprintf("%s:%d", cras.MarshalNodeID(nodeID), volume)
	b.args = append(b.args, "--set_node_volume", argValue)
	return b
}

// Plug adds the --plug argument.
//
// Set the plug state for the ionode with th given ID.
func (b *CmdBuilder) Plug(nodeID *audiopb.CrasNodeID, plug bool) *CmdBuilder {
	argValue := fmt.Sprintf("%s:%s", cras.MarshalNodeID(nodeID), b.boolArgValue(plug))
	b.args = append(b.args, "--plug", argValue)
	return b
}

// SelectOutput adds the --select_output argument.
//
// Select the ionode with the given id as preferred output.
func (b *CmdBuilder) SelectOutput(nodeID *audiopb.CrasNodeID) *CmdBuilder {
	b.args = append(b.args, "--select_output", cras.MarshalNodeID(nodeID))
	return b
}

// PlaybackDelayUS adds the --playback_delay_us argument.
//
// Set the time in microseconds to delay a reply for playback when i is pressed.
func (b *CmdBuilder) PlaybackDelayUS(delayInMicroseconds int) *CmdBuilder {
	b.args = append(b.args, "--playback_delay_us", b.intArgValue(delayInMicroseconds))
	return b
}

// CaptureMute adds the --capture_mute argument.
//
// Set capture mute state.
func (b *CmdBuilder) CaptureMute(captureMute bool) *CmdBuilder {
	b.args = append(b.args, "--capture_mute", b.boolArgValue(captureMute))
	return b
}

// RMActiveInput adds the --rm_active_input argument.
//
// Removes the ionode with the given ID from active input device list.
func (b *CmdBuilder) RMActiveInput(nodeID *audiopb.CrasNodeID) *CmdBuilder {
	b.args = append(b.args, "--rm_active_input", cras.MarshalNodeID(nodeID))
	return b
}

// RMActiveOutput adds the --rm_active_output argument.
//
// Removes the ionode with the given ID from active output device list.
func (b *CmdBuilder) RMActiveOutput(nodeID *audiopb.CrasNodeID) *CmdBuilder {
	b.args = append(b.args, "--rm_active_output", cras.MarshalNodeID(nodeID))
	return b
}

// SwapLeftRight adds the --swap_left_right argument.
//
// Swap or un-swap (true or false) the left and right channel for the
// ionode with the given ID.
func (b *CmdBuilder) SwapLeftRight(nodeID *audiopb.CrasNodeID, swap bool) *CmdBuilder {
	argValue := fmt.Sprintf("%s:%s", cras.MarshalNodeID(nodeID), b.boolArgValue(swap))
	b.args = append(b.args, "--swap_left_right", argValue)
	return b
}

// Version adds the --version argument.
//
// Print the git commit ID that was used to build the client.
func (b *CmdBuilder) Version() *CmdBuilder {
	b.args = append(b.args, "--version")
	return b
}

// AddTestDev adds the --add_test_dev argument.
//
// Add a test iodev.
func (b *CmdBuilder) AddTestDev(devType audiopb.CrasIODevType) *CmdBuilder {
	b.args = append(b.args, "--add_test_dev", b.intArgValue(int(devType.Number())))
	return b
}

// ListenForHotword adds the --listen_for_hotword argument.
//
// Listen and capture hotword stream if supported.
func (b *CmdBuilder) ListenForHotword(name string) *CmdBuilder {
	b.args = append(b.args, "--listen_for_hotword", name)
	return b
}

// PinDevice adds the --pin_device argument.
//
// Playback/Capture only on the given device.
func (b *CmdBuilder) PinDevice(deviceIndex int) *CmdBuilder {
	b.args = append(b.args, "--pin_device", b.intArgValue(deviceIndex))
	return b
}

// Suspend adds the --suspend argument.
//
// Set audio suspend state.
func (b *CmdBuilder) Suspend(suspend bool) *CmdBuilder {
	b.args = append(b.args, "--suspend", b.boolArgValue(suspend))
	return b
}

// SetNodeGain adds the --set_node_gain argument.
//
// Sets the capture gain for the node.
func (b *CmdBuilder) SetNodeGain(nodeID *audiopb.CrasNodeID, gain int) *CmdBuilder {
	argValue := fmt.Sprintf("%s:%d", cras.MarshalNodeID(nodeID), gain)
	b.args = append(b.args, "--set_node_gain", argValue)
	return b
}

// PlayShortSound adds the --play_short_sound argument.
//
// Plays the content in the file for the specified amount of periods when
// single-quote (') is pressed.
func (b *CmdBuilder) PlayShortSound(periods int) *CmdBuilder {
	b.args = append(b.args, "--play_short_sound", b.intArgValue(periods))
	return b
}

// SetHotwordModel adds the --set_hotword_model argument.
//
// Set the model to node.
func (b *CmdBuilder) SetHotwordModel(nodeID *audiopb.CrasNodeID, model string) *CmdBuilder {
	argValue := fmt.Sprintf("%s:%s", cras.MarshalNodeID(nodeID), model)
	b.args = append(b.args, "--set_hotword_model", argValue)
	return b
}

// GetHotwordModels adds the --get_hotword_models argument.
//
// Get the supported hotword models of node.
func (b *CmdBuilder) GetHotwordModels(nodeID *audiopb.CrasNodeID) *CmdBuilder {
	b.args = append(b.args, "--get_hotword_models", cras.MarshalNodeID(nodeID))
	return b
}

// PostDSP adds the --post_dsp argument.
//
// Use this flag with --loopback_file. The default value is 0.
func (b *CmdBuilder) PostDSP(option audiopb.PostDSPOption) *CmdBuilder {
	b.args = append(b.args, "--post_dsp", b.intArgValue(int(option.Number())))
	return b
}

// StreamID adds the --stream_id argument.
func (b *CmdBuilder) StreamID(streamID int) *CmdBuilder {
	b.args = append(b.args, "--stream_id", b.intArgValue(streamID))
	return b
}

// CaptureFile adds the --capture_file argument.
//
// Name of file to record to.
func (b *CmdBuilder) CaptureFile(captureFile string) *CmdBuilder {
	b.args = append(b.args, "--capture_file", captureFile)
	return b
}

// ReloadAECConfig adds the --reload_aec_config argument.
func (b *CmdBuilder) ReloadAECConfig() *CmdBuilder {
	b.args = append(b.args, "--reload_aec_config")
	return b
}

// Effects adds the --effects argument.
//
// Set specific effect(s) on stream parameters. The effects are combined into
// one argument using their bitmasks as specified by cras_test_client.
//
// Note: Invalid CaptureEffect values will cause a panic rather than an error
// to still allow for simple builder usage.
func (b *CmdBuilder) Effects(effects ...audiopb.CaptureEffect) *CmdBuilder {
	effectsArg, err := cras.MarshalCaptureEffects(effects...)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal capture effects: %v", err))
	}
	b.args = append(b.args, "--effects", effectsArg)
	return b
}

// GetAECSupported adds the --get_aec_supported argument.
//
// Prints whether AEC is supported.
func (b *CmdBuilder) GetAECSupported() *CmdBuilder {
	b.args = append(b.args, "--get_aec_supported")
	return b
}

// AECDump adds the --aecdump argument.
//
// Specifies that an AEC dump should be run and results saved to the specified
// file.
func (b *CmdBuilder) AECDump(aecDumpFile string) *CmdBuilder {
	b.args = append(b.args, "--aecdump", aecDumpFile)
	return b
}

// DumpBT adds the --dump_bt argument.
//
// Dumps debug info for bt audio.
func (b *CmdBuilder) DumpBT() *CmdBuilder {
	b.args = append(b.args, "--dump_bt")
	return b
}

// SetWbsEnabled adds the --set_wbs_enabled argument.
//
// This enables or disables WBS.
func (b *CmdBuilder) SetWbsEnabled(enabled bool) *CmdBuilder {
	b.args = append(b.args, "--set_wbs_enabled", b.boolArgValue(enabled))
	return b
}

// FollowATLog adds the --follow_atlog argument.
//
// Continuously dumps audio thread event log.
func (b *CmdBuilder) FollowATLog() *CmdBuilder {
	b.args = append(b.args, "--follow_atlog")
	return b
}

// ConnectionType adds the --connection_type argument.
//
// Set cras_client connection_type (defaults to control client).
func (b *CmdBuilder) ConnectionType(connectionType audiopb.CrasConnectionType) *CmdBuilder {
	b.args = append(b.args, "--connection_type", b.intArgValue(int(connectionType.Number())))
	return b
}

// LoopbackFile adds the --loopback_file argument.
//
// Name of file to record from loopback device.
func (b *CmdBuilder) LoopbackFile(loopbackFile string) *CmdBuilder {
	b.args = append(b.args, "--loopback_file", loopbackFile)
	return b
}

// MuteLoopTest adds the --mute_loop_test argument.
//
// Continuously loop mute/un-mute.
//
// If autoReconnect, it will automatically reconnect to CRAS when there is an
// error. Otherwise, it will stop on error.
func (b *CmdBuilder) MuteLoopTest(autoReconnect bool) *CmdBuilder {
	b.args = append(b.args, "--mute_loop_test", b.boolArgValue(autoReconnect))
	return b
}

// DumpMain adds the --dump_main argument.
//
// Dumps debug info from main thread.
func (b *CmdBuilder) DumpMain() *CmdBuilder {
	b.args = append(b.args, "--dump_main")
	return b
}

// SetAECRef adds the --set_aec_ref argument.
func (b *CmdBuilder) SetAECRef(aecRefDeviceID int) *CmdBuilder {
	b.args = append(b.args, "--set_aec_ref", b.intArgValue(aecRefDeviceID))
	return b
}

// PlaybackFile adds the --playback_file argument value.
//
// This specifies the file to playback. Use "-" to playback raw audio from
// stdin.
func (b *CmdBuilder) PlaybackFile(playbackFile string) *CmdBuilder {
	b.args = append(b.args, "--playback_file", playbackFile)
	return b
}

// ShowOOOTimestamp adds the --show_ooo_timestamp argument.
//
// Display out of order timestamps while playing or recording.
func (b *CmdBuilder) ShowOOOTimestamp() *CmdBuilder {
	b.args = append(b.args, "--show_ooo_timestamp")
	return b
}

// StreamType adds the --stream_type argument.
//
// Specify the type of the stream.
func (b *CmdBuilder) StreamType(streamType audiopb.CrasStreamType) *CmdBuilder {
	b.args = append(b.args, "--stream_type", b.intArgValue(int(streamType.Number())))
	return b
}

// PrintNodesInlined adds the --print_nodes_inlined argument.
//
// Print nodes table with devices inlined.
func (b *CmdBuilder) PrintNodesInlined() *CmdBuilder {
	b.args = append(b.args, "--print_nodes_inlined")
	return b
}

// RequestFloopMask adds the --request_floop_mask argument.
//
// Requests a flexible loopback device with the given mask.
//
// Prints the device ID; prints negative errno on error.
func (b *CmdBuilder) RequestFloopMask(mask int) *CmdBuilder {
	b.args = append(b.args, "--request_floop_mask", b.intArgValue(mask))
	return b
}

// ThreadPriority adds the --thread_priority argument.
//
// Set cras_test_client's thread priority.
//
// If this flag is not specified, it keeps the default behavior of setting rt
// priority, and fallbacks to niceness value.
//
// If threadPriority is "none", audio thread does not set any priority.
//
// If threadPriority is "rt:N", audio thread sets the rt priority to the integer
// value N. The policy is set to SCHED_RR.
//
// If threadPriority is "nice:N", audio thread sets the nice value to the
// integer value N.
func (b *CmdBuilder) ThreadPriority(threadPriority string) *CmdBuilder {
	b.args = append(b.args, "--thread_priority", threadPriority)
	return b
}

// ClientType adds the --client_type argument.
//
// Override the client type.
func (b *CmdBuilder) ClientType(clientType audiopb.CrasClientType) *CmdBuilder {
	b.args = append(b.args, "--client_type", b.intArgValue(int(clientType.Number())))
	return b
}

// DumpDSPOffload adds the --dump_dsp_offload argument.
//
// Print status of DSP offload for supported devices.
func (b *CmdBuilder) DumpDSPOffload() *CmdBuilder {
	b.args = append(b.args, "--dump_dsp_offload")
	return b
}
