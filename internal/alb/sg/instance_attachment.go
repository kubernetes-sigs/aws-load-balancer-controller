package sg

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
)

// InstanceAttachmentController manages InstanceAttachment
type InstanceAttachmentController interface {
	// Reconcile ensures the securityGroupID specified is attached to ENIs of k8s cluster,
	// which enables inbound traffic the targets specified.
	Reconcile(ctx context.Context, groupID string, tgGroup tg.TargetGroupGroup) error

	// Delete ensures the securityGroupID specified is not attached to ENIs of k8s cluster.
	Delete(ctx context.Context, groupID string) error
}

type instanceAttachmentController struct {
	store store.Storer
	cloud aws.CloudAPI
}

var clusterInstanceENILock = &sync.Mutex{}

func (controller *instanceAttachmentController) Reconcile(ctx context.Context, groupID string, tgGroup tg.TargetGroupGroup) error {
	clusterInstanceENILock.Lock()
	defer clusterInstanceENILock.Unlock()
	instanceENIs, err := controller.getClusterInstanceENIs()
	if err != nil {
		return fmt.Errorf("failed to get cluster ENIs due to %v", err)
	}
	supportingENIs := controller.findENIsSupportingTargets(instanceENIs, tgGroup)
	for _, enis := range instanceENIs {
		for _, eni := range enis {
			if _, ok := supportingENIs[aws.StringValue(eni.NetworkInterfaceId)]; ok {
				err := controller.ensureSGAttachedToENI(ctx, groupID, eni)
				if err != nil {
					return err
				}
			} else {
				err := controller.ensureSGDetachedFromENI(ctx, groupID, eni)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (controller *instanceAttachmentController) Delete(ctx context.Context, groupID string) error {
	clusterInstanceENILock.Lock()
	defer clusterInstanceENILock.Unlock()
	instanceENIs, err := controller.getClusterInstanceENIs()
	if err != nil {
		return fmt.Errorf("failed to get cluster enis due to %v", err)
	}
	for _, enis := range instanceENIs {
		for _, eni := range enis {
			err := controller.ensureSGDetachedFromENI(ctx, groupID, eni)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (controller *instanceAttachmentController) ensureSGAttachedToENI(ctx context.Context, sgID string, eni *ec2.InstanceNetworkInterface) error {
	desiredGroups := []string{sgID}
	for _, group := range eni.Groups {
		groupID := aws.StringValue(group.GroupId)
		if groupID == sgID {
			return nil
		}
		desiredGroups = append(desiredGroups, groupID)
	}

	albctx.GetLogger(ctx).Infof("attaching securityGroup %s to ENI %s", sgID, *eni.NetworkInterfaceId)
	_, err := controller.cloud.ModifyNetworkInterfaceAttributeWithContext(ctx, &ec2.ModifyNetworkInterfaceAttributeInput{
		NetworkInterfaceId: eni.NetworkInterfaceId,
		Groups:             aws.StringSlice(desiredGroups),
	})
	return err
}

func (controller *instanceAttachmentController) ensureSGDetachedFromENI(ctx context.Context, sgID string, eni *ec2.InstanceNetworkInterface) error {
	sgAttached := false
	desiredGroups := []string{}
	for _, group := range eni.Groups {
		groupID := aws.StringValue(group.GroupId)
		if groupID == sgID {
			sgAttached = true
		} else {
			desiredGroups = append(desiredGroups, groupID)
		}
	}
	if !sgAttached {
		return nil
	}

	albctx.GetLogger(ctx).Infof("detaching securityGroup %s from ENI %s", sgID, *eni.NetworkInterfaceId)
	_, err := controller.cloud.ModifyNetworkInterfaceAttributeWithContext(ctx, &ec2.ModifyNetworkInterfaceAttributeInput{
		NetworkInterfaceId: eni.NetworkInterfaceId,
		Groups:             aws.StringSlice(desiredGroups),
	})
	return err
}

// findENIsSupportingTargets find the ID of ENIs that are used to supporting ingress traffic to targets
func (controller *instanceAttachmentController) findENIsSupportingTargets(instanceENIs map[string][]*ec2.InstanceNetworkInterface, tgGroup tg.TargetGroupGroup) map[string]bool {
	result := make(map[string]bool)
	for _, tg := range tgGroup.TGByBackend {
		if tg.TargetType == elbv2.TargetTypeEnumInstance {
			for _, eniID := range controller.findENIsSupportingTargetGroupOfTypeInstance(instanceENIs, tg) {
				result[eniID] = true
			}
		} else {
			for _, eniID := range controller.findENIsSupportingTargetGroupOfTypeIP(instanceENIs, tg) {
				result[eniID] = true
			}
		}
	}
	return result
}

// findENIsSupportingTargetGroupOfTypeInstance find the ID of ENIs that are used to supporting ingress traffic to targetGroup with targetType instance.
// For targetType instance, traffic is routed into primary ENI of instance(eth0, i.e. decviceIndex == 0), other network interfaces are not used.
func (controller *instanceAttachmentController) findENIsSupportingTargetGroupOfTypeInstance(instanceENIs map[string][]*ec2.InstanceNetworkInterface, group tg.TargetGroup) (result []string) {
	targetInstanceIDs := make(map[string]bool)
	for _, endpoint := range group.Targets {
		targetInstanceIDs[aws.StringValue(endpoint.Id)] = true
	}
	for instanceID, enis := range instanceENIs {
		if _, ok := targetInstanceIDs[instanceID]; !ok {
			continue
		}
		for _, eni := range enis {
			if aws.Int64Value(eni.Attachment.DeviceIndex) == 0 {
				result = append(result, aws.StringValue(eni.NetworkInterfaceId))
			}
		}
	}
	return result
}

// findENIsSupportingTargetGroupOfTypeIP find the ID of ENIs that are used to supporting ingress traffic to targetGroup with targetType IP.
// For targetType IP, traffic is routed into the ENI for specific pod IPs.
// Warning: this function only works under CNI implementations that use ENI for pod IPs such as amazon k8s cni.
func (controller *instanceAttachmentController) findENIsSupportingTargetGroupOfTypeIP(instanceENIs map[string][]*ec2.InstanceNetworkInterface, group tg.TargetGroup) (result []string) {
	targetPodIPs := make(map[string]bool)
	for _, endpoint := range group.Targets {
		targetPodIPs[aws.StringValue(endpoint.Id)] = true
	}
	for _, enis := range instanceENIs {
		for _, eni := range enis {
			for _, addr := range eni.PrivateIpAddresses {
				if _, ok := targetPodIPs[aws.StringValue(addr.PrivateIpAddress)]; ok {
					result = append(result, aws.StringValue(eni.NetworkInterfaceId))
					break
				}
			}
		}
	}
	return result
}

// getClusterInstanceENIs retrives all ENIs attached to instances indexed by instanceID
func (controller *instanceAttachmentController) getClusterInstanceENIs() (map[string][]*ec2.InstanceNetworkInterface, error) {
	instanceIDs, err := controller.store.GetClusterInstanceIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance IDs within cluster due to %v", err)
	}
	instances, err := controller.cloud.GetInstancesByIDs(instanceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances within cluster due to %v", err)
	}
	result := make(map[string][]*ec2.InstanceNetworkInterface)
	for _, instance := range instances {
		result[aws.StringValue(instance.InstanceId)] = instance.NetworkInterfaces
	}
	return result, nil
}
