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
	"encoding/json"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/imdario/mergo"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/utils"

	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/action"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/conditions"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/loadbalancer"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/targetgroup"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/errors"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
	pkgerrors "github.com/pkg/errors"
)

// Ingress defines the valid annotations present in one AWS ALB Ingress rule
type Ingress struct {
	// TODO: found out why the ObjectMeta is needed?
	metav1.ObjectMeta
	Action       *action.Config
	Conditions   *conditions.Config
	HealthCheck  *healthcheck.Config
	TargetGroup  *targetgroup.Config
	LoadBalancer *loadbalancer.Config
	Tags         *tags.Config
	Error        error
}

func NewIngressDummy() *Ingress {
	return &Ingress{
		Action:       action.Dummy(),
		HealthCheck:  &healthcheck.Config{},
		TargetGroup:  targetgroup.Dummy(),
		LoadBalancer: loadbalancer.Dummy(),
		Tags:         &tags.Config{},
	}
}

// Service contains the same annotations as Ingress
type Service Ingress

// Merge build a new service annotation by merge in ingress annotation
func (s *Service) Merge(b *Ingress, cfg *config.Configuration) *Service {
	return &Service{
		ObjectMeta:   s.ObjectMeta,
		Action:       s.Action,
		Conditions:   s.Conditions,
		LoadBalancer: s.LoadBalancer,
		Tags:         s.Tags,
		Error:        s.Error,
		HealthCheck:  s.HealthCheck.Merge(b.HealthCheck, cfg),
		TargetGroup:  s.TargetGroup.Merge(b.TargetGroup, cfg),
	}
}

func NewServiceDummy() *Service {
	return &Service{
		HealthCheck: &healthcheck.Config{},
		TargetGroup: targetgroup.Dummy(),
		Tags:        &tags.Config{},
	}
}

// Extractor defines the annotation parsers to be used in the extraction of annotations
type Extractor struct {
	annotations map[string]parser.IngressAnnotation
}

// NewIngressAnnotationExtractor creates a new annotations extractor
func NewIngressAnnotationExtractor(cfg resolver.Resolver) Extractor {
	return Extractor{
		map[string]parser.IngressAnnotation{
			"Action":       action.NewParser(),
			"Conditions":   conditions.NewParser(),
			"HealthCheck":  healthcheck.NewParser(cfg),
			"TargetGroup":  targetgroup.NewParser(cfg),
			"LoadBalancer": loadbalancer.NewParser(cfg),
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

// LoadStringAnnotation loads annotation into value of type string from list of annotations by priority.
func LoadStringAnnotation(annotation string, value *string, annotations ...map[string]string) bool {
	key := parser.GetAnnotationWithPrefix(annotation)
	raw, ok := utils.MapFindFirst(key, annotations...)
	if !ok {
		return false
	}
	*value = raw
	return true
}

func LoadStringSliceAnnotation(annotation string, value *[]string, annotations ...map[string]string) bool {
	key := parser.GetAnnotationWithPrefix(annotation)
	raw, ok := utils.MapFindFirst(key, annotations...)
	if !ok {
		return false
	}

	var result []string
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		result = append(result, part)
	}
	*value = result
	return true
}

func LoadBoolAnnocation(annotation string, value *bool, annotations ...map[string]string) (bool, error) {
	key := parser.GetAnnotationWithPrefix(annotation)
	raw, ok := utils.MapFindFirst(key, annotations...)
	if !ok {
		return false, nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return true, pkgerrors.Wrapf(err, "failed to parse annotation, %v: %v", key, raw)
	}
	*value = b
	return true, nil
}

// LoadInt64Annotation loads annotation into value of type int64 from list of annotations by priority.
func LoadInt64Annotation(annotation string, value *int64, annotations ...map[string]string) (bool, error) {
	key := parser.GetAnnotationWithPrefix(annotation)
	raw, ok := utils.MapFindFirst(key, annotations...)
	if !ok {
		return false, nil
	}
	i, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return true, pkgerrors.Wrapf(err, "failed to parse annotation, %v: %v", key, raw)
	}
	*value = i
	return true, nil
}

// LoadInt64Annotation loads annotation into value of type JSON from list of annotations by priority.
func LoadJSONAnnotation(annotation string, value interface{}, annotations ...map[string]string) (bool, error) {
	key := parser.GetAnnotationWithPrefix(annotation)
	raw, ok := utils.MapFindFirst(key, annotations...)
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return true, pkgerrors.Wrapf(err, "failed to parse annotation, %v: %v", key, raw)
	}
	return true, nil
}
