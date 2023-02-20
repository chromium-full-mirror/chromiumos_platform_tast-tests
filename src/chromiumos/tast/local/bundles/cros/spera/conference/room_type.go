// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

// RoomType defines room size for conference CUJ.
type RoomType int

const (
	// TwoRoomSize creates a conference room with 2 participants.
	TwoRoomSize RoomType = iota
	// SmallRoomSize creates a conference room with 5 participants.
	SmallRoomSize
	// LargeRoomSize creates a conference room with 16 participants.
	LargeRoomSize
	// ClassRoomSize creates a conference room with 35/49 participants.
	ClassRoomSize
)

// GoogleMeetRoomParticipants defines room size for Google meet cuj.
var GoogleMeetRoomParticipants = map[RoomType]int{
	TwoRoomSize:   2,
	SmallRoomSize: 6,
	LargeRoomSize: 16,
	ClassRoomSize: 35,
}

// ZoomRoomParticipants defines room size for Zoom cuj.
var ZoomRoomParticipants = map[RoomType]int{
	TwoRoomSize:   2,
	SmallRoomSize: 5,
	LargeRoomSize: 16,
	ClassRoomSize: 38,
}
