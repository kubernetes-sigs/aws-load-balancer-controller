package auth

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	mock_cache "github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks/controller-runtime/cache"
	"github.com/magiconair/properties/assert"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestEnqueueRequestsForSecretEvent_enqueueImpactedObjects(t *testing.T) {
	for _, tc := range []struct {
		name              string
		secret            corev1.Secret
		ingressIndexes    map[string]networking.IngressList
		serviceIndexes    map[string]corev1.ServiceList
		expectedIngresses sets.String
		expectedServices  sets.String
	}{
		{
			name: "secret are not referenced by any ingresses & services",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "secret",
				},
			},
			ingressIndexes: map[string]networking.IngressList{
				"namespace/secret": {},
			},
			serviceIndexes: map[string]corev1.ServiceList{
				"namespace/secret": {},
			},
			expectedIngresses: sets.NewString(),
			expectedServices:  sets.NewString(),
		},
		{
			name: "secret are referenced by ingresses & services",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "secret",
				},
			},
			ingressIndexes: map[string]networking.IngressList{
				"namespace/secret": {
					Items: []networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress1",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress2",
							},
						},
					},
				},
			},
			serviceIndexes: map[string]corev1.ServiceList{
				"namespace/secret": {
					Items: []corev1.Service{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "service1",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "service2",
							},
						},
					},
				},
			},
			expectedIngresses: sets.NewString("namespace/ingress1", "namespace/ingress2"),
			expectedServices:  sets.NewString("namespace/service1", "namespace/service2"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCache := mock_cache.NewMockCache(ctrl)
			for index, value := range tc.ingressIndexes {
				mockCache.EXPECT().List(gomock.Any(), client.MatchingField(FieldAuthOIDCSecret, index), gomock.Any()).SetArg(2, value)
			}
			for index, value := range tc.serviceIndexes {
				mockCache.EXPECT().List(gomock.Any(), client.MatchingField(FieldAuthOIDCSecret, index), gomock.Any()).SetArg(2, value)
			}

			actualIngresses := sets.NewString()
			actualServices := sets.NewString()

			ingressChan := make(chan event.GenericEvent)
			serviceChan := make(chan event.GenericEvent)
			ingressReceiveDone := make(chan bool)
			serviceReceiveDone := make(chan bool)
			go func() {
				for e := range ingressChan {
					actualIngresses.Insert(fmt.Sprintf("%s/%s", e.Meta.GetNamespace(), e.Meta.GetName()))
				}
				ingressReceiveDone <- true
			}()
			go func() {
				for e := range serviceChan {
					actualServices.Insert(fmt.Sprintf("%s/%s", e.Meta.GetNamespace(), e.Meta.GetName()))
				}
				serviceReceiveDone <- true
			}()

			handler := &EnqueueRequestsForSecretEvent{
				Cache:       mockCache,
				IngressChan: ingressChan,
				ServiceChan: serviceChan,
			}
			handler.enqueueImpactedObjects(&tc.secret, nil)

			close(ingressChan)
			close(serviceChan)
			<-ingressReceiveDone
			<-serviceReceiveDone

			assert.Equal(t, actualIngresses, tc.expectedIngresses)
			assert.Equal(t, actualServices, tc.expectedServices)
		})
	}
}
