// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package uidetection provides image-based UI detections/interactions.
package uidetection

import (
	"context"

	pb "go.chromium.org/tast-tests/cros/local/uidetection/api"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/tast/core/errors"
)

type uiDetector struct {
	keyType string
	key     string
	server  string
}

const retryPolicy = `{
	"methodConfig": [{
		"name": [{ "service": "google.chromeos.uidetection.v1.UiDetectionService", "method": "ExecuteDetection" }],
		"timeout": "60s",
		"retryPolicy": {
		  "maxAttempts": 5,
		  "initialBackoff": "1s",
		  "maxBackoff": "10s",
		  "backoffMultiplier": 1.3,
		  "retryableStatusCodes": ["UNAVAILABLE"]
		}
	}]}`

func (d *uiDetector) sendDetectionRequest(ctx context.Context, imagePng []byte, request *pb.DetectionRequest, resizingStrategy ScreenshotResizingStrategy, testMetadata *pb.TestMetadata) (*pb.UiDetectionResponse, error) {
	// Create the UI detection request.
	resizeImage := resizingStrategy == ResizeAsFallback
	uiDetectionRequest := &pb.UiDetectionRequest{
		ImagePng:           imagePng,
		Request:            request,
		ResizeImage:        &resizeImage,
		ForceImageResizing: resizingStrategy == AlwaysResize,
		TestMetadata:       testMetadata,
	}

	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(d.keyType, d.key))

	// Establish grpc connection to the server.
	conn, err := grpc.Dial(
		d.server,
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithDefaultServiceConfig(retryPolicy),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish connection to ui detection server")
	}
	defer conn.Close()

	client := pb.NewUiDetectionServiceClient(conn)

	return client.ExecuteDetection(ctx, uiDetectionRequest)
}
