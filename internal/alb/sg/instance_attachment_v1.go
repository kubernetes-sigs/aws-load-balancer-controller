package sg

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

func NewInstanceAttachmentControllerV1(
	sgController SecurityGroupController,
	targetENIsResolver TargetENIsResolver,
	nameTagGen NameTagGenerator,
	store store.Storer,
	cloud aws.CloudAPI) InstanceAttachmentController {

	return &instanceAttachmentControllerV1{
		sgController:       sgController,
		targetENIsResolver: targetENIsResolver,
		nameTagGen:         nameTagGen,
		store:              store,
		cloud:              cloud,
	}
}

// instanceAttachmentControllerV1 will use new securityGroup that allows traffic from LB securityGroup,
// and attach this new securityGroup to ENIs.
type instanceAttachmentControllerV1 struct {
	sgController       SecurityGroupController
	targetENIsResolver TargetENIsResolver
	nameTagGen         NameTagGenerator

	store store.Storer
	cloud aws.CloudAPI
}

var clusterInstanceENILock = &sync.Mutex{}

func (c *instanceAttachmentControllerV1) Reconcile(ctx context.Context, ingKey types.NamespacedName, lbSGID string,
	tgGroup tg.TargetGroupGroup) error {

	ingAnnos, err := c.store.GetIngressAnnotations(ingKey.String())
	if err != nil {
		return err
	}
	instanceSGID, err := c.ensureInstanceSG(ctx, ingKey, lbSGID, ingAnnos.Tags.LoadBalancer)
	if err != nil {
		return errors.Wrap(err, "failed to reconcile instance securityGroup")
	}

	// We lock here because the security groups on ENI doesn't support add/remove a single group but only replace whole list of SGs.
	// So that we don't need to lock when we attach/detach ENIs.
	// lock is cheaper than aws api calls, but we are assuming that no-other external component is modifying ENI sg at same time :D
	clusterInstanceENILock.Lock()
	defer clusterInstanceENILock.Unlock()
	targetENIs, err := c.targetENIsResolver.Resolve(ctx, tgGroup)
	if err != nil {
		return err
	}
	targetENIIDs := sets.StringKeySet(targetENIs)

	attachedENIs, err := c.findENIsAttachedWithInstanceSG(ctx, instanceSGID)
	if err != nil {
		return err
	}
	attachedENIIDs := sets.StringKeySet(attachedENIs)

	shouldAttachENIIDs := targetENIIDs.Difference(attachedENIIDs)
	for eniID := range shouldAttachENIIDs {
		if err := c.ensureSGAttachedToENI(ctx, instanceSGID, eniID, targetENIs[eniID]); err != nil {
			return err
		}
	}

	shouldDetachENIIDs := attachedENIIDs.Difference(targetENIIDs)
	for eniID := range shouldDetachENIIDs {
		if err := c.ensureSGDetachedFromENI(ctx, instanceSGID, eniID, attachedENIs[eniID]); err != nil {
			return err
		}
	}
	return nil
}

func (c *instanceAttachmentControllerV1) Delete(ctx context.Context, ingKey types.NamespacedName) error {
	sgName := c.nameTagGen.NameInstanceSG(ingKey.Namespace, ingKey.Name)
	sgInstance, err := c.cloud.GetSecurityGroupByName(sgName)
	if err != nil {
		return err
	}
	if sgInstance == nil {
		return nil
	}
	instanceSGID := aws.StringValue(sgInstance.GroupId)
	attachedENIs, err := c.findENIsAttachedWithInstanceSG(ctx, instanceSGID)
	if err != nil {
		return err
	}
	for eniID, eniInfo := range attachedENIs {
		if err := c.ensureSGDetachedFromENI(ctx, instanceSGID, eniID, eniInfo); err != nil {
			return err
		}
	}
	albctx.GetLogger(ctx).Infof("deleting securityGroup %v:%v", aws.StringValue(sgInstance.GroupName), aws.StringValue(sgInstance.Description))
	return c.cloud.DeleteSecurityGroupByID(ctx, aws.StringValue(sgInstance.GroupId))
}

func (c *instanceAttachmentControllerV1) ensureInstanceSG(ctx context.Context, ingKey types.NamespacedName, lbSGID string, additionalTags map[string]string) (string, error) {
	sgName := c.nameTagGen.NameInstanceSG(ingKey.Namespace, ingKey.Name)
	sgTags := c.nameTagGen.TagInstanceSG(ingKey.Namespace, ingKey.Name)
	for k, v := range additionalTags {
		sgTags[k] = v
	}
	inboundPermissions := []*ec2.IpPermission{
		{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(0),
			ToPort:     aws.Int64(65535),
			UserIdGroupPairs: []*ec2.UserIdGroupPair{
				{
					GroupId: aws.String(lbSGID),
				},
			},
		},
	}

	sgInstance, err := c.sgController.EnsureSGInstanceByName(ctx, sgName, "managed instance securityGroup by ALB Ingress Controller")
	if err != nil {
		return "", err
	}
	if err := c.sgController.Reconcile(ctx, sgInstance, inboundPermissions, sgTags); err != nil {
		return "", err
	}
	return aws.StringValue(sgInstance.GroupId), nil
}

// findENIsAttachedWithInstanceSG finds all ENIs attached with instance SG.
func (c *instanceAttachmentControllerV1) findENIsAttachedWithInstanceSG(ctx context.Context, instanceSGID string) (map[string]ENIInfo, error) {
	enis, err := c.cloud.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("group-id"),
				Values: []*string{aws.String(instanceSGID)},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]ENIInfo, len(enis))
	for _, eni := range enis {
		result[aws.StringValue(eni.NetworkInterfaceId)] = NewENIInfoViaENI(eni)
	}
	return result, nil
}

func (c *instanceAttachmentControllerV1) ensureSGAttachedToENI(ctx context.Context, sgID string, eniID string, eniInfo ENIInfo) error {
	desiredGroups := []string{sgID}
	for _, groupID := range eniInfo.SecurityGroups() {
		if groupID == sgID {
			return nil
		}
		desiredGroups = append(desiredGroups, groupID)
	}

	albctx.GetLogger(ctx).Infof("attaching securityGroup %s to ENI %s", sgID, eniID)
	_, err := c.cloud.ModifyNetworkInterfaceAttributeWithContext(ctx, &ec2.ModifyNetworkInterfaceAttributeInput{
		NetworkInterfaceId: aws.String(eniID),
		Groups:             aws.StringSlice(desiredGroups),
	})
	return err
}

func (c *instanceAttachmentControllerV1) ensureSGDetachedFromENI(ctx context.Context, sgID string, eniID string, eniInfo ENIInfo) error {
	sgAttached := false
	desiredGroups := []string{}
	for _, groupID := range eniInfo.SecurityGroups() {
		if groupID == sgID {
			sgAttached = true
		} else {
			desiredGroups = append(desiredGroups, groupID)
		}
	}
	if !sgAttached {
		return nil
	}

	albctx.GetLogger(ctx).Infof("detaching securityGroup %s from ENI %s", sgID, eniID)
	_, err := c.cloud.ModifyNetworkInterfaceAttributeWithContext(ctx, &ec2.ModifyNetworkInterfaceAttributeInput{
		NetworkInterfaceId: aws.String(eniID),
		Groups:             aws.StringSlice(desiredGroups),
	})
	return err
}
