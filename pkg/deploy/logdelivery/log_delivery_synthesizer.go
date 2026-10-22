package logdelivery

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/go-logr/logr"
	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
)

const deliveryDestinationResourcePrefix = "delivery-destination:"

// NewLogDeliverySynthesizer constructs new logDeliverySynthesizer.
func NewLogDeliverySynthesizer(cwlClient services.CloudWatchLogs, trackingProvider tracking.Provider, clusterName string, logger logr.Logger, stack core.Stack) *logDeliverySynthesizer {
	return &logDeliverySynthesizer{
		cwlClient:        cwlClient,
		trackingProvider: trackingProvider,
		clusterName:      clusterName,
		logger:           logger,
		stack:            stack,
	}
}

// logDeliverySynthesizer reconciles the CloudWatch Logs delivery sources, delivery destinations and deliveries of a stack.
type logDeliverySynthesizer struct {
	cwlClient        services.CloudWatchLogs
	trackingProvider tracking.Provider
	clusterName      string
	logger           logr.Logger
	stack            core.Stack
}

type desiredSource struct {
	resourceARN string
	logType     string
	tags        map[string]string
}

type desiredDestination struct {
	destinationARN string
	outputFormat   *string
	tags           map[string]string
}

type desiredDelivery struct {
	sourceName string
	// destinationName is set for delivery destinations that the controller owns.
	destinationName string
	// deliveryDestinationARN is set for delivery destinations that the controller doesn't own.
	deliveryDestinationARN string
	fieldDelimiter         *string
	s3Config               *logdeliverymodel.S3DeliveryConfiguration
	tags                   map[string]string
}

type desiredState struct {
	sources      map[string]desiredSource
	destinations map[string]desiredDestination
	deliveries   []desiredDelivery
}

func (s *logDeliverySynthesizer) Synthesize(ctx context.Context) error {
	var resDeliveries []*logdeliverymodel.LogDelivery
	if err := s.stack.ListResources(&resDeliveries); err != nil {
		return fmt.Errorf("[should never happen] failed to list resources: %w", err)
	}
	// The stack lists resources in random order. Sorting keeps API calls and logs in a stable order.
	slices.SortFunc(resDeliveries, func(a, b *logdeliverymodel.LogDelivery) int {
		return strings.Compare(a.ID(), b.ID())
	})
	prefix := namePrefix(s.clusterName, s.trackingProvider.ResourceIDTagKey(), s.stack.StackID())
	desired, err := s.buildDesiredState(ctx, prefix, resDeliveries)
	if err != nil {
		return err
	}

	// CloudWatch Logs can't filter these lists, so they cover the whole account and region.
	sdkSources, err := s.cwlClient.DescribeDeliverySourcesAsList(ctx, &cloudwatchlogssdk.DescribeDeliverySourcesInput{})
	if err != nil {
		return pkgerrors.Wrap(err, "failed to list delivery sources")
	}
	stackTags := s.trackingProvider.StackTags(s.stack)
	ownedSources := make(map[string]cwltypes.DeliverySource)
	for _, sdkSource := range sdkSources {
		if name := awssdk.ToString(sdkSource.Name); isOwned(name, sdkSource.Tags, prefix, stackTags) {
			ownedSources[name] = sdkSource
		}
	}
	// Delivery sources are created first and deleted last, so a stack without any owns no destinations or deliveries either.
	if len(desired.deliveries) == 0 && len(ownedSources) == 0 {
		return nil
	}

	sdkDestinations, err := s.cwlClient.DescribeDeliveryDestinationsAsList(ctx, &cloudwatchlogssdk.DescribeDeliveryDestinationsInput{})
	if err != nil {
		return pkgerrors.Wrap(err, "failed to list delivery destinations")
	}
	ownedDestinations := make(map[string]cwltypes.DeliveryDestination)
	for _, sdkDestination := range sdkDestinations {
		if name := awssdk.ToString(sdkDestination.Name); isOwned(name, sdkDestination.Tags, prefix, stackTags) {
			ownedDestinations[name] = sdkDestination
		}
	}

	sdkDeliveries, err := s.cwlClient.DescribeDeliveriesAsList(ctx, &cloudwatchlogssdk.DescribeDeliveriesInput{})
	if err != nil {
		return pkgerrors.Wrap(err, "failed to list deliveries")
	}
	var ownedDeliveries []cwltypes.Delivery
	for _, sdkDelivery := range sdkDeliveries {
		if _, ok := ownedSources[awssdk.ToString(sdkDelivery.DeliverySourceName)]; ok {
			ownedDeliveries = append(ownedDeliveries, sdkDelivery)
		}
	}

	return s.reconcile(ctx, desired, ownedSources, ownedDestinations, ownedDeliveries)
}

func (s *logDeliverySynthesizer) PostSynthesize(_ context.Context) error {
	// nothing to do here.
	return nil
}

func (s *logDeliverySynthesizer) buildDesiredState(ctx context.Context, prefix string, resDeliveries []*logdeliverymodel.LogDelivery) (desiredState, error) {
	state := desiredState{
		sources:      make(map[string]desiredSource),
		destinations: make(map[string]desiredDestination),
	}
	stackTags := s.trackingProvider.StackTags(s.stack)
	for _, resDelivery := range resDeliveries {
		lbARN, err := resDelivery.Spec.ResourceARN.Resolve(ctx)
		if err != nil {
			return desiredState{}, err
		}
		sourceName := deliverySourceName(prefix, resDelivery.Spec.LogType)
		if _, exists := state.sources[sourceName]; !exists {
			state.sources[sourceName] = desiredSource{
				resourceARN: lbARN,
				logType:     resDelivery.Spec.LogType,
				tags:        algorithm.MergeStringMap(stackTags, resDelivery.Spec.Tags),
			}
		}

		delivery := desiredDelivery{
			sourceName:     sourceName,
			fieldDelimiter: resDelivery.Spec.FieldDelimiter,
			s3Config:       resDelivery.Spec.S3DeliveryConfiguration,
			tags:           s.trackingProvider.ResourceTags(s.stack, resDelivery, resDelivery.Spec.Tags),
		}
		if resDelivery.Spec.DeliveryDestinationARN != nil {
			delivery.deliveryDestinationARN = *resDelivery.Spec.DeliveryDestinationARN
		} else {
			destinationARN := awssdk.ToString(resDelivery.Spec.DestinationARN)
			destinationType, err := logdeliverymodel.DestinationType(destinationARN)
			if err != nil {
				return desiredState{}, err
			}
			destinationName := deliveryDestinationName(prefix, destinationType, destinationARN, awssdk.ToString(resDelivery.Spec.OutputFormat))
			state.destinations[destinationName] = desiredDestination{
				destinationARN: destinationARN,
				outputFormat:   resDelivery.Spec.OutputFormat,
				tags:           algorithm.MergeStringMap(stackTags, resDelivery.Spec.Tags),
			}
			delivery.destinationName = destinationName
		}
		state.deliveries = append(state.deliveries, delivery)
	}
	return state, nil
}

func (s *logDeliverySynthesizer) reconcile(ctx context.Context, desired desiredState, ownedSources map[string]cwltypes.DeliverySource,
	ownedDestinations map[string]cwltypes.DeliveryDestination, ownedDeliveries []cwltypes.Delivery) error {
	// A source is stale when it is no longer wanted or points at another load balancer, for example after the load balancer was replaced.
	staleSources := sets.New[string]()
	for name, sdkSource := range ownedSources {
		wanted, ok := desired.sources[name]
		if !ok || !sourceMatches(sdkSource, wanted) {
			staleSources.Insert(name)
		}
	}

	// Deliveries go first, because sources and destinations can't be deleted or replaced while a delivery uses them.
	matchedDeliveries := make(map[int]cwltypes.Delivery)
	for _, sdkDelivery := range ownedDeliveries {
		index := -1
		if !staleSources.Has(awssdk.ToString(sdkDelivery.DeliverySourceName)) {
			index = findDesiredDelivery(desired.deliveries, sdkDelivery, matchedDeliveries)
		}
		if index < 0 {
			if err := s.deleteDelivery(ctx, sdkDelivery); err != nil {
				return err
			}
			continue
		}
		matchedDeliveries[index] = sdkDelivery
	}

	for _, name := range slices.Sorted(maps.Keys(desired.sources)) {
		if _, owned := ownedSources[name]; owned && !staleSources.Has(name) {
			continue
		}
		if staleSources.Has(name) {
			if err := s.deleteSource(ctx, name); err != nil {
				return err
			}
		}
		if err := s.putSource(ctx, name, desired.sources[name]); err != nil {
			return err
		}
	}

	destinationARNs := make(map[string]string, len(desired.destinations))
	for _, name := range slices.Sorted(maps.Keys(desired.destinations)) {
		wanted := desired.destinations[name]
		if sdkDestination, owned := ownedDestinations[name]; owned && destinationMatches(sdkDestination, wanted) {
			destinationARNs[name] = awssdk.ToString(sdkDestination.Arn)
			continue
		}
		destinationARN, err := s.putDestination(ctx, name, wanted)
		if err != nil {
			return err
		}
		destinationARNs[name] = destinationARN
	}

	for index, wanted := range desired.deliveries {
		if sdkDelivery, matched := matchedDeliveries[index]; matched {
			if err := s.updateDeliveryIfNeeded(ctx, sdkDelivery, wanted); err != nil {
				return err
			}
			continue
		}
		destinationARN := wanted.deliveryDestinationARN
		if destinationARN == "" {
			destinationARN = destinationARNs[wanted.destinationName]
		}
		if err := s.createDelivery(ctx, wanted, destinationARN); err != nil {
			return err
		}
	}

	// Sources are deleted after destinations, so an owned source still marks this stack for cleanup if a destination can't be deleted yet.
	for _, name := range slices.Sorted(maps.Keys(ownedDestinations)) {
		if _, wanted := desired.destinations[name]; wanted {
			continue
		}
		if err := s.deleteDestination(ctx, name); err != nil {
			return err
		}
	}
	for _, name := range sets.List(staleSources) {
		if _, wanted := desired.sources[name]; wanted {
			continue
		}
		if err := s.deleteSource(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *logDeliverySynthesizer) putSource(ctx context.Context, name string, wanted desiredSource) error {
	s.logger.Info("creating delivery source", "name", name, "logType", wanted.logType, "resourceARN", wanted.resourceARN)
	if _, err := s.cwlClient.PutDeliverySourceWithContext(ctx, &cloudwatchlogssdk.PutDeliverySourceInput{
		Name:        awssdk.String(name),
		ResourceArn: awssdk.String(wanted.resourceARN),
		LogType:     awssdk.String(wanted.logType),
		Tags:        wanted.tags,
	}); err != nil {
		var conflictErr *cwltypes.ConflictException
		if errors.As(err, &conflictErr) {
			return pkgerrors.Wrapf(err, "failed to create delivery source %s: the load balancer may already have a delivery source for %s that the controller doesn't manage", name, wanted.logType)
		}
		return pkgerrors.Wrapf(err, "failed to create delivery source %s", name)
	}
	s.logger.Info("created delivery source", "name", name)
	return nil
}

func (s *logDeliverySynthesizer) deleteSource(ctx context.Context, name string) error {
	s.logger.Info("deleting delivery source", "name", name)
	if _, err := s.cwlClient.DeleteDeliverySourceWithContext(ctx, &cloudwatchlogssdk.DeleteDeliverySourceInput{
		Name: awssdk.String(name),
	}); err != nil && !isNotFound(err) {
		return pkgerrors.Wrapf(err, "failed to delete delivery source %s", name)
	}
	s.logger.Info("deleted delivery source", "name", name)
	return nil
}

func (s *logDeliverySynthesizer) putDestination(ctx context.Context, name string, wanted desiredDestination) (string, error) {
	s.logger.Info("creating delivery destination", "name", name, "destinationARN", wanted.destinationARN)
	input := &cloudwatchlogssdk.PutDeliveryDestinationInput{
		Name: awssdk.String(name),
		DeliveryDestinationConfiguration: &cwltypes.DeliveryDestinationConfiguration{
			DestinationResourceArn: awssdk.String(wanted.destinationARN),
		},
		Tags: wanted.tags,
	}
	if wanted.outputFormat != nil {
		input.OutputFormat = cwltypes.OutputFormat(*wanted.outputFormat)
	}
	output, err := s.cwlClient.PutDeliveryDestinationWithContext(ctx, input)
	if err != nil {
		return "", pkgerrors.Wrapf(err, "failed to create delivery destination %s", name)
	}
	if output.DeliveryDestination == nil || output.DeliveryDestination.Arn == nil {
		return "", pkgerrors.Errorf("[should never happen] delivery destination %s has no ARN", name)
	}
	s.logger.Info("created delivery destination", "name", name, "arn", awssdk.ToString(output.DeliveryDestination.Arn))
	return awssdk.ToString(output.DeliveryDestination.Arn), nil
}

func (s *logDeliverySynthesizer) deleteDestination(ctx context.Context, name string) error {
	s.logger.Info("deleting delivery destination", "name", name)
	if _, err := s.cwlClient.DeleteDeliveryDestinationWithContext(ctx, &cloudwatchlogssdk.DeleteDeliveryDestinationInput{
		Name: awssdk.String(name),
	}); err != nil && !isNotFound(err) {
		return pkgerrors.Wrapf(err, "failed to delete delivery destination %s", name)
	}
	s.logger.Info("deleted delivery destination", "name", name)
	return nil
}

func (s *logDeliverySynthesizer) createDelivery(ctx context.Context, wanted desiredDelivery, destinationARN string) error {
	s.logger.Info("creating delivery", "source", wanted.sourceName, "destinationARN", destinationARN)
	input := &cloudwatchlogssdk.CreateDeliveryInput{
		DeliverySourceName:     awssdk.String(wanted.sourceName),
		DeliveryDestinationArn: awssdk.String(destinationARN),
		FieldDelimiter:         wanted.fieldDelimiter,
		Tags:                   wanted.tags,
	}
	if wanted.s3Config != nil {
		input.S3DeliveryConfiguration = &cwltypes.S3DeliveryConfiguration{
			SuffixPath:               wanted.s3Config.SuffixPath,
			EnableHiveCompatiblePath: wanted.s3Config.EnableHiveCompatiblePath,
		}
	}
	output, err := s.cwlClient.CreateDeliveryWithContext(ctx, input)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to create delivery from %s to %s", wanted.sourceName, destinationARN)
	}
	deliveryID := ""
	if output.Delivery != nil {
		deliveryID = awssdk.ToString(output.Delivery.Id)
	}
	s.logger.Info("created delivery", "id", deliveryID, "source", wanted.sourceName)
	return nil
}

func (s *logDeliverySynthesizer) updateDeliveryIfNeeded(ctx context.Context, sdkDelivery cwltypes.Delivery, wanted desiredDelivery) error {
	input, needsUpdate := buildDeliveryConfigurationUpdate(sdkDelivery, wanted)
	if !needsUpdate {
		return nil
	}
	s.logger.Info("updating delivery", "id", awssdk.ToString(sdkDelivery.Id), "source", wanted.sourceName)
	if _, err := s.cwlClient.UpdateDeliveryConfigurationWithContext(ctx, input); err != nil {
		return pkgerrors.Wrapf(err, "failed to update delivery %s", awssdk.ToString(sdkDelivery.Id))
	}
	s.logger.Info("updated delivery", "id", awssdk.ToString(sdkDelivery.Id))
	return nil
}

func (s *logDeliverySynthesizer) deleteDelivery(ctx context.Context, sdkDelivery cwltypes.Delivery) error {
	s.logger.Info("deleting delivery", "id", awssdk.ToString(sdkDelivery.Id), "source", awssdk.ToString(sdkDelivery.DeliverySourceName))
	if _, err := s.cwlClient.DeleteDeliveryWithContext(ctx, &cloudwatchlogssdk.DeleteDeliveryInput{
		Id: sdkDelivery.Id,
	}); err != nil && !isNotFound(err) {
		return pkgerrors.Wrapf(err, "failed to delete delivery %s", awssdk.ToString(sdkDelivery.Id))
	}
	s.logger.Info("deleted delivery", "id", awssdk.ToString(sdkDelivery.Id))
	return nil
}

// buildDeliveryConfigurationUpdate compares only the settings users set explicitly. CloudWatch Logs fills in defaults for the rest.
func buildDeliveryConfigurationUpdate(sdkDelivery cwltypes.Delivery, wanted desiredDelivery) (*cloudwatchlogssdk.UpdateDeliveryConfigurationInput, bool) {
	input := &cloudwatchlogssdk.UpdateDeliveryConfigurationInput{Id: sdkDelivery.Id}
	needsUpdate := false
	if wanted.fieldDelimiter != nil && awssdk.ToString(sdkDelivery.FieldDelimiter) != *wanted.fieldDelimiter {
		input.FieldDelimiter = wanted.fieldDelimiter
		needsUpdate = true
	}
	if wanted.s3Config != nil {
		current := cwltypes.S3DeliveryConfiguration{}
		if sdkDelivery.S3DeliveryConfiguration != nil {
			current = *sdkDelivery.S3DeliveryConfiguration
		}
		updated := cwltypes.S3DeliveryConfiguration{
			SuffixPath:               current.SuffixPath,
			EnableHiveCompatiblePath: current.EnableHiveCompatiblePath,
		}
		if wanted.s3Config.SuffixPath != nil {
			updated.SuffixPath = wanted.s3Config.SuffixPath
		}
		if wanted.s3Config.EnableHiveCompatiblePath != nil {
			updated.EnableHiveCompatiblePath = wanted.s3Config.EnableHiveCompatiblePath
		}
		if awssdk.ToString(updated.SuffixPath) != awssdk.ToString(current.SuffixPath) ||
			awssdk.ToBool(updated.EnableHiveCompatiblePath) != awssdk.ToBool(current.EnableHiveCompatiblePath) {
			input.S3DeliveryConfiguration = &updated
			needsUpdate = true
		}
	}
	return input, needsUpdate
}

// findDesiredDelivery returns the index of the first unmatched desired delivery with the same source and destination, or -1.
func findDesiredDelivery(deliveries []desiredDelivery, sdkDelivery cwltypes.Delivery, matched map[int]cwltypes.Delivery) int {
	sdkSourceName := awssdk.ToString(sdkDelivery.DeliverySourceName)
	sdkDestinationARN := awssdk.ToString(sdkDelivery.DeliveryDestinationArn)
	for index, wanted := range deliveries {
		if _, taken := matched[index]; taken || wanted.sourceName != sdkSourceName {
			continue
		}
		if wanted.deliveryDestinationARN != "" {
			if wanted.deliveryDestinationARN == sdkDestinationARN {
				return index
			}
			continue
		}
		if deliveryDestinationNameFromARN(sdkDestinationARN) == wanted.destinationName {
			return index
		}
	}
	return -1
}

func deliveryDestinationNameFromARN(deliveryDestinationARN string) string {
	parsed, err := arn.Parse(deliveryDestinationARN)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(parsed.Resource, deliveryDestinationResourcePrefix)
}

// isOwned reports whether a delivery source or destination belongs to the stack: its name has the stack's prefix and,
// where CloudWatch Logs returns tags, none of the stack tags has a different value.
func isOwned(name string, tags map[string]string, prefix string, stackTags map[string]string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	for key, value := range stackTags {
		if actual, tagged := tags[key]; tagged && actual != value {
			return false
		}
	}
	return true
}

func sourceMatches(sdkSource cwltypes.DeliverySource, wanted desiredSource) bool {
	return awssdk.ToString(sdkSource.LogType) == wanted.logType &&
		len(sdkSource.ResourceArns) == 1 && sdkSource.ResourceArns[0] == wanted.resourceARN
}

func destinationMatches(sdkDestination cwltypes.DeliveryDestination, wanted desiredDestination) bool {
	if sdkDestination.DeliveryDestinationConfiguration == nil ||
		awssdk.ToString(sdkDestination.DeliveryDestinationConfiguration.DestinationResourceArn) != wanted.destinationARN {
		return false
	}
	return wanted.outputFormat == nil || string(sdkDestination.OutputFormat) == *wanted.outputFormat
}

func isNotFound(err error) bool {
	var notFoundErr *cwltypes.ResourceNotFoundException
	return errors.As(err, &notFoundErr)
}
