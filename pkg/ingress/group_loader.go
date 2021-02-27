package ingress

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"regexp"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
)

const (
	defaultGroupOrder  int64 = 0
	minGroupOrder      int64 = 1
	maxGroupOder       int64 = 1000
	maxGroupNameLength int   = 63
	// the controller name used in IngressClass for ALB.
	ingressClassControllerALB = "ingress.k8s.aws/alb"
)

var (
	// groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
	// groupName must be no more than 63 character.
	groupNameRegex = regexp.MustCompile("^([a-z0-9][-a-z0-9.]*)?[a-z0-9]$")

	// err represents that ingress group is invalid.
	errInvalidIngressGroup = errors.New("invalid ingress group")
	// err represents that ingress class is invalid.
	errInvalidIngressClass = errors.New("invalid ingress class")
)

// GroupLoader loads Ingress groups.
type GroupLoader interface {
	// FindGroupID returns the IngressGroup's ID or nil if it doesn't belong to any group.
	FindGroupID(ctx context.Context, ing *networking.Ingress) (*GroupID, error)

	// Load returns an Ingress group given groupID.
	Load(ctx context.Context, groupID GroupID) (Group, error)
}

// NewDefaultGroupLoader constructs new GroupLoader instance.
func NewDefaultGroupLoader(client client.Client, eventRecorder record.EventRecorder, annotationParser annotations.Parser, classAnnotationMatcher ClassAnnotationMatcher, manageIngressesWithoutIngressClass bool) *defaultGroupLoader {
	return &defaultGroupLoader{
		client:           client,
		eventRecorder:    eventRecorder,
		annotationParser: annotationParser,

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

	// classAnnotationMatcher checks whether ingresses with "kubernetes.io/ingress.class" annotation should be managed.
	classAnnotationMatcher ClassAnnotationMatcher
	// manageIngressesWithoutIngressClass specifies whether ingresses without "kubernetes.io/ingress.class" annotation
	// and "spec.ingressClassName" should be managed or not.
	manageIngressesWithoutIngressClass bool
}

func (m *defaultGroupLoader) FindGroupID(ctx context.Context, ing *networking.Ingress) (*GroupID, error) {
	matchesIngressClass, err := m.matchesIngressClass(ctx, ing)
	if err != nil {
		return nil, err
	}
	if !matchesIngressClass {
		return nil, nil
	}

	groupName := ""
	if exists := m.annotationParser.ParseStringAnnotation(annotations.IngressSuffixGroupName, &groupName, ing.Annotations); exists {
		if err := validateGroupName(groupName); err != nil {
			return nil, fmt.Errorf("%w: %v", errInvalidIngressGroup, err.Error())
		}
		groupID := NewGroupIDForExplicitGroup(groupName)
		return &groupID, nil
	}

	groupID := NewGroupIDForImplicitGroup(k8s.NamespacedName(ing))
	return &groupID, nil
}

func (m *defaultGroupLoader) Load(ctx context.Context, groupID GroupID) (Group, error) {
	ingList := &networking.IngressList{}
	if err := m.client.List(ctx, ingList); err != nil {
		return Group{}, err
	}

	var members []*networking.Ingress
	var inactiveMembers []*networking.Ingress
	finalizer := buildGroupFinalizer(groupID)
	for index := range ingList.Items {
		ing := &ingList.Items[index]
		isGroupMember, err := m.isGroupMember(ctx, groupID, ing)
		if err != nil {
			return Group{}, errors.Wrapf(err, "ingress: %v", k8s.NamespacedName(ing))
		}
		if isGroupMember {
			members = append(members, ing)
		} else if m.containsGroupFinalizer(groupID, finalizer, ing) {
			inactiveMembers = append(inactiveMembers, ing)
		}
	}
	sortedMembers, err := m.sortGroupMembers(ctx, members)
	if err != nil {
		return Group{}, err
	}
	return Group{
		ID:              groupID,
		Members:         sortedMembers,
		InactiveMembers: inactiveMembers,
	}, nil
}

// matchesIngressClass tests whether provided Ingress are matched by this group loader.
func (m *defaultGroupLoader) matchesIngressClass(ctx context.Context, ing *networking.Ingress) (bool, error) {
	var matchesIngressClassResults []bool
	if ingClassAnnotation, exists := ing.Annotations[annotations.IngressClass]; exists {
		matchesIngressClass := m.classAnnotationMatcher.Matches(ingClassAnnotation)
		matchesIngressClassResults = append(matchesIngressClassResults, matchesIngressClass)
	}

	if ing.Spec.IngressClassName != nil {
		matchesIngressClass, err := m.matchesIngressClassName(ctx, *ing.Spec.IngressClassName)
		if err != nil {
			return false, err
		}
		matchesIngressClassResults = append(matchesIngressClassResults, matchesIngressClass)
	}

	if len(matchesIngressClassResults) == 2 {
		if matchesIngressClassResults[0] != matchesIngressClassResults[1] {
			m.eventRecorder.Event(ing, corev1.EventTypeWarning, k8s.IngressEventReasonConflictingIngressClass, "conflicting values for IngressClass by `spec.IngressClass` and `kubernetes.io/ingress.class` annotation")
		}
		return matchesIngressClassResults[0], nil
	}
	if len(matchesIngressClassResults) == 1 {
		return matchesIngressClassResults[0], nil
	}

	return m.manageIngressesWithoutIngressClass, nil
}

// matchesIngressClassName tests whether provided ingClassName are matched by this group loader.
func (m *defaultGroupLoader) matchesIngressClassName(ctx context.Context, ingClassName string) (bool, error) {
	ingClassKey := types.NamespacedName{Name: ingClassName}
	ingClass := &networking.IngressClass{}
	if err := m.client.Get(ctx, ingClassKey, ingClass); err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf("%w: %v", errInvalidIngressClass, err.Error())
		}
		return false, err
	}
	matchesIngressClass := ingClass.Spec.Controller == ingressClassControllerALB
	return matchesIngressClass, nil
}

// isGroupMember checks whether an ingress is member of a Ingress group
func (m *defaultGroupLoader) isGroupMember(ctx context.Context, groupID GroupID, ing *networking.Ingress) (bool, error) {
	ingGroupID, err := m.FindGroupID(ctx, ing)
	if err != nil {
		if errors.Is(err, errInvalidIngressGroup) || errors.Is(err, errInvalidIngressClass) {
			return false, nil
		}
		return false, err
	}

	if ingGroupID == nil || (*ingGroupID) != groupID {
		return false, nil
	}

	return ing.DeletionTimestamp.IsZero(), nil
}

func (m *defaultGroupLoader) containsGroupFinalizer(groupID GroupID, finalizer string, ing *networking.Ingress) bool {
	if groupID.IsExplicit() {
		return k8s.HasFinalizer(ing, finalizer)
	}

	ingImplicitGroupID := NewGroupIDForImplicitGroup(k8s.NamespacedName(ing))
	return ingImplicitGroupID == groupID && k8s.HasFinalizer(ing, finalizer)
}

type ingressWithOrder struct {
	ingress *networking.Ingress
	order   int64
}

// sortGroupMembers will sort Ingresses within Ingress group in ascending order.
// the order for an ingress can be set as below:
// * explicit denote the order via "group.order" annotation.(It's an error if two Ingress have same explicit order)
// * implicit denote the order of ${defaultGroupOrder}.
// If two Ingress are of same order, they are sorted by lexical order of their full-qualified name.
func (m *defaultGroupLoader) sortGroupMembers(ctx context.Context, members []*networking.Ingress) ([]*networking.Ingress, error) {
	if len(members) == 0 {
		return nil, nil
	}

	ingressWithOrderList := make([]ingressWithOrder, 0, len(members))
	explicitOrders := sets.NewInt64()
	for _, ing := range members {
		var order = defaultGroupOrder
		exists, err := m.annotationParser.ParseInt64Annotation(annotations.IngressSuffixGroupOrder, &order, ing.Annotations)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load Ingress group order for ingress: %v", k8s.NamespacedName(ing))
		}
		if exists {
			if order < minGroupOrder || order > maxGroupOder {
				return nil, errors.Errorf("explicit Ingress group order must be within [%v:%v], Ingress: %v, order: %v",
					minGroupOrder, maxGroupOder, k8s.NamespacedName(ing), order)
			}
			if explicitOrders.Has(order) {
				return nil, errors.Errorf("conflict Ingress group order: %v", order)
			}
			explicitOrders.Insert(order)
		}

		ingressWithOrderList = append(ingressWithOrderList, ingressWithOrder{ingress: ing, order: order})
	}

	sort.Slice(ingressWithOrderList, func(i, j int) bool {
		orderI := ingressWithOrderList[i].order
		orderJ := ingressWithOrderList[j].order
		if orderI != orderJ {
			return orderI < orderJ
		}

		nameI := k8s.NamespacedName(ingressWithOrderList[i].ingress).String()
		nameJ := k8s.NamespacedName(ingressWithOrderList[j].ingress).String()
		return nameI < nameJ
	})

	sortedIngresses := make([]*networking.Ingress, 0, len(ingressWithOrderList))
	for _, item := range ingressWithOrderList {
		sortedIngresses = append(sortedIngresses, item.ingress)
	}
	return sortedIngresses, nil
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
