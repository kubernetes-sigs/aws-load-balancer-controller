package alb_tests

import (
	"context"

	"sigs.k8s.io/aws-load-balancer-controller/v3/test/e2e/gateway/test_resources"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
)

var tf *framework.Framework

// InitTF initializes the package-level framework. Idempotent so a parent test binary
// that blank-imports multiple gateway e2e packages can call it once per package from a
// consolidated BeforeSuite without racing framework state.
func InitTF() error {
	if tf != nil {
		return nil
	}
	var err error
	tf, err = test_resources.InitGatewayFramework(context.Background())
	return err
}
