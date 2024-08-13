// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package glanceables

import "go.chromium.org/tast/core/testing"

// StudentAccountVarName is the student account name.
const StudentAccountVarName = "glanceables.Smoke.studentAccount"

// TeacherAccountVarName is the teacher account name.
const TeacherAccountVarName = "glanceables.Smoke.teacherAccount"

// RegularAccountVarName is the regular account name.
const RegularAccountVarName = "glanceables.Smoke.regularAccount"

const studentDMAAccountVarName = "glanceables.Smoke.studentDMAAccount"

const teacherDMAAccountVarName = "glanceables.Smoke.teacherDMAAccount"

const regularDMAAccountVarName = "glanceables.Smoke.regularDMAAccount"

var studentAccountVar = testing.RegisterVarString(
	StudentAccountVarName,
	"",
	"It contains creds in glanceables.Smoke.studentAccount",
)

var studentDMAAccountVar = testing.RegisterVarString(
	studentDMAAccountVarName,
	"",
	"It contains creds in glanceables.Smoke.studentDMAAccount",
)

var teacherAccountVar = testing.RegisterVarString(
	TeacherAccountVarName,
	"",
	"It contains creds in glanceables.Smoke.teacherAccount",
)

var teacherDMAAccountVar = testing.RegisterVarString(
	teacherDMAAccountVarName,
	"",
	"It contains creds in glanceables.Smoke.teacherDMAAccount",
)

var regularAccountVar = testing.RegisterVarString(
	RegularAccountVarName,
	"",
	"It contains creds in glanceables.Smoke.regularAccount",
)

var regularDMAAccountVar = testing.RegisterVarString(
	regularDMAAccountVarName,
	"",
	"It contains creds in glanceables.Smoke.regularDMAAccount",
)

// StudentAccountValue returns credentials from glanceables.Smoke.studentAccount.
func StudentAccountValue() string {
	return studentAccountVar.Value()
}

// StudentDMAAccountValue returns credentials from glanceables.Smoke.studentDMAAccount.
func StudentDMAAccountValue() string {
	return studentDMAAccountVar.Value()
}

// TeacherAccountValue returns credentials from glanceables.Smoke.teacherAccount.
func TeacherAccountValue() string {
	return teacherAccountVar.Value()
}

// TeacherDMAAccountValue returns credentials from glanceables.Smoke.teacherDMAAccount.
func TeacherDMAAccountValue() string {
	return teacherDMAAccountVar.Value()
}

// RegularAccountValue returns credentials from glanceables.Smoke.regularAccount.
func RegularAccountValue() string {
	return regularAccountVar.Value()
}

// RegularDMAAccountValue returns credentials from glanceables.Smoke.regularDMAAccount.
func RegularDMAAccountValue() string {
	return regularDMAAccountVar.Value()
}
