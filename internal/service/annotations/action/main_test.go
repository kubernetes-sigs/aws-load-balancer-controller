package action

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/dummy"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/resolver"
)

type mockBackend struct {
	resolver.Mock
}

func TestIngressActions(t *testing.T) {
	ing := dummy.NewIngress()

	data := map[string]string{}
	data[parser.GetIngressAnnotationWithPrefix("actions.fixed-response-action")] = `{"Type": "fixed-response", "FixedResponseConfig": {"ContentType":"text/plain",
	"StatusCode":"503", "MessageBody":"message body"}}`
	data[parser.GetIngressAnnotationWithPrefix("actions.redirect-action")] = `{"Type": "redirect", "RedirectConfig": {"Protocol":"HTTPS",
  "Port":"443", "Host":"#{host}", "Path": "/#{path}", "Query": "#{query}", "StatusCode": "HTTP_301"}}`
	data[parser.GetIngressAnnotationWithPrefix("actions.forward")] = `{"Type": "forward", "TargetGroupArn": "legacy-tg-arn"}`
	ing.SetAnnotations(data)

	ai, err := NewParser(mockBackend{}).Parse(ing)
	if err != nil {
		t.Error(err)
		return
	}

	a, ok := ai.(*Config)
	if !ok {
		t.Errorf("expected a Config type")
	}

	if *a.Actions["fixed-response-action"].Type != elbv2.ActionTypeEnumFixedResponse {
		t.Errorf("expected fixed-response-action Type to be fixed-response, but returned %v", *a.Actions["fixed-response-action"].Type)
	}
	if *a.Actions["redirect-action"].RedirectConfig.StatusCode != elbv2.RedirectActionStatusCodeEnumHttp301 {
		t.Errorf("expected redirect-action StatusCode to be %v, but returned %v", elbv2.RedirectActionStatusCodeEnumHttp301, *a.Actions["redirect-action"].RedirectConfig.StatusCode)
	}
}

func TestInvalidIngressActions(t *testing.T) {
	ing := dummy.NewIngress()

	data := map[string]string{}
	data[parser.GetIngressAnnotationWithPrefix("actions.redirect-action")] = `{"Type": "fixed-response", "RedirectConfig": {"Protocol":"HTTPS",
  "Port":"443", "Host":"#{host}", "Path": "/#{path}", "Query": "#{query}", "StatusCode": "HTTP_301"}}`
	ing.SetAnnotations(data)

	_, err := NewParser(mockBackend{}).Parse(ing)
	if err == nil {
		t.Errorf("invalid annotation configuration was provided but an error was not returned: %v", err)
	}
}
