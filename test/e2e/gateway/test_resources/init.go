package test_resources

import (
	"context"
	"fmt"

	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/manifest"
)

// InitGatewayFramework wraps framework.InitFramework with gateway-specific per-partition wiring.
// Callers use this in package InitTF; sets TestHostname and, if --resolve-image-tags is on, rewrites image vars.
func InitGatewayFramework(ctx context.Context) (*framework.Framework, error) {
	tf, err := framework.InitFramework()
	if err != nil {
		return nil, err
	}
	SetTestHostnameForRegion(tf.Options.AWSRegion)
	if tf.Options.ResolveImageTags {
		r, err := manifest.NewImageResolver(ctx, tf.Options.TestImageRegistry, tf.Logger)
		if err != nil {
			return nil, fmt.Errorf("init image resolver: %w", err)
		}
		if err := r.ResolveTestImages(ctx); err != nil {
			return nil, fmt.Errorf("resolve test image tags: %w", err)
		}
	}
	return tf, nil
}
