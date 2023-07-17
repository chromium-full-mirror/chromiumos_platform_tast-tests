// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"os"
	"path/filepath"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/certificate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	pb "go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterCertificateServiceServer(srv, &CertService{
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
	})
}

// CertService implements tast.cros.network.CertificateServiceServer gRPC service.
type CertService struct {
	sharedObject *common.SharedObjectsForService
	manager      *certificate.Manager
}

// Init initializes the certificate service by creating the essential resources.
// Caller must call the Init function before calling any other interfaces.
func (sv *CertService) Init(ctx context.Context, req *pb.InitRequest) (*emptypb.Empty, error) {
	return common.UseTconn(ctx, sv.sharedObject, func(tconn *chrome.TestConn) (_ *emptypb.Empty, retErr error) {
		cr := sv.sharedObject.Chrome
		if cr == nil {
			return &emptypb.Empty{}, errors.New("Chrome has not been started")
		}

		br, err := sv.sharedObject.Browser(req.IsLacros)
		if err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to get browser from shared object")
		}

		switch val := req.InitType; val {
		case pb.InitRequest_LAUNCH:
			if sv.manager != nil {
				return &emptypb.Empty{}, errors.New("the Certificates Manager is already launched")
			}

			if sv.manager, err = certificate.Launch(ctx, tconn, br); err != nil {
				return &emptypb.Empty{}, errors.Wrap(err, "failed to launch the Certificates Manager")
			}
		case pb.InitRequest_CONNECT:
			if sv.manager == nil {
				return &emptypb.Empty{}, errors.New("Certificates Manager hasn't been launched")
			}

			if err := sv.manager.Connect(ctx, tconn, br); err != nil {
				return &emptypb.Empty{}, errors.Wrap(err, "failed to connect to the Certificates Manager")
			}
		default:
			return &emptypb.Empty{}, errors.Errorf("unsupported initialize type %v", val)
		}

		return &emptypb.Empty{}, nil
	})
}

// Close cleans up the resources created by the service.
func (sv *CertService) Close(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if sv.manager != nil {
		if err := sv.manager.Close(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to close the Certificates Manager")
		}
		sv.manager = nil
	}

	return &emptypb.Empty{}, nil
}

// ImportCert imports the specified certificate through the Certificates Manager.
func (sv *CertService) ImportCert(ctx context.Context, req *pb.ImportRequest) (*emptypb.Empty, error) {
	cr := sv.sharedObject.Chrome
	if cr == nil {
		return &emptypb.Empty{}, errors.New("Chrome has not been started")
	}
	if sv.manager == nil {
		return &emptypb.Empty{}, errors.New("Certificates Manager hasn't been launched")
	}

	downloadPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to get the path to the 'Downloads' folder")
	}

	certFilePath := req.GetFilePath()
	certFileName := filepath.Base(certFilePath)
	// Ensure the file is located under 'Downloads' or the file is not accessible to user.
	if filepath.Dir(certFilePath) != downloadPath {
		targetFilePath := filepath.Join(downloadPath, certFileName)
		if err := fsutil.CopyFile(certFilePath, targetFilePath); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to copy the certificate file to the 'Downloads' folder")
		}
		defer os.Remove(targetFilePath)
	}

	var importFunc uiauto.Action
	switch val := req.ImportDetail.(type) {
	case *pb.ImportRequest_Client:
		name, password, org, importType, err := parseClientCertImportRequest(req.GetCertificate(), val.Client)
		if err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to parse the client certificate import request")
		}
		importFunc = sv.manager.ImportClientCert(certFileName, password, name, org, importType)
	case *pb.ImportRequest_Ca:
		org, trustSettings := parseCaCertImportRequest(req.GetCertificate(), val.Ca)
		importFunc = sv.manager.ImportCACert(certFileName, org, trustSettings)
	default:
		return &emptypb.Empty{}, errors.Errorf("unsupported certificate type %v", val)
	}

	return &emptypb.Empty{}, importFunc(ctx)
}

// DeleteCert deletes the specified certificate from the Certificates Manager.
func (sv *CertService) DeleteCert(ctx context.Context, cert *pb.Certificate) (*emptypb.Empty, error) {
	if sv.manager == nil {
		return &emptypb.Empty{}, errors.New("Certificates Manager hasn't been launched")
	}

	certType, err := parseCertType(cert)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse the certificate type")
	}

	return &emptypb.Empty{}, sv.manager.DeleteCert(cert.GetName(), certificate.Organization{Name: cert.Organization}, certType)(ctx)
}

// IsCertImported returns whether a specified certificate has been installed in the Certificates Manager.
func (sv *CertService) IsCertImported(ctx context.Context, cert *pb.Certificate) (*pb.IsCertImportedResponse, error) {
	if sv.manager == nil {
		return nil, errors.New("Certificates Manager hasn't been launched")
	}

	certType, err := parseCertType(cert)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse the certificate type")
	}

	isImported, err := sv.manager.IsCertImported(ctx, cert.GetName(), certificate.Organization{Name: cert.Organization}, certType)
	if err != nil {
		return &pb.IsCertImportedResponse{}, errors.Wrap(err, "failed to get certificate existing state")
	}
	return &pb.IsCertImportedResponse{IsImported: isImported}, nil
}

// parseClientCertImportRequest parses the import request of client certificate.
func parseClientCertImportRequest(cert *pb.Certificate, detail *pb.ImportRequest_ClientImportDetail) (name, password string, org certificate.Organization, importType certificate.ImportType, retErr error) {
	switch detail.ImportType {
	case pb.ImportRequest_ClientImportDetail_IMPORT:
		importType = certificate.TypeImport
	case pb.ImportRequest_ClientImportDetail_IMPORT_AND_BIND:
		importType = certificate.TypeImportAndBind
	default:
		retErr = errors.Errorf("unsupported client certificate import type %v", detail.ImportType)
	}
	return cert.GetName(), cert.GetPassword(), certificate.Organization{Name: cert.GetOrganization()}, importType, retErr
}

// parseCaCertImportRequest parses the import request of ca certificate.
func parseCaCertImportRequest(cert *pb.Certificate, detail *pb.ImportRequest_CaImportDetail) (org certificate.Organization, trustSettings certificate.CATrustSettings) {
	if detail.GetTrustWebsite() {
		trustSettings |= certificate.TrustForWebsites
	}
	if detail.GetTrustEmailUser() {
		trustSettings |= certificate.TrustForEmailUsers
	}
	if detail.GetTrustSoftwareMaker() {
		trustSettings |= certificate.TrustForSoftwareMakers
	}
	return certificate.Organization{Name: cert.GetOrganization()}, trustSettings
}

// parseCertType parses and returns the type of the certificate.
func parseCertType(cert *pb.Certificate) (certType certificate.CertType, err error) {
	switch cert.Type {
	case pb.Certificate_CLIENT:
		certType = certificate.TypeClient
	case pb.Certificate_CA:
		certType = certificate.TypeCA
	default:
		err = errors.Errorf("unsupported certificate type %v", cert.Type)
	}
	return certType, err
}
