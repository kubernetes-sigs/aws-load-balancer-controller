package ingress

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	extensions "k8s.io/api/extensions/v1beta1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	defaultGroupOrder int64 = 0
	minGroupOrder           = 1
	maxGroupOrder           = 1000
)

type GroupBuilder interface {
	BuildGroupID(ctx context.Context, ing *extensions.Ingress) (GroupID, error)
	Build(ctx context.Context, groupID GroupID) (Group, error)
}

func NewGroupBuilder(cache cache.Cache, annotationParser k8s.AnnotationParser, ingressClass string) GroupBuilder {
	return &defaultGroupBuilder{
		cache:            cache,
		annotationParser: annotationParser,
		ingressClass:     ingressClass,
	}
}

type defaultGroupBuilder struct {
	cache            cache.Cache
	annotationParser k8s.AnnotationParser
	ingressClass     string
}

func (b *defaultGroupBuilder) BuildGroupID(_ context.Context, ing *extensions.Ingress) (GroupID, error) {
	groupName := ""
	if exists := b.annotationParser.ParseStringAnnotation(k8s.AnnotationSuffixGroupName, &groupName, ing.Annotations); exists {
		return NewGroupIDForClusterGroup(groupName), nil
	}
	return NewGroupIDForStandaloneGroup(k8s.NamespacedName(ing)), nil
}

func (b *defaultGroupBuilder) Build(ctx context.Context, groupID GroupID) (Group, error) {

	var members []*extensions.Ingress
	var err error
	if groupID.IsStandaloneGroup() {
		if members, err = b.loadMembersForStandaloneGroup(ctx, groupID); err != nil {
			return Group{}, err
		}
	} else {
		if members, err = b.loadMembersForClusterGroup(ctx, groupID); err != nil {
			return Group{}, err
		}
	}

	var activeMembers, leavingMembers []*extensions.Ingress
	for _, member := range members {
		if b.isGroupMemberLeaving(member) {
			leavingMembers = append(leavingMembers, member)
		} else {
			activeMembers = append(activeMembers, member)
		}
	}
	activeMembers, err = b.sortIngresses(activeMembers)
	if err != nil {
		return Group{}, err
	}

	return Group{ID: groupID, ActiveMembers: activeMembers, LeavingMembers: leavingMembers}, nil
}

func (b *defaultGroupBuilder) loadMembersForClusterGroup(ctx context.Context, groupID GroupID) ([]*extensions.Ingress, error) {
	ingList := &extensions.IngressList{}
	if err := b.cache.List(ctx, nil, ingList); err != nil {
		return nil, err
	}

	var members []*extensions.Ingress
	for index := range ingList.Items {
		ingGroupID, err := b.BuildGroupID(ctx, &ingList.Items[index])
		if err != nil {
			return nil, err
		}
		if ingGroupID != groupID {
			continue
		}
		members = append(members, &ingList.Items[index])
	}
	return members, nil
}

func (b *defaultGroupBuilder) loadMembersForStandaloneGroup(ctx context.Context, groupID GroupID) ([]*extensions.Ingress, error) {
	ing := &extensions.Ingress{}
	if err := b.cache.Get(ctx, types.NamespacedName{Namespace: groupID.namespace, Name: groupID.name}, ing); err != nil {
		if k8serr.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	ingGroupID, err := b.BuildGroupID(ctx, ing)
	if err != nil {
		return nil, err
	}
	if ingGroupID != groupID {
		return nil, nil
	}
	return []*extensions.Ingress{ing}, nil
}

func (b *defaultGroupBuilder) isGroupMemberLeaving(ing *extensions.Ingress) bool {
	if !ing.DeletionTimestamp.IsZero() {
		return true
	}
	if !MatchesIngressClass(b.ingressClass, ing) {
		return true
	}
	return false
}

type ingressWithOrder struct {
	ingress *extensions.Ingress
	order   int64
}

// sortIngresses will sort Ingresses within ingress group in ascending order.
//
// the order for an ingress can be set as below:
// * explicit denote the order via "group.order" annotation.(It's an error if two ingress have same explicit order)
// * implicit denote the order of MaxInt64.
func (m *defaultGroupBuilder) sortIngresses(ingList []*extensions.Ingress) ([]*extensions.Ingress, error) {
	ingressWithOrderList := make([]ingressWithOrder, 0, len(ingList))

	explicitOrders := sets.NewInt64()
	for _, ing := range ingList {
		var order = defaultGroupOrder;
		exists, err := m.annotationParser.ParseInt64Annotation(k8s.AnnotationSuffixGroupOrder, &order, ing.Annotations)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load ingress group order for ingress: %v", k8s.NamespacedName(ing))
		}
		if exists {
			if order < minGroupOrder || order > maxGroupOrder {
				return nil, errors.Errorf("explicit ingress group order must be within [%v:%v], order: %v, ingress: %v", minGroupOrder, maxGroupOrder, order, k8s.NamespacedName(ing))
			}

			if explicitOrders.Has(order) {
				return nil, errors.Errorf("conflict explicit ingress group order: %v", order)
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

	sortedIngresses := make([]*extensions.Ingress, 0, len(ingressWithOrderList))
	for _, node := range ingressWithOrderList {
		sortedIngresses = append(sortedIngresses, node.ingress)
	}
	return sortedIngresses, nil
}
