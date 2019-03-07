package config

import (
	"context"
	"strings"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const restrictIngressConfigMap = "alb-ingress-controller-internet-facing-ingresses"
const restrictServiceConfigMap = "alb-ingress-controller-internet-facing-services"

// TODO: I'd prefer to keep config an plain data structure, and move this logic into the object that manages configuration, like current "store" object. Will move this logic there once i clean up the store object.
// BindDynamicSettings will force initial load of these dynamic settings from configMaps, and setup watcher for configMap changes.
func (cfg *Configuration) BindDynamicSettings(mgr manager.Manager, c controller.Controller, cloud aws.CloudAPI) error {
	if cfg.RestrictScheme {
		if err := cfg.initInternetFacingIngresses(mgr.GetClient()); err != nil {
			return err
		}
		if err := cfg.initInternetFacingServices(mgr.GetClient()); err != nil {
			return err
		}
		if err := cfg.watchInternetFacingIngresses(c); err != nil {
			return err
		}
	}
	if cfg.FeatureGate.Enabled(WAF) && !cloud.WAFRegionalAvailable() {
		cfg.FeatureGate.Disable(WAF)
	}

	return nil
}

func (cfg *Configuration) initInternetFacingIngresses(client client.Client) error {
	configMap := &corev1.ConfigMap{}
	configMapKey := types.NamespacedName{
		Namespace: cfg.RestrictSchemeNamespace,
		Name:      restrictIngressConfigMap,
	}
	if err := client.Get(context.Background(), configMapKey, configMap); err != nil {
		cfg.loadInternetFacingIngresses(nil)
	}
	cfg.loadInternetFacingIngresses(configMap)

	return nil
}

func (cfg *Configuration) initInternetFacingServices(client client.Client) error {
	configMap := &corev1.ConfigMap{}
	configMapKey := types.NamespacedName{
		Namespace: cfg.RestrictSchemeNamespace,
		Name:      restrictServiceConfigMap,
	}
	if err := client.Get(context.Background(), configMapKey, configMap); err != nil {
		cfg.loadInternetFacingIngresses(nil)
	}
	cfg.loadInternetFacingIngresses(configMap)

	return nil
}

func (cfg *Configuration) watchInternetFacingIngresses(c controller.Controller) error {
	if err := c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.Funcs{
		CreateFunc: func(e event.CreateEvent, _ workqueue.RateLimitingInterface) {
			if cfg.isRestrictIngressConfigMap(e.Meta) {
				cfg.loadInternetFacingIngresses(e.Object.(*corev1.ConfigMap))
			}
			if cfg.isRestrictServiceConfigMap(e.Meta) {
				cfg.loadInternetFacingServices(e.Object.(*corev1.ConfigMap))
			}
		},
		UpdateFunc: func(e event.UpdateEvent, _ workqueue.RateLimitingInterface) {
			if cfg.isRestrictIngressConfigMap(e.MetaNew) {
				cfg.loadInternetFacingIngresses(e.ObjectNew.(*corev1.ConfigMap))
			}
			if cfg.isRestrictServiceConfigMap(e.MetaNew) {
				cfg.loadInternetFacingServices(e.ObjectNew.(*corev1.ConfigMap))
			}
		},
		DeleteFunc: func(e event.DeleteEvent, _ workqueue.RateLimitingInterface) {
			if cfg.isRestrictIngressConfigMap(e.Meta) {
				cfg.loadInternetFacingIngresses(nil)
			}
			if cfg.isRestrictServiceConfigMap(e.Meta) {
				cfg.loadInternetFacingServices(nil)
			}
		},
	}); err != nil {
		return err
	}

	return nil
}

// TODO: seems the dynamic admission control & initializers can solve this problem more better.(block external facing ingress creation if specific user don't have permissions)
// TODO: we can have a shared configMap to store dynamic settings like this.
// LoadInternetFacingIngresses will load the InternetFacingIngresses settings from configMap.
// The Key:Value pair are interpreted as "namespace: comma-separated list of ingressNames"
func (cfg *Configuration) loadInternetFacingIngresses(configMap *corev1.ConfigMap) {
	cfg.InternetFacingIngresses = make(map[string][]string)
	if configMap != nil {
		for namespace, configLine := range configMap.Data {
			configLine := strings.Replace(configLine, " ", "", -1)
			ingressNames := strings.Split(configLine, ",")
			cfg.InternetFacingIngresses[namespace] = ingressNames
		}
	}
}

func (cfg *Configuration) loadInternetFacingServices(configMap *corev1.ConfigMap) {
	cfg.InternetFacingIngresses = make(map[string][]string)
	if configMap != nil {
		for namespace, configLine := range configMap.Data {
			configLine := strings.Replace(configLine, " ", "", -1)
			ingressNames := strings.Split(configLine, ",")
			cfg.InternetFacingServices[namespace] = ingressNames
		}
	}
}

func (cfg *Configuration) isRestrictIngressConfigMap(meta metav1.Object) bool {
	return (meta.GetNamespace() == cfg.RestrictSchemeNamespace) &&
		(meta.GetName() == restrictIngressConfigMap)
}

func (cfg *Configuration) isRestrictServiceConfigMap(meta metav1.Object) bool {
	return (meta.GetNamespace() == cfg.RestrictSchemeNamespace) &&
		(meta.GetName() == restrictServiceConfigMap)
}
