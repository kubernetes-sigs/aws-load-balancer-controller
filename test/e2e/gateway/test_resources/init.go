package test_resources

import (
	"context"
	"fmt"

	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/manifest"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/utils"
)

// InitGatewayFramework initializes the framework, sets TestHostname, and selects the GRPC/UDP image source.
func InitGatewayFramework(ctx context.Context) (*framework.Framework, error) {
	tf, err := framework.InitFramework()
	if err != nil {
		return nil, err
	}
	SetTestHostnameForRegion(tf.Options.AWSRegion)
	// Default is public (constants.go); --resolve-image-tags switches to the mirror + resolver.
	if tf.Options.ResolveImageTags {
		utils.UDPImage = utils.MirrorUDPImage
		utils.GRPCImage = utils.MirrorGRPCImage
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
