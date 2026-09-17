package manifest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/go-logr/logr"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/utils"
)

const testImageRepoPrefix = "networking-e2e-test-images"

// Multi-arch manifest media types; single-arch tags are skipped to avoid cross-arch scheduling failures.
var imageIndexMediaTypes = map[string]struct{}{
	"application/vnd.docker.distribution.manifest.list.v2+json": {},
	"application/vnd.oci.image.index.v1+json":                   {},
}

// Tags whose segments start with any of these are excluded from selection.
var skipTagMarkers = []string{"latest", "rc", "dev", "alpha", "beta", "snapshot", "preview", "nightly"}

// ImageResolver looks up the newest multi-arch test image tag in ECR and reassigns the utils image vars.
// Rationale: replication tooling to non-ADC regional ECRs does not carry the ":latest" tag.
type ImageResolver struct {
	client     *ecr.Client
	logger     logr.Logger
	registryID string
}

// NewImageResolver parses region/accountID out of the registry URI and returns a resolver bound to that ECR.
func NewImageResolver(ctx context.Context, registry string, logger logr.Logger) (*ImageResolver, error) {
	registryID, region := parseEcrURI(registry)
	if region == "" {
		return nil, fmt.Errorf("cannot infer region from registry %q", registry)
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &ImageResolver{
		client:     ecr.NewFromConfig(cfg),
		logger:     logger,
		registryID: registryID,
	}, nil
}

// ResolveTestImages reassigns utils.HelloImage/ColortellerImage/UDPImage/GRPCImage to concrete versioned tags.
func (r *ImageResolver) ResolveTestImages(ctx context.Context) error {
	dst := map[string]*string{
		"hello-multi":     &utils.HelloImage,
		"colorteller":     &utils.ColortellerImage,
		"udp-echoserver":  &utils.UDPImage,
		"grpc-echoserver": &utils.GRPCImage,
	}
	for name, p := range dst {
		v, err := r.resolve(ctx, name)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", name, err)
		}
		r.logger.Info("resolved test image tag from ECR", "image", name, "value", v)
		*p = v
	}
	return nil
}

// resolve returns "<testImageRepoPrefix>/<imageName>:<tag>" (relative; caller prepends registry via GetDeploymentImage).
func (r *ImageResolver) resolve(ctx context.Context, imageName string) (string, error) {
	tag, err := r.latestImageTag(ctx, imageName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s:%s", testImageRepoPrefix, imageName, tag), nil
}

func (r *ImageResolver) latestImageTag(ctx context.Context, imageName string) (string, error) {
	repository := fmt.Sprintf("%s/%s", testImageRepoPrefix, imageName)
	input := &ecr.DescribeImagesInput{RepositoryName: aws.String(repository)}
	if r.registryID != "" {
		input.RegistryId = aws.String(r.registryID)
	}
	var latestTag string
	var latestPushedAt *time.Time
	paginator := ecr.NewDescribeImagesPaginator(r.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("describe images in ECR repository %q: %w", repository, err)
		}
		for _, img := range page.ImageDetails {
			if img.ImagePushedAt == nil {
				continue
			}
			if !isImageIndex(aws.ToString(img.ImageManifestMediaType)) {
				continue
			}
			tag := preferredTag(img.ImageTags)
			if tag == "" {
				continue
			}
			if latestPushedAt == nil || img.ImagePushedAt.After(*latestPushedAt) {
				latestPushedAt = img.ImagePushedAt
				latestTag = tag
			}
		}
	}
	if latestTag == "" {
		return "", fmt.Errorf("no versioned multi-arch image tag found in ECR repository %q", repository)
	}
	return latestTag, nil
}

// preferredTag returns the first tag whose segments (split on '-', '.', '_') don't start with a skip marker.
func preferredTag(tags []string) string {
nextTag:
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		segments := strings.FieldsFunc(strings.ToLower(tag), func(r rune) bool {
			return r == '-' || r == '.' || r == '_'
		})
		for _, seg := range segments {
			for _, marker := range skipTagMarkers {
				if strings.HasPrefix(seg, marker) {
					continue nextTag
				}
			}
		}
		return tag
	}
	return ""
}

func isImageIndex(mediaType string) bool {
	_, ok := imageIndexMediaTypes[mediaType]
	return ok
}

// parseEcrURI extracts (accountID, region) from "<accountId>.dkr.ecr.<region>.<dnsSuffix>"; empties on mismatch.
func parseEcrURI(uri string) (registryID string, region string) {
	parts := strings.Split(uri, ".")
	if len(parts) >= 4 && parts[1] == "dkr" && parts[2] == "ecr" {
		return parts[0], parts[3]
	}
	return "", ""
}
