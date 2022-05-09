package ingress

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultGroupOrder  int64 = 0
	minGroupOrder      int64 = -1000
	maxGroupOder       int64 = 1000
	maxGroupNameLength int   = 63
)

var (
	// groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
	// groupName must be no more than 63 character.
	groupNameRegex = regexp.MustCompile("^([a-z0-9][-a-z0-9.]*)?[a-z0-9]$")

	// err represents that ingress group is invalid.
	errInvalidIngressGroup = errors.New("invalid ingress group")
)

// GroupLoader loads Ingress groups.
type GroupLoader interface {
	// Load returns an Ingress group given groupID.
	Load(ctx context.Context, groupID GroupID) (Group, error)

	// LoadGroupIDIfAny loads the groupID for Ingress if Ingress belong to any IngressGroup.
	// Ingresses that is not managed by this controller or in deletion state won't have a groupID.
	LoadGroupIDIfAny(ctx context.Context, ing *networking.Ingress) (*GroupID, error)

	// LoadGroupIDsPendingFinalization returns groupIDs that have associated finalizer on Ingress.
	LoadGroupIDsPendingFinalization(ctx context.Context, ing *networking.Ingress) []GroupID
}

// NewDefaultGroupLoader constructs new GroupLoader instance.
func NewDefaultGroupLoader(client client.Client, eventRecorder record.EventRecorder, annotationParser annotations.Parser, classLoader ClassLoader, classAnnotationMatcher ClassAnnotationMatcher, manageIngressesWithoutIngressClass bool) *defaultGroupLoader {
	return &defaultGroupLoader{
		client:           client,
		eventRecorder:    eventRecorder,
		annotationParser: annotationParser,

		classLoader:                        classLoader,
		classAnnotationMatcher:             classAnnotationMatcher,
		manageIngressesWithoutIngressClass: manageIngressesWithoutIngressClass,
	}
}

var _ GroupLoader = (*defaultGroupLoader)(nil)

// default implementation for GroupLoader
type defaultGroupLoader struct {
	client           client.Client
	eventRecorder    record.EventRecorder
	annotationParser annotations.Parser

	// classLoader loads IngressClass configurations for Ingress.
	classLoader ClassLoader

	// classAnnotationMatcher checks whether ingresses with "kubernetes.io/ingress.class" annotation should be managed.
	classAnnotationMatcher ClassAnnotationMatcher

	// manageIngressesWithoutIngressClass specifies whether ingresses without "kubernetes.io/ingress.class" annotation
	// and "spec.ingressClassName" should be managed or not.
	manageIngressesWithoutIngressClass bool
}

func (m *defaultGroupLoader) Load(ctx context.Context, groupID GroupID) (Group, error) {
	ingList := &networking.IngressList{}
	if err := m.client.List(ctx, ingList); err != nil {
		return Group{}, err
	}

	var members []ClassifiedIngress
	var inactiveMembers []*networking.Ingress
	finalizer := buildGroupFinalizer(groupID)
	for index := range ingList.Items {
		ing := &ingList.Items[index]
		classifiedIngress, isGroupMember, err := m.isGroupMember(ctx, groupID, ing)
		if err != nil {
			return Group{}, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing))
		}
		if isGroupMember {
			members = append(members, classifiedIngress)
		} else if m.containsGroupFinalizer(groupID, finalizer, ing) {
			inactiveMembers = append(inactiveMembers, ing)
		}
	}

	sortedMembers, err := m.sortGroupMembers(members)
	if err != nil {
		return Group{}, err
	}

	return Group{
		ID:              groupID,
		Members:         sortedMembers,
		InactiveMembers: inactiveMembers,
	}, nil
}

func (m *defaultGroupLoader) LoadGroupIDIfAny(ctx context.Context, ing *networking.Ingress) (*GroupID, error) {
	_, groupID, err := m.loadGroupIDIfAnyHelper(ctx, ing)
	return groupID, err
}

func (m *defaultGroupLoader) LoadGroupIDsPendingFinalization(_ context.Context, ing *networking.Ingress) []GroupID {
	var groupIDs []GroupID
	for _, finalizer := range ing.GetFinalizers() {
		if finalizer == implicitGroupFinalizer {
			groupIDs = append(groupIDs, NewGroupIDForImplicitGroup(k8s.NamespacedName(ing)))
		} else if strings.HasPrefix(finalizer, explicitGroupFinalizerPrefix) {
			groupName := finalizer[len(explicitGroupFinalizerPrefix):]
			groupIDs = append(groupIDs, NewGroupIDForExplicitGroup(groupName))
		}
	}
	return groupIDs
}

// isGroupMember checks whether specified Ingress is member of specific IngressGroup.
// If it's group member, a valid ClassifiedIngress will be returned as well.
// NOTE: this function should only error out when it's not certain whether the specified ingress is group member. (e.g. due to APIServer failures).
func (m *defaultGroupLoader) isGroupMember(ctx context.Context, groupID GroupID, ing *networking.Ingress) (ClassifiedIngress, bool, error) {
	classifiedIngress, ingGroupID, err := m.loadGroupIDIfAnyHelper(ctx, ing)
	if err != nil {
		if errors.Is(err, ErrInvalidIngressClass) || errors.Is(err, errInvalidIngressGroup) {
			return ClassifiedIngress{}, false, nil
		}
		return ClassifiedIngress{}, false, err
	}
	if ingGroupID == nil {
		return ClassifiedIngress{}, false, nil
	}
	groupIDMatches := groupID == *ingGroupID
	return classifiedIngress, groupIDMatches, nil
}

// loadGroupIDIfAnyHelper loads the groupID for Ingress if Ingress belong to any IngressGroup, along with the ClassifiedIngress object.
func (m *defaultGroupLoader) loadGroupIDIfAnyHelper(ctx context.Context, ing *networking.Ingress) (ClassifiedIngress, *GroupID, error) {
	// Ingress no longer belong to any IngressGroup when it's been deleted.
	if !ing.DeletionTimestamp.IsZero() {
		return ClassifiedIngress{}, nil, nil
	}
	classifiedIngress, matchesIngressClass, err := m.classifyIngress(ctx, ing)
	if err != nil {
		return ClassifiedIngress{}, nil, err
	}
	if !matchesIngressClass {
		return ClassifiedIngress{}, nil, nil
	}

	groupID, err := m.loadGroupID(classifiedIngress)
	if err != nil {
		return ClassifiedIngress{}, nil, err
	}
	return classifiedIngress, &groupID, nil
}

// classifyIngress will classify the Ingress resource and returns whether it should be managed by this controller, along with the ClassifiedIngress object.
func (m *defaultGroupLoader) classifyIngress(ctx context.Context, ing *networking.Ingress) (ClassifiedIngress, bool, error) {
	// the "kubernetes.io/ingress.class" annotation takes higher priority than "ingressClassName" field
	if ingClassAnnotation, exists := ing.Annotations[annotations.IngressClass]; exists {
		if matchesIngressClass := m.classAnnotationMatcher.Matches(ingClassAnnotation); matchesIngressClass {
			return ClassifiedIngress{
				Ing:            ing,
				IngClassConfig: ClassConfiguration{},
			}, true, nil
		}
		return ClassifiedIngress{
			Ing:            ing,
			IngClassConfig: ClassConfiguration{},
		}, false, nil
	}

	if ing.Spec.IngressClassName != nil {
		ingClassConfig, err := m.classLoader.Load(ctx, ing)
		if err != nil {
			return ClassifiedIngress{
				Ing:            ing,
				IngClassConfig: ClassConfiguration{},
			}, false, err
		}

		if matchesIngressClass := ingClassConfig.IngClass != nil && ingClassConfig.IngClass.Spec.Controller == ingressClassControllerALB; matchesIngressClass {
			return ClassifiedIngress{
				Ing:            ing,
				IngClassConfig: ingClassConfig,
			}, true, nil
		}
		return ClassifiedIngress{
			Ing:            ing,
			IngClassConfig: ingClassConfig,
		}, false, nil
	}

	return ClassifiedIngress{
		Ing:            ing,
		IngClassConfig: ClassConfiguration{},
	}, m.manageIngressesWithoutIngressClass, nil
}

// loadGroupID loads the groupID for classified Ingress.
func (m *defaultGroupLoader) loadGroupID(classifiedIng ClassifiedIngress) (GroupID, error) {
	// the "group" settings in associated IngClassParams takes higher priority than "group.name" annotation on Ingresses.
	if classifiedIng.IngClassConfig.IngClassParams != nil && classifiedIng.IngClassConfig.IngClassParams.Spec.Group != nil {
		groupName := classifiedIng.IngClassConfig.IngClassParams.Spec.Group.Name
		if err := validateGroupName(groupName); err != nil {
			return GroupID{}, fmt.Errorf("%w: %v", errInvalidIngressGroup, err.Error())
		}
		groupID := NewGroupIDForExplicitGroup(groupName)
		return groupID, nil
	}

	groupName := ""
	if exists := m.annotationParser.ParseStringAnnotation(annotations.IngressSuffixGroupName, &groupName, classifiedIng.Ing.Annotations); exists {
		if err := validateGroupName(groupName); err != nil {
			return GroupID{}, fmt.Errorf("%w: %v", errInvalidIngressGroup, err.Error())
		}
		groupID := NewGroupIDForExplicitGroup(groupName)
		return groupID, nil
	}

	groupID := NewGroupIDForImplicitGroup(k8s.NamespacedName(classifiedIng.Ing))
	return groupID, nil
}

func (m *defaultGroupLoader) containsGroupFinalizer(groupID GroupID, finalizer string, ing *networking.Ingress) bool {
	if groupID.IsExplicit() {
		return k8s.HasFinalizer(ing, finalizer)
	}

	ingImplicitGroupID := NewGroupIDForImplicitGroup(k8s.NamespacedName(ing))
	return ingImplicitGroupID == groupID && k8s.HasFinalizer(ing, finalizer)
}

type groupMemberWithOrder struct {
	member ClassifiedIngress
	order  int64
}

// sortGroupMembers will sort Ingresses within Ingress group in ascending order.
// the order for an ingress can be set as below:
// * explicit denote the order via "group.order" annotation.
// * implicit denote the order of ${defaultGroupOrder}.
// If two Ingress are of same order, they are sorted by lexical order of their full-qualified name.
func (m *defaultGroupLoader) sortGroupMembers(members []ClassifiedIngress) ([]ClassifiedIngress, error) {
	if len(members) == 0 {
		return nil, nil
	}

	groupMemberWithOrderList := make([]groupMemberWithOrder, 0, len(members))
	for _, member := range members {
		var order = defaultGroupOrder
		exists, err := m.annotationParser.ParseInt64Annotation(annotations.IngressSuffixGroupOrder, &order, member.Ing.Annotations)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load Ingress group order for ingress: %v", k8s.NamespacedName(member.Ing))
		}
		if exists {
			if order < minGroupOrder || order > maxGroupOder {
				return nil, errors.Errorf("explicit Ingress group order must be within [%v:%v], Ingress: %v, order: %v",
					minGroupOrder, maxGroupOder, k8s.NamespacedName(member.Ing), order)
			}
		}

		groupMemberWithOrderList = append(groupMemberWithOrderList, groupMemberWithOrder{member: member, order: order})
	}

	sort.Slice(groupMemberWithOrderList, func(i, j int) bool {
		orderI := groupMemberWithOrderList[i].order
		orderJ := groupMemberWithOrderList[j].order
		if orderI != orderJ {
			return orderI < orderJ
		}

		nameI := k8s.NamespacedName(groupMemberWithOrderList[i].member.Ing).String()
		nameJ := k8s.NamespacedName(groupMemberWithOrderList[j].member.Ing).String()
		return nameI < nameJ
	})

	sortedMembers := make([]ClassifiedIngress, 0, len(groupMemberWithOrderList))
	for _, item := range groupMemberWithOrderList {
		sortedMembers = append(sortedMembers, item.member)
	}
	return sortedMembers, nil
}

// validateGroupName validates whether Ingress group name is valid
func validateGroupName(groupName string) error {
	if !groupNameRegex.MatchString(groupName) {
		return errors.New("groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character")
	}
	if len(groupName) > maxGroupNameLength {
		return errors.Errorf("groupName must be no more than %v characters", maxGroupNameLength)
	}
	return nil
}
