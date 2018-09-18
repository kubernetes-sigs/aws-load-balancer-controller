package sg

import (
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// InstanceAttachment represents the attachment of securityGroups to instance
type InstanceAttachment struct {
	GroupID string
	Targets tg.TargetGroups
}

// InstanceAttachementController manages InstanceAttachment
type InstanceAttachementController interface {
	Reconcile(*InstanceAttachment) error
	Delete(*InstanceAttachment) error
}

type instanceAttachmentController struct {
	store  store.Storer
	ec2    *albec2.EC2
	logger *log.Logger
}

var clusterInstanceENILock = &sync.Mutex{}

func (controller *instanceAttachmentController) Reconcile(attachment *InstanceAttachment) error {
	clusterInstanceENILock.Lock()
	defer clusterInstanceENILock.Unlock()
	instanceENIs, err := controller.getClusterInstanceENIs()
	if err != nil {
		return fmt.Errorf("failed to get cluster enis due to %s", err.Error())
	}
	supportingENIs := controller.findENIsSupportingTargets(instanceENIs, attachment.Targets)
	for _, enis := range instanceENIs {
		for _, eni := range enis {
			if _, ok := supportingENIs[aws.StringValue(eni.NetworkInterfaceId)]; ok {
				err := controller.ensureSGAttachedToENI(attachment.GroupID, eni)
				if err != nil {
					return err
				}
			} else {
				err := controller.ensureSGDetachedFromENI(attachment.GroupID, eni)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (controller *instanceAttachmentController) Delete(attachment *InstanceAttachment) error {
	clusterInstanceENILock.Lock()
	defer clusterInstanceENILock.Unlock()
	instanceENIs, err := controller.getClusterInstanceENIs()
	if err != nil {
		return fmt.Errorf("failed to get cluster enis, Error:%s", err.Error())
	}
	for _, enis := range instanceENIs {
		for _, eni := range enis {
			err := controller.ensureSGDetachedFromENI(attachment.GroupID, eni)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (controller *instanceAttachmentController) ensureSGAttachedToENI(sgID string, eni *ec2.InstanceNetworkInterface) error {
	desiredGroups := []string{sgID}
	for _, group := range eni.Groups {
		groupID := aws.StringValue(group.GroupId)
		if groupID == sgID {
			return nil
		}
		desiredGroups = append(desiredGroups, groupID)
	}

	controller.logger.Infof("attaching securityGroup %s to ENI %s", sgID, *eni.NetworkInterfaceId)
	_, err := controller.ec2.ModifyNetworkInterfaceAttribute(&ec2.ModifyNetworkInterfaceAttributeInput{
		NetworkInterfaceId: eni.NetworkInterfaceId,
		Groups:             aws.StringSlice(desiredGroups),
	})
	return err
}

func (controller *instanceAttachmentController) ensureSGDetachedFromENI(sgID string, eni *ec2.InstanceNetworkInterface) error {
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

	controller.logger.Infof("detaching securityGroup %s from ENI %s", sgID, *eni.NetworkInterfaceId)
	_, err := controller.ec2.ModifyNetworkInterfaceAttribute(&ec2.ModifyNetworkInterfaceAttributeInput{
		NetworkInterfaceId: eni.NetworkInterfaceId,
		Groups:             aws.StringSlice(desiredGroups),
	})
	return err
}

//findENIsSupportingTargets find the ID of ENIs that are used to supporting ingress traffic to targets
func (controller *instanceAttachmentController) findENIsSupportingTargets(instanceENIs map[string][]*ec2.InstanceNetworkInterface, targets tg.TargetGroups) map[string]bool {
	result := make(map[string]bool)
	for _, group := range targets {
		if group.TargetType == elbv2.TargetTypeEnumInstance {
			for _, eniID := range controller.findENIsSupportingTargetGroupOfTypeInstance(instanceENIs, group) {
				result[eniID] = true
			}
		} else {
			for _, eniID := range controller.findENIsSupportingTargetGroupOfTypeIP(instanceENIs, group) {
				result[eniID] = true
			}
		}
	}
	return result
}

// findENIsSupportingTargetGroupOfTypeInstance find the ID of ENIs that are used to supporting ingress traffic to targetGroup with targetType instance.
// For targetType instance, traffic is routed into primary ENI of instance(eth0, i.e. decviceIndex == 0), other network interfaces are not used.
func (controller *instanceAttachmentController) findENIsSupportingTargetGroupOfTypeInstance(instanceENIs map[string][]*ec2.InstanceNetworkInterface, group *tg.TargetGroup) (result []string) {
	targetInstanceIDs := make(map[string]bool)
	for _, endpoint := range group.DesiredTargets() {
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
func (controller *instanceAttachmentController) findENIsSupportingTargetGroupOfTypeIP(instanceENIs map[string][]*ec2.InstanceNetworkInterface, group *tg.TargetGroup) (result []string) {
	targetPodIPs := make(map[string]bool)
	for _, endpoint := range group.DesiredTargets() {
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
	instanceIDs, err := controller.getClusterInstanceIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance IDs within cluster, Error:%s", err.Error())
	}
	instances, err := controller.ec2.GetInstancesByID(instanceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances within cluster, Error:%s", err.Error())
	}
	result := make(map[string][]*ec2.InstanceNetworkInterface)
	for _, instance := range instances {
		result[aws.StringValue(instance.InstanceId)] = instance.NetworkInterfaces
	}
	return result, nil
}

// getClusterInstanceIDs retrives the aws instanceIDs in k8s cluster
func (controller *instanceAttachmentController) getClusterInstanceIDs() (result []string, err error) {
	for _, node := range controller.store.ListNodes() {
		instanceID, err := controller.store.GetNodeInstanceID(node)
		if err != nil {
			return nil, err
		}
		result = append(result, instanceID)
	}
	return result, nil
}
