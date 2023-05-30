// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package socialapp implements helpers for the power.SocialApp* tests.
package socialapp

import "context"

// SocialApp defines the operations applied in social app.
type SocialApp interface {
	Install(ctx context.Context) error
	Uninstall(ctx context.Context) error
	Launch(ctx context.Context) error
	Close(ctx context.Context) error
	SetUp(ctx context.Context) error
	CleanUp(ctx context.Context) error
	SendMessages(ctx context.Context) error
	RunExtraOperations(ctx context.Context) error
}
