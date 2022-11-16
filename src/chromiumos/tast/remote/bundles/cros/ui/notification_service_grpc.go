// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	"chromiumos/tast/common/chrome/ash"
	"chromiumos/tast/remote/crosserverutil"
	pb "chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NotificationServiceGRPC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check basic functionalities of NotificationService",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

const (
	defaultTimeout = 10
)

// NotificationServiceGRPC tests NotificationService functionalities.
func NotificationServiceGRPC(ctx context.Context, s *testing.State) {
	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Start Chrome on DUT
	cs := pb.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, &pb.NewRequest{}, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(ctx, &empty.Empty{})

	tconnSvc := pb.NewTconnServiceClient(cl.Conn)
	notificationSvc := pb.NewNotificationServiceClient(cl.Conn)

	// Close all notifications.
	if _, err := notificationSvc.CloseNotifications(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close notications: ", err)
	}

	// Ensure that there is no notifications to begin with.
	notificationsRes, err := notificationSvc.Notifications(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed when getting notications: ", err)
	}
	if len(notificationsRes.Notifications) > 0 {
		s.Fatal("There should be no notifications")
	}

	// Create sample notifications of different types.
	notificationObjs := []ash.Notification{
		{
			Type:    ash.NotificationTypeBasic,
			Title:   "basic",
			Message: "basic message",
			IconURL: ash.DefaultIconURL,
		},
		{
			Type:     ash.NotificationTypeImage,
			Title:    "image",
			Message:  "image message",
			ImageURL: ash.DefaultImageURL,
			IconURL:  ash.DefaultIconURL,
		},
		{
			Type:    ash.NotificationTypeList,
			Title:   "list",
			Message: "list message",
			IconURL: ash.DefaultIconURL,
			Items:   []ash.NotificationItem{{Message: "item1", Title: "title1"}, {Message: "item2", Title: "title2"}},
		},
	}
	for _, n := range notificationObjs {
		if err := createNotification(ctx, tconnSvc, n); err != nil {
			s.Fatalf("Failed to create basic notification %v: %v", n, err)
		}
	}

	// Wait for "List" type notification and verify notification contents.
	waitRequest := &pb.WaitForNotificationRequest{
		Predicates: []*pb.WaitPredicate{
			{Value: &pb.WaitPredicate_Title{Title: "list"}},
			{Value: &pb.WaitPredicate_TitleContains{TitleContains: "ist"}},
			{Value: &pb.WaitPredicate_MessageContains{MessageContains: "ist message"}},
		},
		TimeoutSecs: defaultTimeout,
	}
	waitForNotificationRes, err := notificationSvc.WaitForNotification(ctx, waitRequest)
	if err != nil {
		s.Fatal("Failed to wait for List type notication: ", err)
	}
	if waitForNotificationRes.Notification.Message != "list message" {
		s.Fatal("Incorrect message in List type notication: ", waitForNotificationRes)
	}

	// Verify the number of notifications.
	notificationsRes, err = notificationSvc.Notifications(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get Notications: ", err)
	}
	if len(notificationsRes.Notifications) != len(notificationObjs) {
		s.Fatalf("Incorrect number of notifications. Want: %d  Got: %d ", len(notificationObjs), len(notificationsRes.Notifications))
	}

	// Close all notifications.
	if _, err := notificationSvc.CloseNotifications(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close notications: ", err)
	}

	// Ensure that all notification are closed.
	// The conditions should match against all the test notifications.
	waitGoneRequest := &pb.WaitUntilNotificationGoneRequest{
		Predicates: []*pb.WaitPredicate{
			{Value: &pb.WaitPredicate_TitleDoesNotContain{TitleDoesNotContain: "NotMatched"}},
			{Value: &pb.WaitPredicate_MessageContains{MessageContains: "message"}},
			{Value: &pb.WaitPredicate_IdContains{IdContains: "notification-ui-manager"}},
		},
		TimeoutSecs: defaultTimeout,
	}
	if _, err := notificationSvc.WaitUntilNotificationGone(ctx, waitGoneRequest); err != nil {
		s.Fatal("There should be no notifications: ", err)
	}
}

// createNotification creates notifications in system tray through test connection.
func createNotification(ctx context.Context, svc pb.TconnServiceClient,
	notificationObj ash.Notification) error {
	b, err := json.Marshal(notificationObj)
	if err != nil {
		return err
	}
	jsonstring := string(b)

	fn := fmt.Sprintf(`async () =>
	  tast.promisify(chrome.notifications.create)(%s)`, jsonstring)
	_, err = svc.Call(ctx, &pb.CallRequest{Fn: fn, Args: []*structpb.Value{}})
	return err
}
