package ingress

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
)

const (
	oidcDiscoveryTimeout         = 10 * time.Second
	oidcDiscoveryMaxResponseSize = 1 << 20 // 1 MiB
	oidcDiscoveryPath            = "/.well-known/openid-configuration"
)

// oidcDiscoveryDocument holds the fields we care about from the OIDC discovery document
// served at {issuer}/.well-known/openid-configuration.
type oidcDiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

const (
	defaultAuthType                     = AuthTypeNone
	defaultAuthScope                    = "openid"
	defaultAuthSessionCookieName        = "AWSELBAuthSessionCookie"
	defaultAuthSessionTimeout           = 604800
	defaultAuthOnUnauthenticatedRequest = "authenticate"
)

// Auth config for Service / Ingresses
type AuthConfig struct {
	Type                     AuthType
	IDPConfigCognito         *AuthIDPConfigCognito
	IDPConfigOIDC            *AuthIDPConfigOIDC
	OnUnauthenticatedRequest string
	Scope                    string
	SessionCookieName        string
	SessionTimeout           int64
}

// AuthConfig builder can build auth configuration for service or ingresses.
type AuthConfigBuilder interface {
	Build(ctx context.Context, svcAndIngAnnotations map[string]string) (AuthConfig, error)
}

// NewDefaultAuthConfigBuilder constructs new defaultAuthConfigBuilder.
func NewDefaultAuthConfigBuilder(annotationParser annotations.Parser) *defaultAuthConfigBuilder {
	return &defaultAuthConfigBuilder{
		annotationParser: annotationParser,
	}
}

var _ AuthConfigBuilder = &defaultAuthConfigBuilder{}

// default implementation for AuthConfigBuilder
type defaultAuthConfigBuilder struct {
	annotationParser annotations.Parser
	// httpClient is used to fetch OIDC discovery documents. Defaults to a client with a timeout when nil.
	httpClient *http.Client
}

func (b *defaultAuthConfigBuilder) getHTTPClient() *http.Client {
	if b.httpClient != nil {
		return b.httpClient
	}
	return &http.Client{Timeout: oidcDiscoveryTimeout}
}

func (b *defaultAuthConfigBuilder) Build(ctx context.Context, svcAndIngAnnotations map[string]string) (AuthConfig, error) {
	authType, err := b.buildAuthType(ctx, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authOnUnauthenticatedRequest := b.buildAuthOnUnauthenticatedRequest(ctx, svcAndIngAnnotations)
	authScope := b.buildAuthScope(ctx, svcAndIngAnnotations)
	authSessionCookieName := b.buildAuthSessionCookieName(ctx, svcAndIngAnnotations)
	authSessionTimeout, err := b.buildAuthSessionTimeout(ctx, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authIDPCognito, err := b.buildAuthIDPConfigCognito(ctx, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}
	authIDPOIDC, err := b.buildAuthIDPConfigOIDC(ctx, svcAndIngAnnotations)
	if err != nil {
		return AuthConfig{}, err
	}

	authConfig := AuthConfig{
		Type:                     authType,
		OnUnauthenticatedRequest: authOnUnauthenticatedRequest,
		Scope:                    authScope,
		SessionCookieName:        authSessionCookieName,
		SessionTimeout:           authSessionTimeout,
		IDPConfigOIDC:            authIDPOIDC,
		IDPConfigCognito:         authIDPCognito,
	}

	return authConfig, nil
}

func (b *defaultAuthConfigBuilder) buildAuthType(_ context.Context, svcAndIngAnnotations map[string]string) (AuthType, error) {
	rawAuthType := string(defaultAuthType)
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthType, &rawAuthType, svcAndIngAnnotations)
	switch rawAuthType {
	case string(AuthTypeCognito):
		return AuthTypeCognito, nil
	case string(AuthTypeOIDC):
		return AuthTypeOIDC, nil
	case string(AuthTypeNone):
		return AuthTypeNone, nil
	default:
		return "", errors.Errorf("unknown authType: %v", rawAuthType)
	}
}

func (b *defaultAuthConfigBuilder) buildAuthIDPConfigCognito(_ context.Context, svcAndIngAnnotations map[string]string) (*AuthIDPConfigCognito, error) {
	authIDP := AuthIDPConfigCognito{}
	exists, err := b.annotationParser.ParseJSONAnnotation(annotations.IngressSuffixAuthIDPCognito, &authIDP, svcAndIngAnnotations)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return &authIDP, nil
}

func (b *defaultAuthConfigBuilder) buildAuthIDPConfigOIDC(ctx context.Context, svcAndIngAnnotations map[string]string) (*AuthIDPConfigOIDC, error) {
	authIDP := AuthIDPConfigOIDC{}
	exists, err := b.annotationParser.ParseJSONAnnotation(annotations.IngressSuffixAuthIDPOIDC, &authIDP, svcAndIngAnnotations)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	// If a discoveryEndpoint is provided, fetch the OIDC discovery document and
	// populate the four endpoint fields automatically.
	if authIDP.DiscoveryEndpoint != "" {
		if err := b.populateOIDCEndpointsFromDiscovery(ctx, &authIDP); err != nil {
			return nil, err
		}
	}
	return &authIDP, nil
}

// populateOIDCEndpointsFromDiscovery fetches {discoveryEndpoint}/.well-known/openid-configuration
// and fills in Issuer, AuthorizationEndpoint, TokenEndpoint, and UserInfoEndpoint on cfg.
// Any field that was already explicitly set by the user is left unchanged.
func (b *defaultAuthConfigBuilder) populateOIDCEndpointsFromDiscovery(ctx context.Context, cfg *AuthIDPConfigOIDC) error {
	if !strings.HasPrefix(cfg.DiscoveryEndpoint, "https://") {
		return errors.Errorf("discoveryEndpoint must use https: %s", cfg.DiscoveryEndpoint)
	}
	discoveryURL := strings.TrimRight(cfg.DiscoveryEndpoint, "/") + oidcDiscoveryPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to build OIDC discovery request for %s", discoveryURL)
	}

	resp, err := b.getHTTPClient().Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch OIDC discovery document from %s", discoveryURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("OIDC discovery endpoint %s returned status %d", discoveryURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, oidcDiscoveryMaxResponseSize))
	if err != nil {
		return errors.Wrapf(err, "failed to read OIDC discovery response body from %s", discoveryURL)
	}

	var doc oidcDiscoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return errors.Wrapf(err, "failed to parse OIDC discovery document from %s", discoveryURL)
	}

	// Only overwrite fields that the user has NOT already explicitly set.
	if cfg.Issuer == "" {
		cfg.Issuer = doc.Issuer
	}
	if cfg.AuthorizationEndpoint == "" {
		cfg.AuthorizationEndpoint = doc.AuthorizationEndpoint
	}
	if cfg.TokenEndpoint == "" {
		cfg.TokenEndpoint = doc.TokenEndpoint
	}
	if cfg.UserInfoEndpoint == "" {
		cfg.UserInfoEndpoint = doc.UserInfoEndpoint
	}

	// Validate that all required fields are now populated.
	if cfg.Issuer == "" {
		return errors.Errorf("OIDC discovery document from %s is missing 'issuer'", discoveryURL)
	}
	if cfg.AuthorizationEndpoint == "" {
		return errors.Errorf("OIDC discovery document from %s is missing 'authorization_endpoint'", discoveryURL)
	}
	if cfg.TokenEndpoint == "" {
		return errors.Errorf("OIDC discovery document from %s is missing 'token_endpoint'", discoveryURL)
	}
	if cfg.UserInfoEndpoint == "" {
		return errors.Errorf("OIDC discovery document from %s is missing 'userinfo_endpoint'", discoveryURL)
	}

	return nil
}

func (b *defaultAuthConfigBuilder) buildAuthOnUnauthenticatedRequest(_ context.Context, svcAndIngAnnotations map[string]string) string {
	rawOnUnauthenticatedRequest := defaultAuthOnUnauthenticatedRequest
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthOnUnauthenticatedRequest, &rawOnUnauthenticatedRequest, svcAndIngAnnotations)
	return rawOnUnauthenticatedRequest
}

func (b *defaultAuthConfigBuilder) buildAuthScope(_ context.Context, svcAndIngAnnotations map[string]string) string {
	rawAuthScope := defaultAuthScope
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthScope, &rawAuthScope, svcAndIngAnnotations)
	return rawAuthScope
}

func (b *defaultAuthConfigBuilder) buildAuthSessionCookieName(_ context.Context, svcAndIngAnnotations map[string]string) string {
	rawAuthSessionCookieName := defaultAuthSessionCookieName
	_ = b.annotationParser.ParseStringAnnotation(annotations.IngressSuffixAuthSessionCookie, &rawAuthSessionCookieName, svcAndIngAnnotations)
	return rawAuthSessionCookieName
}

func (b *defaultAuthConfigBuilder) buildAuthSessionTimeout(_ context.Context, svcAndIngAnnotations map[string]string) (int64, error) {
	rawAuthSessionTimeout := int64(defaultAuthSessionTimeout)
	if _, err := b.annotationParser.ParseInt64Annotation(annotations.IngressSuffixAuthSessionTimeout, &rawAuthSessionTimeout, svcAndIngAnnotations); err != nil {
		return 0, err
	}
	return rawAuthSessionTimeout, nil
}
