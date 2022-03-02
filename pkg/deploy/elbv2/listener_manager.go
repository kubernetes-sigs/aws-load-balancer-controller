package elbv2

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
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
	"time"
)

// ListenerManager is responsible for create/update/delete Listener resources.
type ListenerManager interface {
	Create(ctx context.Context, resLS *elbv2model.Listener) (elbv2model.ListenerStatus, error)

	Update(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) (elbv2model.ListenerStatus, error)

	Delete(ctx context.Context, sdkLS ListenerWithTags) error
}

func NewDefaultListenerManager(elbv2Client services.ELBV2, trackingProvider tracking.Provider,
	taggingManager TaggingManager, externalManagedTags []string, featureGates config.FeatureGates, logger logr.Logger) *defaultListenerManager {
	return &defaultListenerManager{
		elbv2Client:                 elbv2Client,
		trackingProvider:            trackingProvider,
		taggingManager:              taggingManager,
		externalManagedTags:         externalManagedTags,
		featureGates:                featureGates,
		logger:                      logger,
		waitLSExistencePollInterval: defaultWaitLSExistencePollInterval,
		waitLSExistenceTimeout:      defaultWaitLSExistenceTimeout,
	}
}

var _ ListenerManager = &defaultListenerManager{}

// default implementation for ListenerManager
type defaultListenerManager struct {
	elbv2Client         services.ELBV2
	trackingProvider    tracking.Provider
	taggingManager      TaggingManager
	externalManagedTags []string
	featureGates        config.FeatureGates
	logger              logr.Logger

	waitLSExistencePollInterval time.Duration
	waitLSExistenceTimeout      time.Duration
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
		Listener: resp.Listeners[0],
		Tags:     lsTags,
	}
	m.logger.Info("created listener",
		"stackID", resLS.Stack().StackID(),
		"resourceID", resLS.ID(),
		"arn", awssdk.StringValue(sdkLS.Listener.ListenerArn))

	if err := runtime.RetryImmediateOnError(m.waitLSExistencePollInterval, m.waitLSExistenceTimeout, isListenerNotFoundError, func() error {
		return m.updateSDKListenerWithExtraCertificates(ctx, resLS, sdkLS, true)
	}); err != nil {
		return elbv2model.ListenerStatus{}, errors.Wrap(err, "failed to update extra certificates on listener")
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
	return buildResListenerStatus(sdkLS), nil
}

func (m *defaultListenerManager) Delete(ctx context.Context, sdkLS ListenerWithTags) error {
	req := &elbv2sdk.DeleteListenerInput{
		ListenerArn: sdkLS.Listener.ListenerArn,
	}
	m.logger.Info("deleting listener",
		"arn", awssdk.StringValue(req.ListenerArn))
	if _, err := m.elbv2Client.DeleteListenerWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("deleted listener",
		"arn", awssdk.StringValue(req.ListenerArn))
	return nil
}

func (m *defaultListenerManager) updateSDKListenerWithTags(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) error {
	desiredLSTags := m.trackingProvider.ResourceTags(resLS.Stack(), resLS, resLS.Spec.Tags)
	return m.taggingManager.ReconcileTags(ctx, awssdk.StringValue(sdkLS.Listener.ListenerArn), desiredLSTags,
		WithCurrentTags(sdkLS.Tags),
		WithIgnoredTagKeys(m.externalManagedTags))
}

func (m *defaultListenerManager) updateSDKListenerWithSettings(ctx context.Context, resLS *elbv2model.Listener, sdkLS ListenerWithTags) error {
	desiredDefaultActions, err := buildSDKActions(resLS.Spec.DefaultActions, m.featureGates)
	if err != nil {
		return err
	}
	desiredDefaultCerts, _ := buildSDKCertificates(resLS.Spec.Certificates)
	if !isSDKListenerSettingsDrifted(resLS.Spec, sdkLS, desiredDefaultActions, desiredDefaultCerts) {
		return nil
	}
	req := buildSDKModifyListenerInput(resLS.Spec, desiredDefaultActions, desiredDefaultCerts)
	req.ListenerArn = sdkLS.Listener.ListenerArn
	m.logger.Info("modifying listener",
		"stackID", resLS.Stack().StackID(),
		"resourceID", resLS.ID(),
		"arn", awssdk.StringValue(sdkLS.Listener.ListenerArn))
	if _, err := m.elbv2Client.ModifyListenerWithContext(ctx, req); err != nil {
		return err
	}
	m.logger.Info("modified listener",
		"stackID", resLS.Stack().StackID(),
		"resourceID", resLS.ID(),
		"arn", awssdk.StringValue(sdkLS.Listener.ListenerArn))
	return nil
}

// updateSDKListenerWithExtraCertificates will update the extra certificates on listener.
// currentExtraCertificates is the current extra certificates, if it's nil, the current extra certificates will be fetched from AWS.
func (m *defaultListenerManager) updateSDKListenerWithExtraCertificates(ctx context.Context, resLS *elbv2model.Listener,
	sdkLS ListenerWithTags, isNewSDKListener bool) error {
	// if TLS is not supported, we shouldn't update
	if sdkLS.Listener.SslPolicy == nil {
		m.logger.V(1).Info("SDK Listner doesn't have SSL Policy set, we skip updating extra certs for non-TLS listener.")
		return nil
	}

	desiredExtraCertARNs := sets.NewString()
	_, desiredExtraCerts := buildSDKCertificates(resLS.Spec.Certificates)
	for _, cert := range desiredExtraCerts {
		desiredExtraCertARNs.Insert(awssdk.StringValue(cert.CertificateArn))
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
			Certificates: []*elbv2sdk.Certificate{
				{
					CertificateArn: awssdk.String(certARN),
				},
			},
		}
		m.logger.Info("removing certificate from listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.StringValue(sdkLS.Listener.ListenerArn),
			"certificateARN", certARN)
		if _, err := m.elbv2Client.RemoveListenerCertificatesWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("removed certificate from listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.StringValue(sdkLS.Listener.ListenerArn),
			"certificateARN", certARN)
	}

	for _, certARN := range desiredExtraCertARNs.Difference(currentExtraCertARNs).List() {
		req := &elbv2sdk.AddListenerCertificatesInput{
			ListenerArn: sdkLS.Listener.ListenerArn,
			Certificates: []*elbv2sdk.Certificate{
				{
					CertificateArn: awssdk.String(certARN),
				},
			},
		}
		m.logger.Info("adding certificate to listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.StringValue(sdkLS.Listener.ListenerArn),
			"certificateARN", certARN)
		if _, err := m.elbv2Client.AddListenerCertificatesWithContext(ctx, req); err != nil {
			return err
		}
		m.logger.Info("added certificate to listener",
			"stackID", resLS.Stack().StackID(),
			"resourceID", resLS.ID(),
			"arn", awssdk.StringValue(sdkLS.Listener.ListenerArn),
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
		if !awssdk.BoolValue(cert.IsDefault) {
			extraCertARNs = append(extraCertARNs, awssdk.StringValue(cert.CertificateArn))
		}
	}
	return extraCertARNs, nil
}

func isSDKListenerSettingsDrifted(lsSpec elbv2model.ListenerSpec, sdkLS ListenerWithTags,
	desiredDefaultActions []*elbv2sdk.Action, desiredDefaultCerts []*elbv2sdk.Certificate) bool {
	if lsSpec.Port != awssdk.Int64Value(sdkLS.Listener.Port) {
		return true
	}
	if string(lsSpec.Protocol) != awssdk.StringValue(sdkLS.Listener.Protocol) {
		return true
	}
	if !cmp.Equal(desiredDefaultActions, sdkLS.Listener.DefaultActions, elbv2equality.CompareOptionForActions()) {
		return true
	}
	if !cmp.Equal(desiredDefaultCerts, sdkLS.Listener.Certificates, elbv2equality.CompareOptionForCertificates()) {
		return true
	}
	if lsSpec.SSLPolicy != nil && awssdk.StringValue(lsSpec.SSLPolicy) != awssdk.StringValue(sdkLS.Listener.SslPolicy) {
		return true
	}
	if len(lsSpec.ALPNPolicy) != 0 && !cmp.Equal(lsSpec.ALPNPolicy, awssdk.StringValueSlice(sdkLS.Listener.AlpnPolicy), cmpopts.EquateEmpty()) {
		return true
	}

	return false
}

func buildSDKCreateListenerInput(lsSpec elbv2model.ListenerSpec, featureGates config.FeatureGates) (*elbv2sdk.CreateListenerInput, error) {
	ctx := context.Background()
	lbARN, err := lsSpec.LoadBalancerARN.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	sdkObj := &elbv2sdk.CreateListenerInput{}
	sdkObj.LoadBalancerArn = awssdk.String(lbARN)
	sdkObj.Port = awssdk.Int64(lsSpec.Port)
	sdkObj.Protocol = awssdk.String(string(lsSpec.Protocol))
	defaultActions, err := buildSDKActions(lsSpec.DefaultActions, featureGates)
	if err != nil {
		return nil, err
	}
	sdkObj.DefaultActions = defaultActions
	sdkObj.Certificates, _ = buildSDKCertificates(lsSpec.Certificates)
	sdkObj.SslPolicy = lsSpec.SSLPolicy
	if len(lsSpec.ALPNPolicy) != 0 {
		sdkObj.AlpnPolicy = awssdk.StringSlice(lsSpec.ALPNPolicy)
	}
	return sdkObj, nil
}

func buildSDKModifyListenerInput(lsSpec elbv2model.ListenerSpec, desiredDefaultActions []*elbv2sdk.Action, desiredDefaultCerts []*elbv2sdk.Certificate) *elbv2sdk.ModifyListenerInput {
	sdkObj := &elbv2sdk.ModifyListenerInput{}
	sdkObj.Port = awssdk.Int64(lsSpec.Port)
	sdkObj.Protocol = awssdk.String(string(lsSpec.Protocol))
	sdkObj.DefaultActions = desiredDefaultActions
	sdkObj.Certificates = desiredDefaultCerts
	sdkObj.SslPolicy = lsSpec.SSLPolicy
	if len(lsSpec.ALPNPolicy) != 0 {
		sdkObj.AlpnPolicy = awssdk.StringSlice(lsSpec.ALPNPolicy)
	}
	return sdkObj
}

// buildSDKCertificates builds the certificate list for listener.
// returns the default certificates and extra certificates.
func buildSDKCertificates(modelCerts []elbv2model.Certificate) ([]*elbv2sdk.Certificate, []*elbv2sdk.Certificate) {
	if len(modelCerts) == 0 {
		return nil, nil
	}

	var defaultSDKCerts []*elbv2sdk.Certificate
	var extraSDKCerts []*elbv2sdk.Certificate
	defaultSDKCerts = append(defaultSDKCerts, buildSDKCertificate(modelCerts[0]))
	for _, cert := range modelCerts[1:] {
		extraSDKCerts = append(extraSDKCerts, buildSDKCertificate(cert))
	}
	return defaultSDKCerts, extraSDKCerts
}

func buildSDKCertificate(modelCert elbv2model.Certificate) *elbv2sdk.Certificate {
	return &elbv2sdk.Certificate{
		CertificateArn: modelCert.CertificateARN,
	}
}

func buildResListenerStatus(sdkLS ListenerWithTags) elbv2model.ListenerStatus {
	return elbv2model.ListenerStatus{
		ListenerARN: awssdk.StringValue(sdkLS.Listener.ListenerArn),
	}
}
