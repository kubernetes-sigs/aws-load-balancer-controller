package targetgroupbinding

import (
	"context"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/algorithm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/backend"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

const (
	testNamespace = "test-ns"
	testTGBName   = "test-tgb"
)

func TestUpdateTrackedTargets(t *testing.T) {
	testCases := []struct {
		name            string
		updateRequested bool
		endpoints       []string
		expectedCache   sets.Set[string]
		cachedValue     sets.Set[string]
		multiTg         bool
		validateCM      bool
	}{
		{
			name:            "update requested tgb not shared",
			updateRequested: true,
			cachedValue:     nil,
			expectedCache:   nil,
			endpoints:       []string{},
		},
		{
			name:            "update not requested tgb not shared",
			updateRequested: false,
			cachedValue:     nil,
			expectedCache:   nil,
			endpoints:       []string{},
		},
		{
			name:            "update not requested tgb shared should still backfill",
			updateRequested: false,
			cachedValue:     nil,
			expectedCache:   sets.Set[string]{},
			endpoints:       []string{},
			multiTg:         true,
			validateCM:      true,
		},
		{
			name:            "update not requested tgb shared should not backfill as the cache has a value already",
			updateRequested: false,
			cachedValue: sets.Set[string]{
				"127.0.0.1:80": {},
			},
			expectedCache: sets.Set[string]{
				"127.0.0.1:80": {},
			},
			endpoints: []string{},
			multiTg:   true,
		},
		{
			name:            "update requested tgb shared empty endpoints",
			updateRequested: true,
			cachedValue:     nil,
			endpoints:       []string{},
			expectedCache:   sets.Set[string]{},
			multiTg:         true,
			validateCM:      true,
		},
		{
			name:            "update not requested tgb shared should still backfill with endpoints",
			updateRequested: false,
			endpoints: []string{
				"127.0.0.1:80",
				"127.0.0.2:80",
				"127.0.0.3:80",
				"127.0.0.4:80",
				"127.0.0.5:80",
			},
			cachedValue: nil,
			expectedCache: sets.Set[string]{
				"127.0.0.1:80": {},
				"127.0.0.2:80": {},
				"127.0.0.3:80": {},
				"127.0.0.4:80": {},
				"127.0.0.5:80": {},
			},
			multiTg:    true,
			validateCM: true,
		},
		{
			name:            "update requested tgb shared with endpoints",
			updateRequested: true,
			endpoints: []string{
				"127.0.0.1:80",
				"127.0.0.2:80",
				"127.0.0.3:80",
				"127.0.0.4:80",
				"127.0.0.5:80",
			},
			cachedValue: nil,
			expectedCache: sets.Set[string]{
				"127.0.0.1:80": {},
				"127.0.0.2:80": {},
				"127.0.0.3:80": {},
				"127.0.0.4:80": {},
				"127.0.0.5:80": {},
			},
			multiTg:    true,
			validateCM: true,
		},
		{
			name:            "update requested tgb shared with endpoints. endpoints from different ports",
			updateRequested: true,
			endpoints: []string{
				"127.0.0.1:80",
				"127.0.0.2:80",
				"127.0.0.3:80",
				"127.0.0.4:80",
				"127.0.0.5:80",
				"127.0.0.1:85",
				"127.0.0.2:85",
				"127.0.0.3:85",
				"127.0.0.4:85",
				"127.0.0.5:85",
			},
			cachedValue: nil,
			expectedCache: sets.Set[string]{
				"127.0.0.1:80": {},
				"127.0.0.2:80": {},
				"127.0.0.3:80": {},
				"127.0.0.4:80": {},
				"127.0.0.5:80": {},
				"127.0.0.1:85": {},
				"127.0.0.2:85": {},
				"127.0.0.3:85": {},
				"127.0.0.4:85": {},
				"127.0.0.5:85": {},
			},
			multiTg:    true,
			validateCM: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testclient.NewClientBuilder().Build()
			mc := NewMultiClusterManager(k8sClient, k8sClient, logr.New(&log.NullLogSink{})).(*multiClusterManagerImpl)

			tgb := &elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      testTGBName,
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					MultiClusterTargetGroup: tc.multiTg,
				},
			}

			if tc.cachedValue != nil {
				setCachedValue(mc, tc.cachedValue, testNamespace, testTGBName)
			}

			err := mc.updateTrackedTargets(context.Background(), tc.updateRequested, func() []string {
				return tc.endpoints
			}, tgb)
			assert.Nil(t, err)
			cachedValue := getCachedValue(mc, testNamespace, testTGBName)
			assert.Equal(t, tc.expectedCache, cachedValue)
			if tc.validateCM {
				cm := &corev1.ConfigMap{}
				k8sClient.Get(context.Background(), client.ObjectKey{
					Namespace: tgb.Namespace,
					Name:      getConfigMapName(tgb),
				}, cm)
				assert.Equal(t, tc.expectedCache, algorithm.CSVToStringSet(cm.Data[targetsKey]))
			}
		})
	}
}

func TestUpdateTrackedTargetsUpdateConfigMap(t *testing.T) {
	k8sClient := testclient.NewClientBuilder().Build()
	reader := testclient.NewClientBuilder().Build()
	mc := NewMultiClusterManager(k8sClient, reader, logr.New(&log.NullLogSink{})).(*multiClusterManagerImpl)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testTGBName,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			MultiClusterTargetGroup: true,
		},
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: tgb.Namespace,
			Name:      getConfigMapName(tgb),
		},
		Data: map[string]string{
			targetsKey: algorithm.StringSetToCSV(sets.Set[string]{}),
		},
	}
	k8sClient.Create(context.Background(), cm)

	endpoints := []string{"127.0.0.1:80"}
	endpointsFn := func() []string {
		return endpoints
	}

	err := mc.updateTrackedTargets(context.Background(), true, endpointsFn, tgb)
	assert.Nil(t, err)
	cachedValue := getCachedValue(mc, testNamespace, testTGBName)
	assert.Equal(t, sets.Set[string]{
		"127.0.0.1:80": {},
	}, cachedValue)

	cm = &corev1.ConfigMap{}
	k8sGetError := k8sClient.Get(context.Background(), client.ObjectKey{
		Namespace: tgb.Namespace,
		Name:      getConfigMapName(tgb),
	}, cm)
	assert.Nil(t, k8sGetError)
	assert.Equal(t, sets.Set[string]{
		"127.0.0.1:80": {},
	}, algorithm.CSVToStringSet(cm.Data[targetsKey]))

}

func TestUpdateTrackedIPTargets(t *testing.T) {
	k8sClient := testclient.NewClientBuilder().Build()
	reader := testclient.NewClientBuilder().Build()
	mc := NewMultiClusterManager(k8sClient, reader, logr.New(&log.NullLogSink{})).(*multiClusterManagerImpl)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testTGBName,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			MultiClusterTargetGroup: true,
		},
	}

	endpoints := []backend.PodEndpoint{
		{
			IP:   "127.0.0.1",
			Port: 80,
		},
		{
			IP:   "127.0.0.2",
			Port: 80,
		},
		{
			IP:   "127.0.0.3",
			Port: 80,
		},
	}

	err := mc.UpdateTrackedIPTargets(context.Background(), true, endpoints, tgb)
	assert.Nil(t, err)
	cachedValue := getCachedValue(mc, testNamespace, testTGBName)
	assert.Equal(t, sets.Set[string]{
		"127.0.0.1:80": {},
		"127.0.0.2:80": {},
		"127.0.0.3:80": {},
	}, cachedValue)
}

func TestUpdateTrackedInstanceTargets(t *testing.T) {
	k8sClient := testclient.NewClientBuilder().Build()
	reader := testclient.NewClientBuilder().Build()
	mc := NewMultiClusterManager(k8sClient, reader, logr.New(&log.NullLogSink{})).(*multiClusterManagerImpl)

	tgb := &elbv2api.TargetGroupBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testTGBName,
		},
		Spec: elbv2api.TargetGroupBindingSpec{
			MultiClusterTargetGroup: true,
		},
	}

	endpoints := []backend.NodePortEndpoint{
		{
			InstanceID: "i-1234",
			Port:       80,
		},
		{
			InstanceID: "i-5678",
			Port:       80,
		},
		{
			InstanceID: "i-91011",
			Port:       80,
		},
	}

	err := mc.UpdateTrackedInstanceTargets(context.Background(), true, endpoints, tgb)
	assert.Nil(t, err)
	cachedValue := getCachedValue(mc, testNamespace, testTGBName)
	assert.Equal(t, sets.Set[string]{
		"i-1234:80":  {},
		"i-5678:80":  {},
		"i-91011:80": {},
	}, cachedValue)
}

func TestFilterTargetsForDeregistration(t *testing.T) {

	cachedTargets := sets.Set[string]{
		"127.0.0.1:80": {},
		"127.0.0.2:80": {},
		"127.0.0.3:80": {},
		"127.0.0.4:80": {},
		"127.0.0.5:80": {},
	}

	testCases := []struct {
		name            string
		multiTg         bool
		setData         bool
		targets         []TargetInfo
		expectedTargets []TargetInfo
		cachedTargets   sets.Set[string]
		refreshNeeded   bool
	}{
		{
			name: "multicluster not enabled",
			targets: []TargetInfo{
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.100"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.101"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.102"),
						Port: awssdk.Int32(80),
					},
				},
			},
			expectedTargets: []TargetInfo{
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.100"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.101"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.102"),
						Port: awssdk.Int32(80),
					},
				},
			},
		},
		{
			name:            "multicluster enabled, need to refresh config map",
			multiTg:         true,
			refreshNeeded:   true,
			targets:         []TargetInfo{},
			expectedTargets: []TargetInfo{},
		},
		{
			name:    "multicluster enabled, all targets filtered out",
			setData: true,
			multiTg: true,
			targets: []TargetInfo{
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.100"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.101"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.102"),
						Port: awssdk.Int32(80),
					},
				},
			},
			expectedTargets: []TargetInfo{},
		},
		{
			name:    "multicluster enabled, some targets filtered out",
			setData: true,
			multiTg: true,
			targets: []TargetInfo{
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.100"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.101"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.102"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.1"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.2"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.3"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.4"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.5"),
						Port: awssdk.Int32(80),
					},
				},
			},
			expectedTargets: []TargetInfo{
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.1"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.2"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.3"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.4"),
						Port: awssdk.Int32(80),
					},
				},
				{
					Target: elbv2types.TargetDescription{
						Id:   awssdk.String("127.0.0.5"),
						Port: awssdk.Int32(80),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testclient.NewClientBuilder().Build()
			reader := testclient.NewClientBuilder().Build()
			mc := NewMultiClusterManager(k8sClient, reader, logr.New(&log.NullLogSink{})).(*multiClusterManagerImpl)
			tgb := &elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      testTGBName,
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					MultiClusterTargetGroup: tc.multiTg,
				},
			}

			if tc.setData {
				setCachedValue(mc, cachedTargets, testNamespace, testTGBName)
			}

			returnedTargets, refreshNeeded, err := mc.FilterTargetsForDeregistration(context.Background(), tgb, tc.targets)
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedTargets, returnedTargets)
			assert.Equal(t, tc.refreshNeeded, refreshNeeded)
		})
	}
}

func TestGetConfigMapContents(t *testing.T) {

	inMemoryCache := sets.Set[string]{
		"1": {},
		"2": {},
		"3": {},
	}

	cmData := sets.Set[string]{
		"4": {},
		"5": {},
		"6": {},
	}

	testCases := []struct {
		name     string
		setCache bool
		expected sets.Set[string]
	}{
		{
			name:     "use cached value",
			setCache: true,
			expected: inMemoryCache,
		},
		{
			name:     "use cm value",
			expected: cmData,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testclient.NewClientBuilder().Build()
			mc := NewMultiClusterManager(k8sClient, k8sClient, logr.New(&log.NullLogSink{})).(*multiClusterManagerImpl)
			tgb := &elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      testTGBName,
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					MultiClusterTargetGroup: true,
				},
			}

			if tc.setCache {
				setCachedValue(mc, inMemoryCache, testNamespace, testTGBName)
			}

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: tgb.Namespace,
					Name:      getConfigMapName(tgb),
				},
				Data: map[string]string{
					targetsKey: algorithm.StringSetToCSV(cmData),
				},
			}
			k8sClient.Create(context.Background(), cm)

			res, err := mc.getConfigMapContents(context.Background(), tgb)
			assert.Nil(t, err)
			assert.Equal(t, tc.expected, res)

			cachedValue := getCachedValue(mc, testNamespace, testTGBName)
			assert.Equal(t, cachedValue, res)
		})
	}
}

func TestCleanUp(t *testing.T) {
	testCases := []struct {
		name    string
		multiTg bool
		setData bool
	}{
		{
			name: "multicluster not enabled",
		},
		{
			name:    "multicluster enabled, data not set",
			multiTg: true,
		},
		{
			name:    "multicluster enabled",
			multiTg: true,
			setData: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testclient.NewClientBuilder().Build()
			reader := testclient.NewClientBuilder().Build()
			mc := NewMultiClusterManager(k8sClient, reader, logr.New(&log.NullLogSink{})).(*multiClusterManagerImpl)
			tgb := &elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      testTGBName,
				},
				Spec: elbv2api.TargetGroupBindingSpec{
					MultiClusterTargetGroup: tc.multiTg,
				},
			}

			otherNs := "otherns"
			otherName := "othername"

			if tc.setData {
				cachedValue := sets.Set[string]{
					"foo": {},
				}
				setCachedValue(mc, cachedValue, testNamespace, testTGBName)

				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: tgb.Namespace,
						Name:      getConfigMapName(tgb),
					},
					Data: map[string]string{
						targetsKey: algorithm.StringSetToCSV(cachedValue),
					},
				}
				k8sClient.Create(context.Background(), cm)
			}

			otherCacheValue := sets.Set[string]{
				"baz": {},
			}
			setCachedValue(mc, otherCacheValue, otherNs, otherName)

			err := mc.CleanUp(context.Background(), tgb)
			assert.Nil(t, err)
			assert.Nil(t, getCachedValue(mc, testNamespace, testTGBName))
			assert.Equal(t, otherCacheValue, getCachedValue(mc, otherNs, otherName))
			cm := &corev1.ConfigMap{}
			k8sGetError := k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: tgb.Namespace,
				Name:      getConfigMapName(tgb),
			}, cm)
			assert.NotNil(t, k8sGetError)
			assert.Nil(t, client.IgnoreNotFound(k8sGetError))
		})
	}
}

func getCachedValue(mc *multiClusterManagerImpl, namespace, name string) sets.Set[string] {
	key := fmt.Sprintf("%s-%s", namespace, name)

	if v, ok := mc.configMapCache[key]; !ok {
		return nil
	} else {
		return v
	}
}

func setCachedValue(mc *multiClusterManagerImpl, v sets.Set[string], namespace, name string) {
	key := fmt.Sprintf("%s-%s", namespace, name)
	mc.configMapCache[key] = v
}
