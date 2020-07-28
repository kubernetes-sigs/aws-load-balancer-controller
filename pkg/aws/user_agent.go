package aws

import (
	"fmt"
	"github.com/aws/aws-app-mesh-controller-for-k8s/pkg/version"
	"github.com/aws/aws-sdk-go/aws/request"
)

const appName = "appmesh.k8s.aws"

// injectUserAgent will inject app specific user-agent into awsSDK
func injectUserAgent(handlers *request.Handlers) {
	handlers.Build.PushFrontNamed(request.NamedHandler{
		Name: fmt.Sprintf("%s/user-agent", appName),
		Fn:   request.MakeAddToUserAgentHandler(appName, version.GitVersion),
	})
}
