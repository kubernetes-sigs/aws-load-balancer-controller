package logdelivery

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	logdeliverymodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/logdelivery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	reconcileTimeout = 15 * time.Minute
	pollInterval     = 10 * time.Second
)

type deliveryState struct {
	sources      map[string]cwltypes.DeliverySource
	destinations map[string]cwltypes.DeliveryDestination // keyed by ARN
	deliveries   map[string]cwltypes.Delivery
}

// readDeliveryState identifies sources by the real LB ARN, independently of the controller's naming code.
func readDeliveryState(ctx context.Context, cwl services.CloudWatchLogs, lbARN string) (deliveryState, error) {
	state := deliveryState{sources: map[string]cwltypes.DeliverySource{}, destinations: map[string]cwltypes.DeliveryDestination{}, deliveries: map[string]cwltypes.Delivery{}}
	sources, err := cwl.DescribeDeliverySourcesAsList(ctx, &cloudwatchlogssdk.DescribeDeliverySourcesInput{})
	if err != nil {
		return state, err
	}
	for _, source := range sources {
		if slices.Contains(source.ResourceArns, lbARN) {
			state.sources[awssdk.ToString(source.Name)] = source
		}
	}
	if len(state.sources) == 0 {
		return state, nil
	}
	deliveries, err := cwl.DescribeDeliveriesAsList(ctx, &cloudwatchlogssdk.DescribeDeliveriesInput{})
	if err != nil {
		return state, err
	}
	destinationARNs := sets.New[string]()
	for _, delivery := range deliveries {
		if _, owned := state.sources[awssdk.ToString(delivery.DeliverySourceName)]; owned {
			state.deliveries[awssdk.ToString(delivery.Id)] = delivery
			destinationARNs.Insert(awssdk.ToString(delivery.DeliveryDestinationArn))
		}
	}
	destinations, err := cwl.DescribeDeliveryDestinationsAsList(ctx, &cloudwatchlogssdk.DescribeDeliveryDestinationsInput{})
	if err != nil {
		return state, err
	}
	for _, destination := range destinations {
		if destinationARNs.Has(awssdk.ToString(destination.Arn)) {
			state.destinations[awssdk.ToString(destination.Arn)] = destination
		}
	}
	return state, nil
}

func expectDeliveryState(ctx context.Context, lbARN string, configs []logdeliverymodel.Config) deliveryState {
	var state deliveryState
	Eventually(func(g Gomega) {
		var err error
		state, err = readDeliveryState(ctx, tf.Cloud.CloudWatchLogs(), lbARN)
		g.Expect(err).NotTo(HaveOccurred())
		logTypes := sets.New[string]()
		for _, cfg := range configs {
			logTypes.Insert(cfg.LogType)
		}
		g.Expect(state.sources).To(HaveLen(logTypes.Len()))
		g.Expect(state.deliveries).To(HaveLen(len(configs)))
		for _, cfg := range configs {
			matches := 0
			for _, delivery := range state.deliveries {
				source := state.sources[awssdk.ToString(delivery.DeliverySourceName)]
				if awssdk.ToString(source.LogType) != cfg.LogType {
					continue
				}
				if cfg.DeliveryDestinationARN != nil {
					if awssdk.ToString(delivery.DeliveryDestinationArn) != *cfg.DeliveryDestinationARN {
						continue
					}
				} else {
					destination, exists := state.destinations[awssdk.ToString(delivery.DeliveryDestinationArn)]
					g.Expect(exists).To(BeTrue())
					g.Expect(destination.DeliveryDestinationConfiguration).NotTo(BeNil())
					if awssdk.ToString(destination.DeliveryDestinationConfiguration.DestinationResourceArn) != *cfg.DestinationARN {
						continue
					}
					if cfg.OutputFormat != nil {
						g.Expect(string(destination.OutputFormat)).To(Equal(*cfg.OutputFormat))
					}
				}
				if cfg.FieldDelimiter != nil {
					g.Expect(awssdk.ToString(delivery.FieldDelimiter)).To(Equal(*cfg.FieldDelimiter))
				}
				if cfg.S3DeliveryConfiguration != nil {
					g.Expect(delivery.S3DeliveryConfiguration).NotTo(BeNil())
					if cfg.S3DeliveryConfiguration.SuffixPath != nil {
						g.Expect(awssdk.ToString(delivery.S3DeliveryConfiguration.SuffixPath)).To(Equal(*cfg.S3DeliveryConfiguration.SuffixPath))
					}
					if cfg.S3DeliveryConfiguration.EnableHiveCompatiblePath != nil {
						g.Expect(awssdk.ToBool(delivery.S3DeliveryConfiguration.EnableHiveCompatiblePath)).To(Equal(*cfg.S3DeliveryConfiguration.EnableHiveCompatiblePath))
					}
				}
				matches++
			}
			g.Expect(matches).To(Equal(1), "expected log delivery config: %+v", cfg)
		}
	}, reconcileTimeout, pollInterval).Should(Succeed())
	return state
}

func expectDeleted(ctx context.Context, state deliveryState, externalARN string) {
	Eventually(func(g Gomega) {
		for id := range state.deliveries {
			_, err := logsClient.GetDelivery(ctx, &cloudwatchlogssdk.GetDeliveryInput{Id: awssdk.String(id)})
			g.Expect(isNotFound(err)).To(BeTrue(), "delivery %s: %v", id, err)
		}
		for destinationARN, destination := range state.destinations {
			if destinationARN == externalARN {
				continue
			}
			_, err := logsClient.GetDeliveryDestination(ctx, &cloudwatchlogssdk.GetDeliveryDestinationInput{Name: destination.Name})
			g.Expect(isNotFound(err)).To(BeTrue(), "destination %s: %v", destinationARN, err)
		}
		for name := range state.sources {
			_, err := logsClient.GetDeliverySource(ctx, &cloudwatchlogssdk.GetDeliverySourceInput{Name: awssdk.String(name)})
			g.Expect(isNotFound(err)).To(BeTrue(), "source %s: %v", name, err)
		}
	}, reconcileTimeout, pollInterval).Should(Succeed())
}

func isNotFound(err error) bool {
	var notFound *cwltypes.ResourceNotFoundException
	return errors.As(err, &notFound)
}

func readLogGroup(ctx context.Context) *cwltypes.LogGroup {
	paginator := cloudwatchlogssdk.NewDescribeLogGroupsPaginator(logsClient, &cloudwatchlogssdk.DescribeLogGroupsInput{LogGroupNamePrefix: awssdk.String(logGroupName)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		Expect(err).NotTo(HaveOccurred())
		for _, group := range page.LogGroups {
			if awssdk.ToString(group.LogGroupName) == logGroupName {
				return &group
			}
		}
	}
	return nil
}

type resourceFixture struct {
	objects     []client.Object
	lbARN       string
	states      []deliveryState
	externalARN string
	logGroup    *cwltypes.LogGroup
}

func newFixture(ctx context.Context, objects ...client.Object) *resourceFixture {
	f := &resourceFixture{logGroup: readLogGroup(ctx)}
	Expect(f.logGroup).NotTo(BeNil())
	DeferCleanup(f.cleanup)
	for _, object := range objects {
		Expect(tf.K8sClient.Create(ctx, object)).To(Succeed())
		f.objects = append(f.objects, object)
	}
	return f
}

func (f *resourceFixture) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if f.lbARN != "" {
		if state, err := readDeliveryState(ctx, tf.Cloud.CloudWatchLogs(), f.lbARN); err == nil {
			f.states = append(f.states, state)
		}
	}
	for i := len(f.objects) - 1; i >= 0; i-- {
		object := f.objects[i]
		Expect(client.IgnoreNotFound(tf.K8sClient.Delete(ctx, object))).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(object), object.DeepCopyObject().(client.Object)))
		}, reconcileTimeout, pollInterval).Should(BeTrue(), "resource must finish deletion: %T %s", object, object.GetName())
	}
	for _, state := range f.states {
		expectDeleted(ctx, state, f.externalARN)
	}
	group := readLogGroup(ctx)
	Expect(group).NotTo(BeNil(), "the externally managed log group must remain")
	Expect(group.RetentionInDays).To(Equal(f.logGroup.RetentionInDays))
	Expect(group.KmsKeyId).To(Equal(f.logGroup.KmsKeyId))
}

func (f *resourceFixture) expectState(ctx context.Context, configs []logdeliverymodel.Config) deliveryState {
	state := expectDeliveryState(ctx, f.lbARN, configs)
	f.states = append(f.states, state)
	return state
}

func (f *resourceFixture) waitForLB(ctx context.Context, object client.Object) string {
	var dnsName string
	Eventually(func(g Gomega) {
		g.Expect(tf.K8sClient.Get(ctx, client.ObjectKeyFromObject(object), object)).To(Succeed())
		switch resource := object.(type) {
		case *networking.Ingress:
			if len(resource.Status.LoadBalancer.Ingress) > 0 {
				dnsName = resource.Status.LoadBalancer.Ingress[0].Hostname
			}
		case *corev1.Service:
			if len(resource.Status.LoadBalancer.Ingress) > 0 {
				dnsName = resource.Status.LoadBalancer.Ingress[0].Hostname
			}
		case *gwv1.Gateway:
			if len(resource.Status.Addresses) > 0 {
				dnsName = resource.Status.Addresses[0].Value
			}
		}
		g.Expect(dnsName).NotTo(BeEmpty())
		var err error
		f.lbARN, err = tf.LBManager.FindLoadBalancerByDNSName(ctx, dnsName)
		g.Expect(err).NotTo(HaveOccurred())
	}, reconcileTimeout, pollInterval).Should(Succeed())
	Expect(tf.LBManager.WaitUntilLoadBalancerAvailable(ctx, f.lbARN)).To(Succeed())
	return dnsName
}

func patchAnnotation(ctx context.Context, object client.Object, key, value string) {
	Expect(tf.K8sClient.Get(ctx, types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}, object)).To(Succeed())
	base := object.DeepCopyObject().(client.Object)
	if value == "" {
		delete(object.GetAnnotations(), key)
	} else {
		object.GetAnnotations()[key] = value
	}
	Expect(tf.K8sClient.Patch(ctx, object, client.MergeFrom(base))).To(Succeed())
}

func sendTraffic(ctx context.Context, dnsName, path string, useTLS bool) error {
	transport := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // test certificates do not match the LB DNS name
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	protocol := "http"
	if useTLS {
		protocol = "https"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s://%s/%s", protocol, dnsName, path), nil)
	if err != nil {
		return err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %d", response.StatusCode)
	}
	return nil
}

func expectLogEvents(ctx context.Context, dnsName, path, marker string, useTLS bool) {
	startTime := time.Now().Add(-time.Minute).UnixMilli()
	Eventually(func(g Gomega) {
		// Repeat requests because ELB logs are delivered on a best-effort basis.
		for i := 0; i < 3; i++ {
			g.Expect(sendTraffic(ctx, dnsName, path, useTLS)).To(Succeed())
		}
		paginator := cloudwatchlogssdk.NewFilterLogEventsPaginator(logsClient, &cloudwatchlogssdk.FilterLogEventsInput{
			LogGroupName: awssdk.String(logGroupName), StartTime: awssdk.Int64(startTime), FilterPattern: awssdk.String(fmt.Sprintf("%q", marker)),
		})
		found := false
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			for _, event := range page.Events {
				if strings.Contains(awssdk.ToString(event.Message), marker) {
					found = true
				}
			}
			if found {
				break
			}
		}
		g.Expect(found).To(BeTrue(), "logs for %s must reach %s", marker, logGroupName)
	}, reconcileTimeout, pollInterval).Should(Succeed())
}
