/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package annotations

import (
	"github.com/golang/glog"
	"github.com/imdario/mergo"

	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/listener"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/rule"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

// DeniedKeyName name of the key that contains the reason to deny a location
const DeniedKeyName = "Denied"

// Ingress defines the valid annotations present in one AWS ALB Ingress rule
type Ingress struct {
	metav1.ObjectMeta
	HealthCheck healthcheck.Config

	TargetGroup *targetgroup.Config

	LoadBalancer *loadbalancer.Config

	Rule *rule.Config

	Listener *listener.Config

	Tags *tags.Config

	Error error
}

// Service contains the same annotations as Ingress
type Service Ingress

func (s *Service) Merge(b *Ingress) {
	s.HealthCheck.Merge(b.HealthCheck)
	s.TargetGroup.Merge(b.TargetGroup)
	s.Rule.Merge(b.Rule)
	s.Listener.Merge(b.Listener)
}

// Extractor defines the annotation parsers to be used in the extraction of annotations
type Extractor struct {
	annotations map[string]parser.IngressAnnotation
}

// NewIngressAnnotationExtractor creates a new annotations extractor
func NewIngressAnnotationExtractor(cfg resolver.Resolver) Extractor {
	return Extractor{
		map[string]parser.IngressAnnotation{
			"HealthCheck":  healthcheck.NewParser(cfg),
			"TargetGroup":  targetgroup.NewParser(cfg),
			"LoadBalancer": loadbalancer.NewParser(cfg),
			"Rule":         rule.NewParser(cfg),
			"Listener":     listener.NewParser(cfg),
			"Tags":         tags.NewParser(cfg),
		},
	}
}

// NewServiceAnnotationExtractor creates a new annotations extractor
func NewServiceAnnotationExtractor(cfg resolver.Resolver) Extractor {
	return Extractor{
		map[string]parser.IngressAnnotation{
			"HealthCheck": healthcheck.NewParser(cfg),
			"TargetGroup": targetgroup.NewParser(cfg),
			"Rule":        rule.NewParser(cfg),
			"Listener":    listener.NewParser(cfg),
			"Tags":        tags.NewParser(cfg),
		},
	}
}

// ExtractIngress extracts the annotations from an Ingress
func (e Extractor) ExtractIngress(ing *extensions.Ingress) *Ingress {
	pia := &Ingress{
		ObjectMeta: ing.ObjectMeta,
	}

	i, err := e.extract(pia, ing)
	pia.Error = err
	return i.(*Ingress)
}

// ExtractService extracts the annotations from a Service
func (e Extractor) ExtractService(svc *corev1.Service) *Service {
	psa := &Service{
		ObjectMeta: svc.ObjectMeta,
	}
	s, err := e.extract(psa, svc)
	psa.Error = err
	return s.(*Service)
}

// Extract extracts the annotations from metadata
// TODO put kind in log message
func (e Extractor) extract(dst interface{}, o metav1.Object) (interface{}, error) {
	data := make(map[string]interface{})
	for name, annotationParser := range e.annotations {
		val, err := annotationParser.Parse(o)
		glog.V(6).Infof("annotation %v in %v %v/%v: %v", name, "o.GetKind()", o.GetNamespace(), o.GetName(), val)
		if err != nil {
			if errors.IsMissingAnnotations(err) {
				continue
			}

			glog.V(5).Infof("error reading %v annotation in %v %v/%v: %v", name, "o.GetKind()", o.GetNamespace(), o.GetName(), err)
			return dst, err
		}
		if val != nil {
			data[name] = val
		}
	}
	err := mergo.MapWithOverwrite(dst, data)
	if err != nil {
		glog.Errorf("unexpected error merging extracted annotations: %v", err)
		return dst, err
	}

	return dst, nil
}
