package ingress

import (
	"context"
	"github.com/pkg/errors"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
	"regexp"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
)

const (
	defaultGroupOrder int64 = 0
	minGroupOrder     int64 = 1
	maxGroupOder      int64 = 1000
)

var (
	// groupName must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character
	groupNameRegex = regexp.MustCompile("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
)

// GroupLoader loads Ingress groups.
type GroupLoader interface {
	// FindGroupID returns the Ingress groups's ID or nil if it doesn't belong to any group.
	FindGroupID(ctx context.Context, ing *networking.Ingress) (*GroupID, error)

	// Load returns an Ingress group given groupID.
	Load(ctx context.Context, groupID GroupID) (Group, error)
}

// NewDefaultGroupLoader constructs new GroupLoader instance.
func NewDefaultGroupLoader(client client.Client, annotationParser annotations.Parser, ingressClass string) *defaultGroupLoader {
	return &defaultGroupLoader{
		client:           client,
		annotationParser: annotationParser,
		ingressClass:     ingressClass,
	}
}

var _ GroupLoader = (*defaultGroupLoader)(nil)

type defaultGroupLoader struct {
	client           client.Client
	annotationParser annotations.Parser

	ingressClass string
}

func (m *defaultGroupLoader) FindGroupID(ctx context.Context, ing *networking.Ingress) (*GroupID, error) {
	if !m.matchesIngressClass(ing) {
		return nil, nil
	}

	groupName := ""
	if exists := m.annotationParser.ParseStringAnnotation(annotations.AnnotationSuffixGroupName, &groupName, ing.Annotations); exists {
		if err := validateGroupName(groupName); err != nil {
			return nil, err
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

	finalizer := buildGroupFinalizer(groupID)
	var members []*networking.Ingress
	var inactiveMembers []*networking.Ingress
	for index := range ingList.Items {
		ing := &ingList.Items[index]
		isGroupMember, err := m.isGroupMember(ctx, groupID, ing)
		if err != nil {
			return Group{}, err
		}
		if isGroupMember {
			members = append(members, ing)
		} else if k8s.HasFinalizer(ing, finalizer) {
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

// matchesIngressClass tests whether Ingress matches ingress class of this group manager.
func (m *defaultGroupLoader) matchesIngressClass(ing *networking.Ingress) bool {
	ingClass := ing.Annotations[annotations.AnnotationIngressClass]
	return ingClass == m.ingressClass
}

// isGroupMember checks whether an ingress is member of a Ingress group
func (m *defaultGroupLoader) isGroupMember(ctx context.Context, groupID GroupID, ing *networking.Ingress) (bool, error) {
	ingGroupID, err := m.FindGroupID(ctx, ing)
	if err != nil {
		return false, err
	}
	if ingGroupID == nil || (*ingGroupID) != groupID {
		return false, nil
	}

	return ing.DeletionTimestamp.IsZero(), nil
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
	ingressWithOrderList := make([]ingressWithOrder, 0, len(members))

	explicitOrders := sets.NewInt64()
	for _, ing := range members {
		var order = defaultGroupOrder
		exists, err := m.annotationParser.ParseInt64Annotation(annotations.AnnotationSuffixGroupOrder, &order, ing.Annotations)
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
	if groupNameRegex.MatchString(groupName) {
		return nil
	}
	return errors.New("groupName must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character")
}
