package elbv2

import (
	"context"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	elbv2equality "sigs.k8s.io/aws-load-balancer-controller/pkg/equality/elbv2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/runtime"
)

var mTLSOff = &elbv2types.MutualAuthenticationAttributes{
	Mode: awssdk.String(string(elbv2model.MutualAuthenticationOffMode)),
}

var alpnNone = []string{
	string(elbv2model.ALPNPolicyNone),
}

var PROTOCOLS_SUPPORTING_LISTENER_ATTRIBUTES = map[elbv2model.Protocol]bool{
	elbv2model.ProtocolHTTP:    true,
	elbv2model.ProtocolHTTPS:   true,
	elbv2model.ProtocolTCP:     true,
	elbv2model.ProtocolUDP:     false,
	elbv2model.ProtocolTLS:     false,
	elbv2model.ProtocolTCP_UDP: true,
}

// ListenerManager is responsible for create/update/delete Listener resources.
type ListenerManager interface {
	Create(ctx context.Context, resLS *elbv2model.Listener) (elbv2model.ListenerStatus, error)

	Update(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) (elbv2model.ListenerStatus, error)

	Delete(ctx context.Context, sdkLS ListenerWithTags) error
}

func NewDefaultListenerManager(elbv2Client services.ELBV2, trackingProvider tracking.Provider,
	taggingManager TaggingManager, externalManagedTags []string, featureGates config.FeatureGates, enhancedDefaultingPolicyEnabled bool, logger logr.Logger) *defaultListenerManager {
	return &defaultListenerManager{
		elbv2Client:                     elbv2Client,
		trackingProvider:                trackingProvider,
		taggingManager:                  taggingManager,
		externalManagedTags:             externalManagedTags,
		featureGates:                    featureGates,
		logger:                          logger,
		enhancedDefaultingPolicyEnabled: enhancedDefaultingPolicyEnabled,
		waitLSExistencePollInterval:     defaultWaitLSExistencePollInterval,
		waitLSExistenceTimeout:          defaultWaitLSExistenceTimeout,
		attributesReconciler:            NewDefaultListenerAttributesReconciler(elbv2Client, logger),
	}
}

var _ ListenerManager = &defaultListenerManager{}

// default implementation for ListenerManager
type defaultListenerManager struct {
	elbv2Client                     services.ELBV2
	trackingProvider                tracking.Provider
	taggingManager                  TaggingManager
	externalManagedTags             []string
	featureGates                    config.FeatureGates
	logger                          logr.Logger
	enhancedDefaultingPolicyEnabled bool

	waitLSExistencePollInterval time.Duration
	waitLSExistenceTimeout      time.Duration
	attributesReconciler        ListenerAttributesReconciler
}

func (m *defaultListenerManager) Create(ctx context.Context, resLS *elbv2model.Listener) (elbv2model.ListenerStatus, error) {
	req, err := buildSDKCreateListenerInput(resLS.Spec, m.featureGates)
	if err != nil {
		return elbv2model.ListenerStatus{}, err
	}
	var lsTags map[string]string
	if m.featureGates.Enabled(config.ListenerRulesTagging) {
		lsTags = m.trackingProvider.ResourceTags(resLS.Stack(), resLS, resLS.Spec.Tags)
	}
	req.Tags = convertTagsToSDKTags(lsTags)

	m.logger.Info("creating listener",
		"stackID", resLS.Stack().StackID(),
		"resourceID", resLS.ID())
	resp, err := m.elbv2Client.CreateListenerWithContext(ctx, req)
	if err != nil {
		return elbv2model.ListenerStatus{}, err
	}
	sdkLS := ListenerWithTags{
		Listener: &resp.Listeners[0],
		Tags:     lsTags,
	}
	m.logger.Info("created listener",
		"stackID", resLS.Stack().StackID(),
		"resourceID", resLS.ID(),
		"arn", awssdk.ToString(sdkLS.Listener.ListenerArn))

	if err := runtime.RetryImmediateOnError(m.waitLSExistencePollInterval, m.waitLSExistenceTimeout, isListenerNotFoundError, func() error {
		return m.updateSDKListenerWithExtraCertificates(ctx, resLS, sdkLS, true)
	}); err != nil {
		return elbv2model.ListenerStatus{}, errors.Wrap(err, "failed to update extra certificates on listener")
	}
	listenerARN := awssdk.ToString(sdkLS.Listener.ListenerArn)
	if !isIsolatedRegion(getRegionFromARN(listenerARN)) && areListenerAttributesSupported(resLS.Spec.Protocol) {
		if err := m.attributesReconciler.Reconcile(ctx, resLS, sdkLS); err != nil {
			return elbv2model.ListenerStatus{}, err
		}
	}
	return buildResListenerStatus(sdkLS), nil
}

func (m *defaultListenerManager) Update(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) (elbv2model.ListenerStatus, error) {
	if m.featureGates.Enabled(config.ListenerRulesTagging) {
		if err := m.updateSDKListenerWithTags(ctx, resLS, sdkLS); err != nil {
			return elbv2model.ListenerStatus{}, err
		}
	}
	if err := m.updateSDKListenerWithSettings(ctx, resLS, sdkLS); err != nil {
		return elbv2model.ListenerStatus{}, err
	}
	if err := m.updateSDKListenerWithExtraCertificates(ctx, resLS, sdkLS, false); err != nil {
		return elbv2model.ListenerStatus{}, err
	}
	listenerARN := awssdk.ToString(sdkLS.Listener.ListenerArn)
	if !isIsolatedRegion(getRegionFromARN(listenerARN)) && areListenerAttributesSupported(resLS.Spec.Protocol) {
		if err := m.attributesReconciler.Reconcile(ctx, resLS, sdkLS); err != nil {
			return elbv2model.ListenerStatus{}, err
		}
	}
	return buildResListenerStatus(sdkLS), nil
}

func (m *defaultListenerManager) Delete(ctx context.Context, sdkLS ListenerWithTags) error {
	req := &elbv2sdk.DeleteListenerInput{
		ListenerArn: sdkLS.Listener.ListenerArn,
	}
	m.logger.Info("deleting listener",
		"arn", awssdk.ToString(req.ListenerArn))
	if _, err := m.elbv2Client.DeleteListenerWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("deleted listener",
		"arn", awssdk.ToString(req.ListenerArn))
	return nil
}

func (m *defaultListenerManager) updateSDKListenerWithTags(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) error {
	desiredLSTags := m.trackingProvider.ResourceTags(resLS.Stack(), resLS, resLS.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, awssdk.ToString(sdkLS.Listener.ListenerArn), desiredLSTags,
		WithCurrentTags(sdkLS.Tags),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func (m *defaultListenerManager) updateSDKListenerWithSettings(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) error {
	desiredDefaultActions, err := buildSDKActions(resLS.Spec.DefaultActions, m.featureGates)
	if err != nil {
		return err
	}
	desiredDefaultCerts, _ := buildSDKCertificates(resLS.Spec.Certificates)
	desiredDefaultMutualAuthentication := buildSDKMutualAuthenticationConfig(resLS.Spec.MutualAuthentication)
	if !m.isSDKListenerSettingsDrifted(resLS.Spec, sdkLS, desiredDefaultActions, desiredDefaultCerts, desiredDefaultMutualAuthentication) {
		return nil
	}
	var removeMutualAuth bool
	var removeALPN bool
	if m.enhancedDefaultingPolicyEnabled {
		removeMutualAuth = isRemoveMTLS(sdkLS, desiredDefaultMutualAuthentication)
		removeALPN = isRemoveALPN(sdkLS, resLS.Spec)
	}

	req := buildSDKModifyListenerInput(resLS.Spec, desiredDefaultActions, desiredDefaultCerts, removeMutualAuth, removeALPN)
	req.ListenerArn = sdkLS.Listener.ListenerArn
	m.logger.Info("modifying listener",
		"stackID", resLS.Stack().StackID(),
		"resourceID", resLS.ID(),
		"arn", awssdk.ToString(sdkLS.Listener.ListenerArn))
	if _, err := m.elbv2Client.ModifyListenerWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified listener",
		"stackID", resLS.Stack().StackID(),
		"resourceID", resLS.ID(),
		"arn", awssdk.ToString(sdkLS.Listener.ListenerArn))
	return nil
}

// updateSDKListenerWithExtraCertificates will update the extra certificates on listener.
// currentExtraCertificates is the current extra certificates, if it's nil, the current extra certificates will be fetched from AWS.
func (m *defaultListenerManager) updateSDKListenerWithExtraCertificates(ctx context.Context, resLS *elbv2model.Listener,
	sdkLS ListenerWithTags, isNewSDKListener bool) error {
	// if TLS is not supported, we shouldn't update
	if resLS.Spec.SSLPolicy == nil && sdkLS.Listener.SslPolicy == nil {
		m.logger.V(1).Info("Res and Sdk Listener don't have SSL Policy set, skip updating extra certs for non-TLS listener.")
		return nil
	}

	desiredExtraCertARNs := sets.NewString()
	_, desiredExtraCerts := buildSDKCertificates(resLS.Spec.Certificates)
	for _, cert := range desiredExtraCerts {
		desiredExtraCertARNs.Insert(awssdk.ToString(cert.CertificateArn))
	}
	currentExtraCertARNs := sets.NewString()
	if !isNewSDKListener {
		certARNs, err := m.fetchSDKListenerExtraCertificateARNs(ctx, sdkLS)
		if err != nil {
			return err
		}
		currentExtraCertARNs.Insert(certARNs...)
	}

	for _, certARN := range currentExtraCertARNs.Difference(desiredExtraCertARNs).List() {
		req := &elbv2sdk.RemoveListenerCertificatesInput{
			ListenerArn: sdkLS.Listener.ListenerArn,
			Certificates: []elbv2types.Certificate{
				{
					CertificateArn: awssdk.String(certARN),
				},
			},
		}
		m.logger.Info("removing certificate from listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.ToString(sdkLS.Listener.ListenerArn),
			"certificateARN", certARN)
		if _, err := m.elbv2Client.RemoveListenerCertificatesWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("removed certificate from listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.ToString(sdkLS.Listener.ListenerArn),
			"certificateARN", certARN)
	}

	for _, certARN := range desiredExtraCertARNs.Difference(currentExtraCertARNs).List() {
		req := &elbv2sdk.AddListenerCertificatesInput{
			ListenerArn: sdkLS.Listener.ListenerArn,
			Certificates: []elbv2types.Certificate{
				{
					CertificateArn: awssdk.String(certARN),
				},
			},
		}
		m.logger.Info("adding certificate to listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.ToString(sdkLS.Listener.ListenerArn),
			"certificateARN", certARN)
		if _, err := m.elbv2Client.AddListenerCertificatesWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("added certificate to listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.ToString(sdkLS.Listener.ListenerArn),
			"certificateARN", certARN)
	}

	return nil
}

func (m *defaultListenerManager) fetchSDKListenerExtraCertificateARNs(ctx context.Context, sdkLS ListenerWithTags) ([]string, error) {
	req := &elbv2sdk.DescribeListenerCertificatesInput{
		ListenerArn: sdkLS.Listener.ListenerArn,
	}
	sdkCerts, err := m.elbv2Client.DescribeListenerCertificatesAsList(ctx, req)
	if err != nil {
		return nil, err
	}
	extraCertARNs := make([]string, 0, len(sdkCerts))
	for _, cert := range sdkCerts {
		if !awssdk.ToBool(cert.IsDefault) {
			extraCertARNs = append(extraCertARNs, awssdk.ToString(cert.CertificateArn))
		}
	}
	return extraCertARNs, nil
}

func (m *defaultListenerManager) isSDKListenerSettingsDrifted(lsSpec elbv2model.ListenerSpec, sdkLS ListenerWithTags,
	desiredDefaultActions []elbv2types.Action, desiredDefaultCerts []elbv2types.Certificate, desiredDefaultMutualAuthentication *elbv2types.MutualAuthenticationAttributes) bool {
	if lsSpec.Port != awssdk.ToInt32(sdkLS.Listener.Port) {
		return true
	}
	if string(lsSpec.Protocol) != string(sdkLS.Listener.Protocol) {
		return true
	}
	if !cmp.Equal(desiredDefaultActions, sdkLS.Listener.DefaultActions, elbv2equality.CompareOptionForActions(desiredDefaultActions, sdkLS.Listener.DefaultActions)) {
		return true
	}
	if !cmp.Equal(desiredDefaultCerts, sdkLS.Listener.Certificates, elbv2equality.CompareOptionForCertificates()) {
		return true
	}
	if lsSpec.SSLPolicy != nil && awssdk.ToString(lsSpec.SSLPolicy) != awssdk.ToString(sdkLS.Listener.SslPolicy) {
		return true
	}

	if m.enhancedDefaultingPolicyEnabled {
		// Cases for ALPN
		// 1. desired = nil, sdk = nil. Result = no drift.
		// 2. desired = nil, sdk = None, Result = no drift
		// 3. desired = nil, sdk = Other value, Result = drift.
		// 4. desired = None, sdk = nil. Result = no drift.
		// 5. desired = None, sdk = None. Result = no drift
		// 6. desired = None, sdk = Other value. Result = drift
		// 7. desired = Other value, sdk = Nil. Result = drift
		// 8. desired = Other value, sdk = None. Result = drift
		// 9. desired = Other value, sdk = Other value. Result = drift depending on equality.

		desiredIsNilOrNone := lsSpec.ALPNPolicy == nil || len(lsSpec.ALPNPolicy) == 0 || lsSpec.ALPNPolicy[0] == string(elbv2model.ALPNPolicyNone)
		actualIsNilOrNone := sdkLS.Listener.AlpnPolicy == nil || len(sdkLS.Listener.AlpnPolicy) == 0 || sdkLS.Listener.AlpnPolicy[0] == string(elbv2model.ALPNPolicyNone)

		// case 3, case 6
		if desiredIsNilOrNone && !actualIsNilOrNone {
			return true
		}

		// case 7, case 8
		if actualIsNilOrNone && !desiredIsNilOrNone {
			return true
		}

		if !desiredIsNilOrNone && !actualIsNilOrNone {
			/// Case 9
			if !cmp.Equal(lsSpec.ALPNPolicy, sdkLS.Listener.AlpnPolicy, cmpopts.EquateEmpty()) {
				return true
			}
		}

	} else {
		if len(lsSpec.ALPNPolicy) != 0 && !cmp.Equal(lsSpec.ALPNPolicy, sdkLS.Listener.AlpnPolicy, cmpopts.EquateEmpty()) {
			return true
		}
	}

	if !m.enhancedDefaultingPolicyEnabled && desiredDefaultMutualAuthentication == nil {
		// Legacy behavior -- we ignored (sad) removing mutual auth
		return false
	}

	// Cases for mutual auth (mtls)
	// 1. desired = nil, sdk = nil. Result = no drift
	// 2. desired = nil, sdk = mtls off. result = no drift
	// 3. desired = nil, sdk = mtls on. result = drift
	// 4. desired = mtls off, sdk = nil result = no drift
	// 5. desired = mtls off, sdk = off, result = no drift
	// 6. desired = mtls on, sdk = nil. result = drift
	// 7. desired = mtls on, sdk = mtls on. result = compare values

	// case 1
	if desiredDefaultMutualAuthentication == nil && sdkLS.Listener.MutualAuthentication == nil {
		return false
	}

	if desiredDefaultMutualAuthentication == nil || desiredDefaultMutualAuthentication.Mode == nil {
		// case 2
		if sdkLS.Listener.MutualAuthentication.Mode == nil || *sdkLS.Listener.MutualAuthentication.Mode == string(elbv2model.MutualAuthenticationOffMode) {
			return false
		}

		// case 3
		return true
	}

	// case 4 & case 5
	if *desiredDefaultMutualAuthentication.Mode == string(elbv2model.MutualAuthenticationOffMode) {
		return !(sdkLS.Listener.MutualAuthentication == nil || sdkLS.Listener.MutualAuthentication.Mode == nil || *sdkLS.Listener.MutualAuthentication.Mode == string(elbv2model.MutualAuthenticationOffMode))
	}

	// case 6 and case 7
	return !cmp.Equal(desiredDefaultMutualAuthentication, sdkLS.Listener.MutualAuthentication, elbv2equality.CompareOptionsForMTLS())
}

func buildSDKCreateListenerInput(lsSpec elbv2model.ListenerSpec, featureGates config.FeatureGates) (*elbv2sdk.CreateListenerInput, error) {
	ctx := context.Background()
	lbARN, err := lsSpec.LoadBalancerARN.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	sdkObj := &elbv2sdk.CreateListenerInput{}
	sdkObj.LoadBalancerArn = awssdk.String(lbARN)
	sdkObj.Port = awssdk.Int32(lsSpec.Port)
	sdkObj.Protocol = elbv2types.ProtocolEnum(lsSpec.Protocol)
	defaultActions, err := buildSDKActions(lsSpec.DefaultActions, featureGates)
	if err != nil {
		return nil, err
	}
	sdkObj.DefaultActions = defaultActions
	sdkObj.Certificates, _ = buildSDKCertificates(lsSpec.Certificates)
	sdkObj.SslPolicy = lsSpec.SSLPolicy
	if len(lsSpec.ALPNPolicy) != 0 {
		sdkObj.AlpnPolicy = lsSpec.ALPNPolicy
	}
	sdkObj.MutualAuthentication = buildSDKMutualAuthenticationConfig(lsSpec.MutualAuthentication)

	return sdkObj, nil
}

func buildSDKModifyListenerInput(lsSpec elbv2model.ListenerSpec, desiredDefaultActions []elbv2types.Action, desiredDefaultCerts []elbv2types.Certificate, removeMTLS bool, removeALPN bool) *elbv2sdk.ModifyListenerInput {
	sdkObj := &elbv2sdk.ModifyListenerInput{}
	sdkObj.Port = awssdk.Int32(lsSpec.Port)
	sdkObj.Protocol = elbv2types.ProtocolEnum(lsSpec.Protocol)
	sdkObj.DefaultActions = desiredDefaultActions
	sdkObj.Certificates = desiredDefaultCerts
	sdkObj.SslPolicy = lsSpec.SSLPolicy

	if removeALPN {
		sdkObj.AlpnPolicy = alpnNone
	} else if len(lsSpec.ALPNPolicy) != 0 {
		sdkObj.AlpnPolicy = lsSpec.ALPNPolicy
	}

	if removeMTLS {
		sdkObj.MutualAuthentication = mTLSOff
	} else {
		sdkObj.MutualAuthentication = buildSDKMutualAuthenticationConfig(lsSpec.MutualAuthentication)
	}

	return sdkObj
}

// buildSDKCertificates builds the certificate list for listener.
// returns the default certificates and extra certificates.
func buildSDKCertificates(modelCerts []elbv2model.Certificate) ([]elbv2types.Certificate, []elbv2types.Certificate) {
	if len(modelCerts) == 0 {
		return nil, nil
	}

	var defaultSDKCerts []elbv2types.Certificate
	var extraSDKCerts []elbv2types.Certificate
	defaultSDKCerts = append(defaultSDKCerts, buildSDKCertificate(modelCerts[0]))
	for _, cert := range modelCerts {
		extraSDKCerts = append(extraSDKCerts, buildSDKCertificate(cert))
	}
	return defaultSDKCerts, extraSDKCerts
}

func buildSDKCertificate(modelCert elbv2model.Certificate) elbv2types.Certificate {
	return elbv2types.Certificate{
		CertificateArn: modelCert.CertificateARN,
	}
}

// buildSDKMutualAuthenticationConfig builds the mutual TLS authentication config for listener
func buildSDKMutualAuthenticationConfig(modelMutualAuthenticationCfg *elbv2model.MutualAuthenticationAttributes) *elbv2types.MutualAuthenticationAttributes {
	if modelMutualAuthenticationCfg == nil {
		return nil
	}
	attributes := &elbv2types.MutualAuthenticationAttributes{
		IgnoreClientCertificateExpiry: modelMutualAuthenticationCfg.IgnoreClientCertificateExpiry,
		Mode:                          awssdk.String(modelMutualAuthenticationCfg.Mode),
		TrustStoreArn:                 modelMutualAuthenticationCfg.TrustStoreArn,
	}

	if modelMutualAuthenticationCfg.Mode == string(elbv2model.MutualAuthenticationVerifyMode) {
		attributes.AdvertiseTrustStoreCaNames = translateAdvertiseCAToEnum(modelMutualAuthenticationCfg.AdvertiseTrustStoreCaNames)
	}

	return attributes
}

func buildResListenerStatus(sdkLS ListenerWithTags) elbv2model.ListenerStatus {
	return elbv2model.ListenerStatus{
		ListenerARN: awssdk.ToString(sdkLS.Listener.ListenerArn),
	}
}

func areListenerAttributesSupported(protocol elbv2model.Protocol) bool {
	supported, exists := PROTOCOLS_SUPPORTING_LISTENER_ATTRIBUTES[protocol]
	return exists && supported
}

func getRegionFromARN(arn string) string {
	if strings.HasPrefix(arn, "arn:") {
		arnElements := strings.Split(arn, ":")
		if len(arnElements) > 3 {
			return arnElements[3]
		}
	}
	return ""
}

// isRemoveMTLS -- should we inject the 'off' button for mutual auth?
func isRemoveMTLS(sdkLS ListenerWithTags, desiredDefaultMutualAuthentication *elbv2types.MutualAuthenticationAttributes) bool {
	// If the mutual auth annotation config was specified, we never inject our own value.
	if desiredDefaultMutualAuthentication != nil {
		return false
	}

	// If the mutual auth is already turned off on the listener, we don't need to inject off again.
	if sdkLS.Listener.MutualAuthentication == nil || sdkLS.Listener.MutualAuthentication.Mode == nil || *sdkLS.Listener.MutualAuthentication.Mode == string(elbv2model.MutualAuthenticationOffMode) {
		return false
	}

	// Now, we can inject IIF there is mutual auth specified on the listener.
	return sdkLS.Listener.MutualAuthentication != nil
}

// isRemoveALPN -- should we inject the 'off' button for alpn?
func isRemoveALPN(sdkLS ListenerWithTags, lsSpec elbv2model.ListenerSpec) bool {
	// If the desired state already has alpn data, we should not inject.
	if lsSpec.ALPNPolicy != nil && len(lsSpec.ALPNPolicy) > 0 {
		return false
	}

	// We should not inject alpn data if the SDK already has alpn set to none OR has no data for it.
	if sdkLS.Listener.AlpnPolicy == nil || len(sdkLS.Listener.AlpnPolicy) == 0 || sdkLS.Listener.AlpnPolicy[0] == string(elbv2model.ALPNPolicyNone) {
		return false
	}

	// Getting here indicates that the desired state is nil AND the SDK has ALPN data which needs to be removed.
	return true
}

func isIsolatedRegion(region string) bool {
	return strings.Contains(strings.ToLower(region), "-iso-")
}

func translateAdvertiseCAToEnum(s *string) elbv2types.AdvertiseTrustStoreCaNamesEnum {
	if s == nil {
		return elbv2types.AdvertiseTrustStoreCaNamesEnumOff
	}
	return elbv2types.AdvertiseTrustStoreCaNamesEnum(*s)
}
