// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cras

import (
	"reflect"
	"testing"

	audiopb "go.chromium.org/tast-tests/cros/services/cros/audio"
)

func TestMarshalNodeID(t *testing.T) {
	type args struct {
		nodeID *audiopb.CrasNodeID
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"default values allowed",
			args{
				nodeID: &audiopb.CrasNodeID{},
			},
			"0:0",
		},
		{
			"distinct values",
			args{
				nodeID: &audiopb.CrasNodeID{
					DeviceIndex: 1,
					NodeIndex:   2,
				},
			},
			"1:2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MarshalNodeID(tt.args.nodeID); got != tt.want {
				t.Errorf("MarshalNodeID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarshalNodeType(t *testing.T) {
	type args struct {
		nodeType audiopb.CrasNodeType
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"invalid",
			args{
				nodeType: -1,
			},
			"",
			true,
		},
		{
			"valid removes enum prefix",
			args{
				nodeType: audiopb.CrasNodeType_CRAS_NODE_TYPE_BLUETOOTH,
			},
			"BLUETOOTH",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarshalNodeType(tt.args.nodeType)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalNodeType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MarshalNodeType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarshalSampleFormat(t *testing.T) {
	type args struct {
		sampleFormat audiopb.SampleFormat
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"invalid",
			args{
				sampleFormat: -1,
			},
			"",
			true,
		},
		{
			"valid removes enum prefix",
			args{
				sampleFormat: audiopb.SampleFormat_SAMPLE_FORMAT_S24_LE,
			},
			"S24_LE",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarshalSampleFormat(tt.args.sampleFormat)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalSampleFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MarshalSampleFormat() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnmarshalNodeID(t *testing.T) {
	type args struct {
		nodeID string
	}
	tests := []struct {
		name    string
		args    args
		want    *audiopb.CrasNodeID
		wantErr bool
	}{
		{
			"empty string",
			args{
				nodeID: "",
			},
			nil,
			true,
		},
		{
			"bad device index",
			args{
				nodeID: "a:0",
			},
			nil,
			true,
		},
		{
			"bad node index",
			args{
				nodeID: "0:a",
			},
			nil,
			true,
		},
		{
			"valid id",
			args{
				nodeID: "1:2",
			},
			&audiopb.CrasNodeID{
				DeviceIndex: 1,
				NodeIndex:   2,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalNodeID(tt.args.nodeID)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalNodeID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnmarshalNodeID() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnmarshalNodeType(t *testing.T) {
	type args struct {
		nodeType string
	}
	tests := []struct {
		name    string
		args    args
		want    audiopb.CrasNodeType
		wantErr bool
	}{
		{
			"empty",
			args{
				nodeType: "",
			},
			0,
			true,
		},
		{
			"invalid",
			args{
				nodeType: "bad string",
			},
			0,
			true,
		},
		{
			"valid without enum prefix",
			args{
				nodeType: "BLUETOOTH",
			},
			audiopb.CrasNodeType_CRAS_NODE_TYPE_BLUETOOTH,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalNodeType(tt.args.nodeType)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalNodeType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("UnmarshalNodeType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnmarshalSampleFormat(t *testing.T) {
	type args struct {
		sampleFormat string
	}
	tests := []struct {
		name    string
		args    args
		want    audiopb.SampleFormat
		wantErr bool
	}{
		{
			"empty",
			args{
				sampleFormat: "",
			},
			0,
			true,
		},
		{
			"invalid",
			args{
				sampleFormat: "bad string",
			},
			0,
			true,
		},
		{
			"valid without enum prefix",
			args{
				sampleFormat: "S32_LE",
			},
			audiopb.SampleFormat_SAMPLE_FORMAT_S32_LE,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalSampleFormat(tt.args.sampleFormat)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalSampleFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("UnmarshalSampleFormat() got = %v, want %v", got, tt.want)
			}
		})
	}
}
