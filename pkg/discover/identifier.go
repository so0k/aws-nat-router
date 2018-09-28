package discover

import (
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
)

// Identifier interface helps identify instance Id
type Identifier interface {
	// GetIdentity returns the identity of
	GetIdentity() (string, error)
}

// ref - https://github.com/stylight/etcd-bootstrap

// EC2MetadataIface is an interface for AWS EC2 Metadata service for Unit Test mocking
type EC2MetadataIface interface {
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
	Available() bool
}

// NewAwsIdentifierFromSession returns Identifier for ec2Metadata service using a session object
func NewAwsIdentifierFromSession(s *session.Session) (Identifier, error) {
	return NewAwsIdentifier(ec2metadata.New(s))
}

// NewAwsIdentifier returns Identifier for ec2Metadata service
func NewAwsIdentifier(svc EC2MetadataIface) (Identifier, error) {
	return &AwsIdentifier{
		service: svc,
	}, nil
}

// AwsIdentifier Identifier for ec2Metadata service
type AwsIdentifier struct {
	service EC2MetadataIface
}

// GetIdentity gets the curernt InstanceID
func (i AwsIdentifier) GetIdentity() (string, error) {
	if !i.service.Available() {
		return "", errors.New("Metadata is not available")
	}

	d, e := i.service.GetInstanceIdentityDocument()
	if e != nil {
		return "", errors.Wrap(e, "Unable to retrieve Instance Identity")
	}
	return d.InstanceID, nil
}
