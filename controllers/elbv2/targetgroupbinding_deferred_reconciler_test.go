package controllers

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"testing"
	"time"
)

func TestDeferredReconcilerConstructor(t *testing.T) {
	dq := workqueue.NewDelayingQueue()
	defer dq.ShutDown()
	syncPeriod := 5 * time.Minute
	k8sClient := testclient.NewClientBuilder().Build()
	logger := logr.New(&log.NullLogSink{})

	d := NewDeferredTargetGroupBindingReconciler(dq, syncPeriod, k8sClient, logger)

	deferredReconciler := d.(*deferredTargetGroupBindingReconcilerImpl)
	assert.Equal(t, dq, deferredReconciler.delayQueue)
	assert.Equal(t, syncPeriod, deferredReconciler.syncPeriod)
	assert.Equal(t, k8sClient, deferredReconciler.k8sClient)
	assert.Equal(t, logger, deferredReconciler.logger)
}

func TestDeferredReconcilerEnqueue(t *testing.T) {
	syncPeriod := 5 * time.Minute
	testCases := []struct {
		name                 string
		tgbToEnqueue         []*elbv2api.TargetGroupBinding
		expectedQueueEntries sets.Set[types.NamespacedName]
	}{
		{
			name: "one tgb to enqueue",
			tgbToEnqueue: []*elbv2api.TargetGroupBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
					},
				},
			},
			expectedQueueEntries: sets.New(types.NamespacedName{
				Name:      "tgb1",
				Namespace: "ns",
			}),
		},
		{
			name: "sync period too short, do not enqueue",
			tgbToEnqueue: []*elbv2api.TargetGroupBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
						Annotations: map[string]string{
							annotations.AnnotationCheckPointTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
						},
					},
				},
			},
			expectedQueueEntries: make(sets.Set[types.NamespacedName]),
		},
		{
			name: "sync period too long, do enqueue",
			tgbToEnqueue: []*elbv2api.TargetGroupBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
						Annotations: map[string]string{
							annotations.AnnotationCheckPointTimestamp: strconv.FormatInt(time.Now().Add(-2*syncPeriod).Unix(), 10),
						},
					},
				},
			},
			expectedQueueEntries: sets.New(types.NamespacedName{
				Name:      "tgb1",
				Namespace: "ns",
			}),
		},
		{
			name: "multiple tgb",
			tgbToEnqueue: []*elbv2api.TargetGroupBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb2",
						Namespace: "ns",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb2",
						Namespace: "ns1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb3",
						Namespace: "ns3",
					},
				},
			},
			expectedQueueEntries: sets.New(types.NamespacedName{
				Name:      "tgb1",
				Namespace: "ns",
			}, types.NamespacedName{
				Name:      "tgb1",
				Namespace: "ns1",
			}, types.NamespacedName{
				Name:      "tgb2",
				Namespace: "ns",
			}, types.NamespacedName{
				Name:      "tgb2",
				Namespace: "ns1",
			}, types.NamespacedName{
				Name:      "tgb3",
				Namespace: "ns3",
			}),
		},
		{
			name: "de-dupe same tgb",
			tgbToEnqueue: []*elbv2api.TargetGroupBinding{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tgb1",
						Namespace: "ns",
					},
				},
			},
			expectedQueueEntries: sets.New(types.NamespacedName{
				Name:      "tgb1",
				Namespace: "ns",
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dq := workqueue.NewDelayingQueue()
			defer dq.ShutDown()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sSchema).
				Build()

			impl := deferredTargetGroupBindingReconcilerImpl{
				delayQueue: dq,
				syncPeriod: syncPeriod,
				k8sClient:  k8sClient,
				logger:     logr.New(&log.NullLogSink{}),

				delayedReconcileTime: 0 * time.Millisecond,
				maxJitter:            0 * time.Millisecond,
			}

			for _, tgb := range tc.tgbToEnqueue {
				impl.Enqueue(tgb)
			}

			assert.Equal(t, tc.expectedQueueEntries.Len(), dq.Len())

			for dq.Len() > 0 {
				v, _ := dq.Get()
				assert.True(t, tc.expectedQueueEntries.Has(v.(types.NamespacedName)), "Expected queue entry not found %+v", v)
			}

		})
	}
}

func TestDeferredReconcilerRun(t *testing.T) {
	testCases := []struct {
		name string
		nsns []types.NamespacedName
	}{
		{
			name: "nothing enqueued",
		},
		{
			name: "something enqueued",
			nsns: []types.NamespacedName{
				{
					Name:      "name",
					Namespace: "ns",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dq := workqueue.NewDelayingQueue()
			go func() {
				time.Sleep(2 * time.Second)
				assert.Equal(t, 0, dq.Len())
				dq.ShutDown()
			}()

			for _, nsn := range tc.nsns {
				dq.Add(nsn)
			}

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sSchema).
				Build()

			impl := deferredTargetGroupBindingReconcilerImpl{
				delayQueue: dq,
				syncPeriod: 5 * time.Minute,
				k8sClient:  k8sClient,
				logger:     logr.New(&log.NullLogSink{}),

				delayedReconcileTime: 0 * time.Millisecond,
				maxJitter:            0 * time.Millisecond,
			}

			impl.Run()

			time.Sleep(5 * time.Second)
		})
	}
}

func TestHandleDeferredItem(t *testing.T) {
	syncPeriod := 5 * time.Minute
	testCases := []struct {
		name               string
		nsn                types.NamespacedName
		storedTGB          *elbv2api.TargetGroupBinding
		requeue            bool
		expectedCheckPoint *string
	}{
		{
			name: "not found",
			nsn: types.NamespacedName{
				Name:      "name",
				Namespace: "ns",
			},
		},
		{
			name: "not eligible",
			nsn: types.NamespacedName{
				Name:      "name",
				Namespace: "ns",
			},
			storedTGB: &elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "ns",
					Annotations: map[string]string{
						annotations.AnnotationCheckPoint:          "foo",
						annotations.AnnotationCheckPointTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
					},
				},
			},
			expectedCheckPoint: aws.String("foo"),
		},
		{
			name: "eligible",
			nsn: types.NamespacedName{
				Name:      "name",
				Namespace: "ns",
			},
			storedTGB: &elbv2api.TargetGroupBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "ns",
					Annotations: map[string]string{
						annotations.AnnotationCheckPoint:          "foo",
						annotations.AnnotationCheckPointTimestamp: strconv.FormatInt(time.Now().Add(-2*syncPeriod).Unix(), 10),
					},
				},
			},
			expectedCheckPoint: aws.String(""),
		},
		{
			name: "failure causes requeue",
			nsn: types.NamespacedName{
				Name:      "name",
				Namespace: "ns",
			},
			requeue: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dq := workqueue.NewDelayingQueue()
			defer dq.ShutDown()

			k8sSchema := runtime.NewScheme()
			// quick hack to inject a fault into the k8s client.
			if !tc.requeue {
				clientgoscheme.AddToScheme(k8sSchema)
				elbv2api.AddToScheme(k8sSchema)
			}
			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sSchema).
				Build()

			impl := deferredTargetGroupBindingReconcilerImpl{
				delayQueue: dq,
				syncPeriod: syncPeriod,
				k8sClient:  k8sClient,
				logger:     logr.New(&log.NullLogSink{}),

				delayedReconcileTime: 0 * time.Millisecond,
				maxJitter:            0 * time.Millisecond,
			}

			if tc.storedTGB != nil {
				k8sClient.Create(context.Background(), tc.storedTGB)
			}

			impl.handleDeferredItem(tc.nsn)

			if tc.requeue {
				assert.Equal(t, 1, dq.Len())
			} else {
				assert.Equal(t, 0, dq.Len())
			}

			if tc.expectedCheckPoint != nil {
				storedTGB := &elbv2api.TargetGroupBinding{}
				k8sClient.Get(context.Background(), tc.nsn, storedTGB)
				assert.Equal(t, *tc.expectedCheckPoint, storedTGB.Annotations[annotations.AnnotationCheckPoint])
			}

		})
	}
}
