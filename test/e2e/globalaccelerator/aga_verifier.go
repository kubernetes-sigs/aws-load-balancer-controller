package globalaccelerator

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

type PortRangeExpectation struct {
	FromPort int32
	ToPort   int32
}

type PortOverrideExpectation struct {
	ListenerPort int32
	EndpointPort int32
}

type EndpointGroupExpectation struct {
	TrafficDialPercentage int32
	PortOverrides         []PortOverrideExpectation
	NumEndpoints          int
}

type ListenerExpectation struct {
	Protocol       string
	PortRanges     []PortRangeExpectation
	ClientAffinity string
	EndpointGroups []EndpointGroupExpectation
}

type GlobalAcceleratorExpectation struct {
	Name          string
	IPAddressType string
	Status        string
	Listeners     []ListenerExpectation
}

func verifyGlobalAcceleratorConfiguration(ctx context.Context, f *framework.Framework, acceleratorARN string, expected GlobalAcceleratorExpectation) error {
	agaClient := f.Cloud.GlobalAccelerator()

	describeAccelResp, err := agaClient.DescribeAcceleratorWithContext(ctx, &globalaccelerator.DescribeAcceleratorInput{
		AcceleratorArn: awssdk.String(acceleratorARN),
	})
	if err != nil {
		return err
	}

	accelerator := describeAccelResp.Accelerator
	if expected.Name != "" && awssdk.ToString(accelerator.Name) != expected.Name {
		return fmt.Errorf("name mismatch: expected %s, got %s", expected.Name, awssdk.ToString(accelerator.Name))
	}
	if expected.IPAddressType != "" && string(accelerator.IpAddressType) != expected.IPAddressType {
		return fmt.Errorf("IP address type mismatch: expected %s, got %s", expected.IPAddressType, string(accelerator.IpAddressType))
	}
	if expected.Status != "" && string(accelerator.Status) != expected.Status {
		return fmt.Errorf("status mismatch: expected %s, got %s", expected.Status, string(accelerator.Status))
	}

	if len(expected.Listeners) > 0 {
		listListenersResp, err := agaClient.ListListenersForAcceleratorWithContext(ctx, &globalaccelerator.ListListenersInput{
			AcceleratorArn: awssdk.String(acceleratorARN),
		})
		if err != nil {
			return err
		}
		if len(listListenersResp.Listeners) != len(expected.Listeners) {
			return fmt.Errorf("listener count mismatch: expected %d, got %d", len(expected.Listeners), len(listListenersResp.Listeners))
		}

		for i, expectedListener := range expected.Listeners {
			listener := listListenersResp.Listeners[i]

			if expectedListener.Protocol != "" && string(listener.Protocol) != expectedListener.Protocol {
				return fmt.Errorf("listener[%d] protocol mismatch: expected %s, got %s", i, expectedListener.Protocol, string(listener.Protocol))
			}
			if expectedListener.ClientAffinity != "" && string(listener.ClientAffinity) != expectedListener.ClientAffinity {
				return fmt.Errorf("listener[%d] client affinity mismatch: expected %s, got %s", i, expectedListener.ClientAffinity, string(listener.ClientAffinity))
			}

			if len(expectedListener.PortRanges) > 0 {
				if len(listener.PortRanges) != len(expectedListener.PortRanges) {
					return fmt.Errorf("listener[%d] port range count mismatch: expected %d, got %d", i, len(expectedListener.PortRanges), len(listener.PortRanges))
				}
				for j, expectedPortRange := range expectedListener.PortRanges {
					if awssdk.ToInt32(listener.PortRanges[j].FromPort) != expectedPortRange.FromPort {
						return fmt.Errorf("listener[%d] port range[%d] from port mismatch: expected %d, got %d", i, j, expectedPortRange.FromPort, awssdk.ToInt32(listener.PortRanges[j].FromPort))
					}
					if awssdk.ToInt32(listener.PortRanges[j].ToPort) != expectedPortRange.ToPort {
						return fmt.Errorf("listener[%d] port range[%d] to port mismatch: expected %d, got %d", i, j, expectedPortRange.ToPort, awssdk.ToInt32(listener.PortRanges[j].ToPort))
					}
				}
			}

			if len(expectedListener.EndpointGroups) > 0 {
				listEGResp, err := agaClient.ListEndpointGroupsAsList(ctx, &globalaccelerator.ListEndpointGroupsInput{
					ListenerArn: listener.ListenerArn,
				})
				if err != nil {
					return err
				}
				if len(listEGResp) != len(expectedListener.EndpointGroups) {
					return fmt.Errorf("listener[%d] endpoint group count mismatch: expected %d, got %d", i, len(expectedListener.EndpointGroups), len(listEGResp))
				}

				for k, expectedEG := range expectedListener.EndpointGroups {
					eg := listEGResp[k]

					if expectedEG.TrafficDialPercentage > 0 && awssdk.ToFloat32(eg.TrafficDialPercentage) != float32(expectedEG.TrafficDialPercentage) {
						return fmt.Errorf("listener[%d] endpoint group[%d] traffic dial percentage mismatch: expected %d, got %f", i, k, expectedEG.TrafficDialPercentage, awssdk.ToFloat32(eg.TrafficDialPercentage))
					}

					if len(expectedEG.PortOverrides) > 0 {
						if len(eg.PortOverrides) != len(expectedEG.PortOverrides) {
							return fmt.Errorf("listener[%d] endpoint group[%d] port override count mismatch: expected %d, got %d", i, k, len(expectedEG.PortOverrides), len(eg.PortOverrides))
						}
						for l, expectedPO := range expectedEG.PortOverrides {
							if awssdk.ToInt32(eg.PortOverrides[l].ListenerPort) != expectedPO.ListenerPort {
								return fmt.Errorf("listener[%d] endpoint group[%d] port override[%d] listener port mismatch: expected %d, got %d", i, k, l, expectedPO.ListenerPort, awssdk.ToInt32(eg.PortOverrides[l].ListenerPort))
							}
							if awssdk.ToInt32(eg.PortOverrides[l].EndpointPort) != expectedPO.EndpointPort {
								return fmt.Errorf("listener[%d] endpoint group[%d] port override[%d] endpoint port mismatch: expected %d, got %d", i, k, l, expectedPO.EndpointPort, awssdk.ToInt32(eg.PortOverrides[l].EndpointPort))
							}
						}
					}

					if expectedEG.NumEndpoints > 0 && len(eg.EndpointDescriptions) != expectedEG.NumEndpoints {
						return fmt.Errorf("listener[%d] endpoint group[%d] endpoint count mismatch: expected %d, got %d", i, k, expectedEG.NumEndpoints, len(eg.EndpointDescriptions))
					}
				}
			}
		}
	}

	return nil
}

func waitForEndpointsHealthy(ctx context.Context, f *framework.Framework, acceleratorARN string) error {
	agaClient := f.Cloud.GlobalAccelerator()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	return wait.PollImmediateUntil(utils.PollIntervalMedium, func() (bool, error) {
		listListenersResp, err := agaClient.ListListenersForAcceleratorWithContext(ctx, &globalaccelerator.ListListenersInput{
			AcceleratorArn: awssdk.String(acceleratorARN),
		})
		if err != nil {
			return false, err
		}

		hasEndpoints := false
		for _, listener := range listListenersResp.Listeners {
			listEGResp, err := agaClient.ListEndpointGroupsAsList(ctx, &globalaccelerator.ListEndpointGroupsInput{
				ListenerArn: listener.ListenerArn,
			})
			if err != nil {
				return false, err
			}

			for _, eg := range listEGResp {
				if len(eg.EndpointDescriptions) == 0 {
					f.Logger.Info("waiting for endpoints to be added", "endpointGroupArn", awssdk.ToString(eg.EndpointGroupArn))
					return false, nil
				}
				hasEndpoints = true
				for _, endpoint := range eg.EndpointDescriptions {
					if endpoint.HealthState != types.HealthStateHealthy {
						f.Logger.Info("waiting for endpoint to be healthy",
							"endpointId", awssdk.ToString(endpoint.EndpointId),
							"healthState", string(endpoint.HealthState))
						return false, nil
					}
				}
			}
		}
		if !hasEndpoints {
			f.Logger.Info("no endpoints found in any endpoint group")
			return false, nil
		}
		return true, nil
	}, timeoutCtx.Done())
}

func verifyLoadBalancerScheme(ctx context.Context, f *framework.Framework, lbHostname, expectedScheme string) error {
	elbClient := f.Cloud.ELBV2()
	lbs, err := elbClient.DescribeLoadBalancersAsList(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
	if err != nil {
		return fmt.Errorf("failed to describe load balancers: %w", err)
	}

	for _, lb := range lbs {
		if awssdk.ToString(lb.DNSName) == lbHostname {
			actualScheme := string(lb.Scheme)
			if actualScheme != expectedScheme {
				return fmt.Errorf("load balancer scheme mismatch: expected %s, got %s", expectedScheme, actualScheme)
			}
			f.Logger.Info("verified load balancer scheme", "hostname", lbHostname, "scheme", actualScheme)
			return nil
		}
	}
	return fmt.Errorf("load balancer with hostname %s not found", lbHostname)
}

func verifyEndpointPointsToLoadBalancer(ctx context.Context, f *framework.Framework, acceleratorARN, expectedLBHostname string) error {
	agaClient := f.Cloud.GlobalAccelerator()
	elbClient := f.Cloud.ELBV2()

	lbs, err := elbClient.DescribeLoadBalancersAsList(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
	if err != nil {
		return fmt.Errorf("failed to describe load balancers: %w", err)
	}

	var expectedLBARN string
	for _, lb := range lbs {
		if awssdk.ToString(lb.DNSName) == expectedLBHostname {
			expectedLBARN = awssdk.ToString(lb.LoadBalancerArn)
			break
		}
	}
	if expectedLBARN == "" {
		return fmt.Errorf("load balancer with hostname %s not found", expectedLBHostname)
	}

	listListenersResp, err := agaClient.ListListenersForAcceleratorWithContext(ctx, &globalaccelerator.ListListenersInput{
		AcceleratorArn: awssdk.String(acceleratorARN),
	})
	if err != nil {
		return err
	}

	for _, listener := range listListenersResp.Listeners {
		listEGResp, err := agaClient.ListEndpointGroupsAsList(ctx, &globalaccelerator.ListEndpointGroupsInput{
			ListenerArn: listener.ListenerArn,
		})
		if err != nil {
			return err
		}

		for _, eg := range listEGResp {
			if len(eg.EndpointDescriptions) == 0 {
				return fmt.Errorf("no endpoints in endpoint group %s", awssdk.ToString(eg.EndpointGroupArn))
			}
			for _, endpoint := range eg.EndpointDescriptions {
				if endpoint.HealthState != types.HealthStateHealthy {
					return fmt.Errorf("endpoint %s not healthy: %s", awssdk.ToString(endpoint.EndpointId), string(endpoint.HealthState))
				}
				if awssdk.ToString(endpoint.EndpointId) != expectedLBARN {
					return fmt.Errorf("endpoint ARN mismatch: expected %s, got %s", expectedLBARN, awssdk.ToString(endpoint.EndpointId))
				}
				f.Logger.Info("verified endpoint points to correct load balancer",
					"endpointId", awssdk.ToString(endpoint.EndpointId),
					"expectedLBARN", expectedLBARN,
					"healthState", string(endpoint.HealthState))
			}
		}
	}
	return nil
}
