// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	commonash "go.chromium.org/tast-tests/cros/common/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterNotificationServiceServer(srv,
				&NotificationService{sharedObject: common.SharedObjectsForServiceSingleton})
		},
		GuaranteeCompatibility: true,
	})
}

// NotificationService implements tast.cros.ui.NotificationService.
type NotificationService struct {
	sharedObject *common.SharedObjectsForService
}

const defaultWaitForNotificationTimeout = 60

// Notifications returns an array of notifications in system tray.
func (svc *NotificationService) Notifications(ctx context.Context, req *empty.Empty) (*pb.NotificationsResponse, error) {
	return common.UseTconn(ctx, svc.sharedObject, func(tconn *chrome.TestConn) (*pb.NotificationsResponse, error) {
		notifications, err := ash.Notifications(ctx, tconn)
		notificationsPB := pb.NotificationsResponse{}
		if err != nil {
			return &notificationsPB, err
		}

		for _, n := range notifications {
			notificationsPB.Notifications = append(notificationsPB.Notifications, toNotificationPB(n))
		}
		return &notificationsPB, nil
	})
}

// CloseNotifications closes all notifications in system tray.
func (svc *NotificationService) CloseNotifications(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	return common.UseTconn(ctx, svc.sharedObject, func(tconn *chrome.TestConn) (*empty.Empty, error) {
		err := ash.CloseNotifications(ctx, tconn)
		return &empty.Empty{}, err
	})
}

// WaitForNotification waits for the first notification that satisfies all wait
// predicates.
func (svc *NotificationService) WaitForNotification(ctx context.Context, req *pb.WaitForNotificationRequest) (*pb.WaitForNotificationResponse, error) {
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = defaultWaitForNotificationTimeout
	}
	return common.UseTconn(ctx, svc.sharedObject, func(tconn *chrome.TestConn) (*pb.WaitForNotificationResponse, error) {
		var predicates []ash.WaitPredicate
		if len(req.Predicates) == 0 {
			return &pb.WaitForNotificationResponse{}, errors.New("no predicate was specified")
		}

		for _, p := range req.Predicates {
			predicate, err := toPredicate(p)
			if err != nil {
				return &pb.WaitForNotificationResponse{}, err
			}
			predicates = append(predicates, predicate)
		}

		notification, err := ash.WaitForNotification(ctx, tconn, time.Duration(req.TimeoutSecs)*time.Second, predicates...)
		if err != nil {
			return &pb.WaitForNotificationResponse{}, err
		}
		return &pb.WaitForNotificationResponse{Notification: toNotificationPB(notification)}, nil
	})
}

// WaitUntilNotificationGone waits for the notifications that satisfies all predicates to disappear.
func (svc *NotificationService) WaitUntilNotificationGone(ctx context.Context, req *pb.WaitUntilNotificationGoneRequest) (*empty.Empty, error) {
	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = defaultWaitForNotificationTimeout
	}
	return common.UseTconn(ctx, svc.sharedObject, func(tconn *chrome.TestConn) (*empty.Empty, error) {
		var predicates []ash.WaitPredicate
		if len(req.Predicates) == 0 {
			return &empty.Empty{}, errors.New("no predicate was specified")
		}

		for _, p := range req.Predicates {
			predicate, err := toPredicate(p)
			if err != nil {
				return &empty.Empty{}, err
			}
			predicates = append(predicates, predicate)
		}
		return &empty.Empty{}, ash.WaitUntilNotificationGone(ctx, tconn, time.Duration(req.TimeoutSecs)*time.Second, predicates...)
	})
}

func toNotificationPB(n *commonash.Notification) *pb.Notification {
	return &pb.Notification{
		Id:       n.ID,
		Type:     string(n.Type),
		Title:    n.Title,
		Message:  n.Message,
		Priority: uint32(n.Priority),
		Progress: uint32(n.Progress),
	}
}

func toPredicate(wp *pb.WaitPredicate) (ash.WaitPredicate, error) {
	switch val := wp.Value.(type) {
	case *pb.WaitPredicate_IdContains:
		return ash.WaitIDContains(val.IdContains), nil
	case *pb.WaitPredicate_Title:
		return ash.WaitTitle(val.Title), nil
	case *pb.WaitPredicate_TitleContains:
		return ash.WaitTitleContains(val.TitleContains), nil
	case *pb.WaitPredicate_TitleDoesNotContain:
		return ash.WaitTitleDoesntContain(val.TitleDoesNotContain), nil
	case *pb.WaitPredicate_MessageContains:
		return ash.WaitMessageContains(val.MessageContains), nil
	default:
		return nil, errors.Errorf("unknown predicate type: %v", val)
	}
}
