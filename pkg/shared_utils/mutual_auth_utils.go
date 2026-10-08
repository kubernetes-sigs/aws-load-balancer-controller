package shared_utils

import (
	"context"
	"slices"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/pkg/errors"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
)

type MutualAuthenticationConfig struct {
	Port                          int32   `json:"port"`
	Mode                          string  `json:"mode"`
	TrustStore                    *string `json:"trustStore,omitempty"`
	IgnoreClientCertificateExpiry *bool   `json:"ignoreClientCertificateExpiry,omitempty"`
	AdvertiseTrustStoreCaNames    *string `json:"advertiseTrustStoreCaNames,omitempty"`
}

// ParseMtlsConfigEntries returns a map from listener ports to MutualAuthenticationAttributes
func ParseMtlsConfigEntries(ctx context.Context, elbv2Client services.ELBV2, entries []MutualAuthenticationConfig, protocol elbv2model.Protocol) (map[int32]*elbv2model.MutualAuthenticationAttributes, error) {
	portAndMtlsAttributes := make(map[int32]*elbv2model.MutualAuthenticationAttributes, len(entries))

	for _, mutualAuthenticationConfig := range entries {
		port := mutualAuthenticationConfig.Port
		mode := mutualAuthenticationConfig.Mode
		truststoreNameOrArn := awssdk.ToString(mutualAuthenticationConfig.TrustStore)
		ignoreClientCert := mutualAuthenticationConfig.IgnoreClientCertificateExpiry
		advertiseTrustStoreCaNames := mutualAuthenticationConfig.AdvertiseTrustStoreCaNames

		err := validateMutualAuthenticationConfig(protocol, port, mode, truststoreNameOrArn, ignoreClientCert, advertiseTrustStoreCaNames)
		if err != nil {
			return nil, err
		}

		if mode == string(elbv2model.MutualAuthenticationVerifyMode) && ignoreClientCert == nil {
			ignoreClientCert = awssdk.Bool(false)
		}
		portAndMtlsAttributes[port] = &elbv2model.MutualAuthenticationAttributes{Mode: mode, TrustStoreArn: awssdk.String(truststoreNameOrArn), IgnoreClientCertificateExpiry: ignoreClientCert, AdvertiseTrustStoreCaNames: advertiseTrustStoreCaNames}
	}

	return parseMtlsAttributesForTrustStoreNames(ctx, elbv2Client, portAndMtlsAttributes)
}

func validateMutualAuthenticationConfig(protocol elbv2model.Protocol, port int32, mode string, truststoreNameOrArn string, ignoreClientCert *bool, advertiseTrustStoreCaNames *string) error {
	// Verify that port is in range 1-65535, inclusive
	if port < 1 || port > 65535 {
		return errors.Errorf("listen port must be within [1, 65535]: %v", port)
	}

	// Verify that mutualAuthentication mode is not empty for a port
	if mode == "" {
		return errors.Errorf("mutualAuthentication mode cannot be empty for port %v", port)
	}

	// Verify that mutualAuthentication mode is valid for the given listener protocol
	var validModes []string
	if protocol == elbv2model.ProtocolHTTPS {
		validModes = []string{string(elbv2model.MutualAuthenticationOffMode), string(elbv2model.MutualAuthenticationPassthroughMode), string(elbv2model.MutualAuthenticationVerifyMode)}
	}
	if !slices.Contains(validModes, mode) {
		return errors.Errorf("mutualAuthentication mode value must be among [%v] for protocol %v port %v : %s", strings.Join(validModes, ", "), protocol, port, mode)
	}

	// Verify if the mutualAuthentication truststoreNameOrArn is not empty for Verify mode
	if mode == string(elbv2model.MutualAuthenticationVerifyMode) && truststoreNameOrArn == "" {
		return errors.Errorf("trustStore is required when mutualAuthentication mode is verify for port %v", port)
	}

	// Verify if the mutualAuthentication truststoreNameOrArn is empty for Off and Passthrough modes
	if (mode == string(elbv2model.MutualAuthenticationOffMode) || mode == string(elbv2model.MutualAuthenticationPassthroughMode)) && truststoreNameOrArn != "" {
		return errors.Errorf("Mutual Authentication mode %s does not support trustStore for port %v", mode, port)
	}

	// Verify if the mutualAuthentication ignoreClientCert is valid for Off and Passthrough modes
	if (mode == string(elbv2model.MutualAuthenticationOffMode) || mode == string(elbv2model.MutualAuthenticationPassthroughMode)) && ignoreClientCert != nil {
		return errors.Errorf("Mutual Authentication mode %s does not support ignoring client certificate expiry for port %v", mode, port)
	}

	// Verify advertise trust ca names.
	// The value (if specified) must be "on" or "off"
	// The value can be only specified when using verify mode on the listener.
	if advertiseTrustStoreCaNames != nil {
		if mode != string(elbv2model.MutualAuthenticationVerifyMode) {
			return errors.Errorf("Mutual Authentication mode %s does not support advertiseTrustStoreCaNames for port %v", mode, port)
		}

		if *advertiseTrustStoreCaNames != string(elbv2types.AdvertiseTrustStoreCaNamesEnumOff) && *advertiseTrustStoreCaNames != string(elbv2types.AdvertiseTrustStoreCaNamesEnumOn) {
			return errors.Errorf("advertiseTrustStoreCaNames only supports the values \"on\" and \"off\" got value %s for port %v", *advertiseTrustStoreCaNames, port)
		}
	}

	return nil
}

// parseMtlsAttributesForTrustStoreNames replaces trust store names with ARNs
func parseMtlsAttributesForTrustStoreNames(ctx context.Context, elbv2Client services.ELBV2, portAndMtlsAttributes map[int32]*elbv2model.MutualAuthenticationAttributes) (map[int32]*elbv2model.MutualAuthenticationAttributes, error) {
	var trustStoreNames []string
	trustStoreNameAndPortMap := make(map[string][]int32)

	for port, attributes := range portAndMtlsAttributes {
		mode := attributes.Mode
		truststoreNameOrArn := awssdk.ToString(attributes.TrustStoreArn)
		if mode == string(elbv2model.MutualAuthenticationVerifyMode) && !strings.HasPrefix(truststoreNameOrArn, "arn:") {
			trustStoreNameAndPortMap[truststoreNameOrArn] = append(trustStoreNameAndPortMap[truststoreNameOrArn], port)
		}
	}

	if len(trustStoreNameAndPortMap) != 0 {
		for names := range trustStoreNameAndPortMap {
			trustStoreNames = append(trustStoreNames, names)
		}
		tsNameAndArnMap, err := GetTrustStoreArnFromName(ctx, elbv2Client, trustStoreNames)
		if err != nil {
			return nil, err
		}
		for name, ports := range trustStoreNameAndPortMap {
			for _, port := range ports {
				attributes := portAndMtlsAttributes[port]
				if awssdk.ToString(attributes.TrustStoreArn) != "" {
					attributes.TrustStoreArn = tsNameAndArnMap[name]
				}
				portAndMtlsAttributes[port] = attributes
			}
		}
	}
	return portAndMtlsAttributes, nil
}
