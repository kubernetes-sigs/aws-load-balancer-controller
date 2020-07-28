package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/appmesh"
	"github.com/aws/aws-sdk-go/service/appmesh/appmeshiface"
)

type AppMesh interface {
	appmeshiface.AppMeshAPI
}

// NewAppMesh constructs new AppMesh implementation.
func NewAppMesh(session *session.Session) AppMesh {
	return &defaultAppMesh{
		AppMeshAPI: appmesh.New(session),
	}
}

type defaultAppMesh struct {
	appmeshiface.AppMeshAPI
}
