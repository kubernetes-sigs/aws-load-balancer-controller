package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/ingress/core/pkg/ingress/controller"
	"k8s.io/ingress/core/pkg/ingress/defaults"
	"k8s.io/ingress/core/pkg/ingress/store"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/albingress"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/config"
	"github.com/spf13/pflag"
)

const albRegex = "^[a-zA-Z0-9]+$"

func TestALBNamePrefixGeneratedCompliesWithALB(t *testing.T) {
	expectedName := "clustername" // dashes removed and limited to 11 chars
	in := "cluster-name-hello"
	actualName, err := cleanClusterName(in)
	if err != nil {
		t.Errorf("Error returned atttempted to create ALB prefix. Error: %s", err.Error())
	}

	if actualName != expectedName {
		t.Errorf("ALBNamePrefix generated incorrectly was: %s | expected: %s",
			actualName, expectedName)
	}

	// sanity check on expectedName; ensures it's compliant with ALB naming
	match, err := regexp.MatchString(albRegex, expectedName)
	if err != nil {
		t.Errorf("Failed to parse regex for test. Likley an issues with the test. Regex: %s",
			albRegex)
	}
	if !match {
		t.Errorf("Expected name was not compliant with AWS-ALB naming restrictions. Could be "+
			"issue with test. expectedName: %s, compliantRegexTest: %s", expectedName, albRegex)
	}
}

func TestNewALBController(t *testing.T) {
	ac := NewALBController(&aws.Config{}, &config.Config{})
	if ac == nil {
		t.Errorf("NewALBController returned nil")
	}
}

func TestALBController_Configure(t *testing.T) {
	configureTests := []struct {
		clusterName string
		expectError bool
	}{
		{"okFakeName", false},
		{"", true},
	}
	for _, tt := range configureTests {
		var wg sync.WaitGroup
		if !tt.expectError {
			wg.Add(2)
		}
		initialSyncInvoked := false
		syncInvoked := false
		pollerInvoked := false
		recorder := record.NewFakeRecorder(1)
		ac := &albController{
			initialSync: func(ac *albController) { initialSyncInvoked = true },
			syncer: func(ac *albController) {
				defer wg.Done()
				syncInvoked = true
			},
			poller: func(ac *albController) {
				defer wg.Done()
				pollerInvoked = true
			},
			classNameGetter: func(ic *controller.GenericController) string { return "fake" },
			recorderGetter:  func(ic *controller.GenericController) record.EventRecorder { return recorder },
			clusterName:     tt.clusterName,
		}
		ic := &controller.GenericController{}

		err := ac.Configure(ic)
		wg.Wait()

		if tt.expectError {
			if err == nil {
				t.Errorf("Expected an error from Configure()")
			}
			if initialSyncInvoked {
				t.Errorf("Expected initialSync NOT to be invoked")
			}
			if syncInvoked {
				t.Errorf("Expected syncer NOT to be invoked")
			}
			if pollerInvoked {
				t.Errorf("Expected poller NOT to be invoked")
			}
		} else {
			if err != nil {
				t.Errorf("Did not expect error from Configure()")
			}
			if !initialSyncInvoked {
				t.Errorf("Expected initialSync to be invoked")
			}
			if !syncInvoked {
				t.Errorf("Expected syncer to be invoked")
			}
			if !pollerInvoked {
				t.Errorf("Expected poller to be invoked")
			}
		}
	}
}

func TestALBController_Name(t *testing.T) {
	ac := albController{}
	if ac.Name() != "AWS Application Load Balancer Controller" {
		t.Errorf("Expected name to be 'AWS Application Load Balancer Controller'")
	}
}

func TestALBController_OverrideFlags(t *testing.T) {
	ac := albController{}
	flags := &pflag.FlagSet{}
	flags.Bool("update-status-on-shutdown", true, "")
	flags.Duration("sync-period", time.Second, "")
	flags.SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		return pflag.NormalizedName(name)
	})
	ac.OverrideFlags(flags)
	if flag, _ := flags.GetBool("update-status-on-shutdown"); flag != false {
		t.Errorf("Expected update-status-on-shutdown to be false, got %v", flag)
	}
	if flag, _ := flags.GetDuration("sync-period"); flag != 30*time.Second {
		t.Errorf("Expected sync-period to be 30s, got %v", flag)
	}
}

func TestALBController_SetConfig(t *testing.T) {
	ac := albController{}
	cfgMap := &api.ConfigMap{}
	ac.SetConfig(cfgMap)
	freshCfgMap := &api.ConfigMap{}
	if !reflect.DeepEqual(cfgMap, freshCfgMap) {
		t.Errorf("cfgMap was unexpectedly modified")
	}
}

func TestALBController_DefaultEndpoint(t *testing.T) {
	ac := albController{}
	if !reflect.DeepEqual(ac.DefaultEndpoint(), ingress.Endpoint{}) {
		t.Errorf("Expected DefaultEndpoint to be %v", ingress.Endpoint{})
	}
}

func TestALBController_SetListers(t *testing.T) {
	ac := albController{}
	lister := ingress.StoreLister{}
	ac.SetListers(lister)
	if !reflect.DeepEqual(ac.storeLister, lister) {
		t.Errorf("Expected listers to be set and equal to %v", lister)
	}
}

func TestALBController_BackendDefaults(t *testing.T) {
	ac := albController{}
	if !reflect.DeepEqual(ac.BackendDefaults(), defaults.Backend{}) {
		t.Errorf("Expected BackendDefaults() to return %v", defaults.Backend{})
	}
}

func TestALBController_Check(t *testing.T) {
	ac := albController{}
	if ac.Check(&http.Request{}) != nil {
		t.Errorf("Expected Check() to return nil")
	}
}

func TestALBController_DefaultIngressClass(t *testing.T) {
	ac := albController{}
	if ac.DefaultIngressClass() != "alb" {
		t.Errorf("Expected DefaultIngressClass() to return 'alb'")
	}
}

func TestALBController_Info(t *testing.T) {
	ac := albController{}
	expected := &ingress.BackendInfo{
		Name:       "ALB Ingress Controller",
		Release:    "1.0.0",
		Build:      "git-00000000",
		Repository: "git://github.com/kubernetes-sigs/aws-alb-ingress-controller",
	}
	if !reflect.DeepEqual(ac.Info(), expected) {
		t.Errorf("Expected Info() to return %v", expected)
	}
}

func TestALBController_ConfigureFlags(t *testing.T) {
	ac := albController{}
	flags := &pflag.FlagSet{}
	ac.ConfigureFlags(flags)

	// Check flags without having set any env variables
	if flag, _ := flags.GetBool("restrict-scheme"); flag != false {
		t.Errorf("Expected restrict-scheme to be false by default")
	}
	if flag, _ := flags.GetString("restrict-scheme-namespace"); flag != "default" {
		t.Errorf("Expected restrict-scheme-namespace to be 'default' by default")
	}
	if flag, _ := flags.GetDuration("alb-sync-interval"); flag != time.Hour {
		t.Error("Expected alb-sync-interval to be 1 hour by default")
	}

	// Reset flags and set some env variables before configuring flags
	flags = &pflag.FlagSet{}
	os.Setenv("ALB_SYNC_INTERVAL", "30m")
	os.Setenv("ALB_CONTROLLER_RESTRICT_SCHEME", "true")
	os.Setenv("ALB_CONTROLLER_RESTRICT_SCHEME_CONFIG_NAMESPACE", "notDefault")
	ac.ConfigureFlags(flags)
	if flag, _ := flags.GetBool("restrict-scheme"); flag != true {
		t.Errorf("Expected restrict-scheme to be true")
	}
	if flag, _ := flags.GetString("restrict-scheme-namespace"); flag != "notDefault" {
		t.Errorf("Expected restrict-scheme-namespace to be 'notDefault'")
	}
	if flag, _ := flags.GetDuration("alb-sync-interval"); flag != 30*time.Minute {
		t.Error("Expected alb-sync-interval to be 30 mins")
	}
}

func TestALBController_GetNodes(t *testing.T) {
	instanceA := "i-aaaa"
	instanceB := "i-bbbb"
	instanceC := "i-cccc"
	nodeStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	nodeStore.Add(&api.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"node-role.kubernetes.io/master": ""}},
		Spec:       api.NodeSpec{ExternalID: instanceC},
	})
	nodeStore.Add(&api.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node2", Labels: make(map[string]string)},
		Spec:       api.NodeSpec{ExternalID: instanceB},
	})
	nodeStore.Add(&api.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node3", Labels: make(map[string]string)},
		Spec:       api.NodeSpec{ExternalID: instanceA},
	})
	ac := albController{
		storeLister: ingress.StoreLister{
			Node: store.NodeLister{Store: nodeStore},
		},
	}

	nodes := ac.GetNodes()

	if instanceA != *nodes[0] {
		t.Errorf("Expected: %v, Actual: %v", instanceA, *nodes[0])
	}
	if instanceB != *nodes[1] {
		t.Errorf("Expected: %v, Actual: %v", instanceB, *nodes[1])
	}
}

func TestALBController_GetServiceNodePort(t *testing.T) {
	serviceStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	serviceStore.Add(&api.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "service1"},
		Spec: api.ServiceSpec{
			Type:  api.ServiceTypeNodePort,
			Ports: []api.ServicePort{{Port: 4000, NodePort: 8000}},
		},
	})
	serviceStore.Add(&api.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "service2"},
		Spec: api.ServiceSpec{
			Type:  api.ServiceTypeClusterIP,
			Ports: []api.ServicePort{{Port: 4001}},
		},
	})
	ac := albController{
		storeLister: ingress.StoreLister{
			Service: store.ServiceLister{Store: serviceStore},
		},
	}

	np, err := ac.GetServiceNodePort("service1", 4000)
	if *np != 8000 {
		t.Errorf("Expected node port for service1 to be 8000")
	}

	np, err = ac.GetServiceNodePort("service2", 4001)
	if np != nil {
		t.Error("Expected nil as service2 is not a node port service")
	}
	if err == nil {
		t.Errorf("Expected error as service2 is not a node port service")
	}

	np, err = ac.GetServiceNodePort("service1", 4001)
	if np != nil {
		t.Errorf("Expected nil as service1 is not listening on backend port 4001")
	}
	if err == nil {
		t.Errorf("Expected failure as service1 is not listening on backend port 4001")
	}

	np, err = ac.GetServiceNodePort("service3", 5000)
	if np != nil {
		t.Errorf("Expected nil as service3 does not exist")
	}
	if err == nil {
		t.Errorf("Expected error as service3 does not exist")
	}
}

func TestALBController_StateHandler(t *testing.T) {
	ingress := albingress.ALBIngress{}
	encodedIngressByteSlice, _ := json.Marshal(ingress)
	expectedBody := fmt.Sprintf("[%s]\n", encodedIngressByteSlice)
	ac := albController{
		ALBIngresses: []*albingress.ALBIngress{&ingress},
	}
	rw := httptest.NewRecorder()

	ac.StateHandler(rw, nil)

	if rw.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected header Content-Type: application-json")
	}
	bodyString := fmt.Sprintf("%s", rw.Body.Bytes())
	if expectedBody != bodyString {
		t.Errorf("Expected response body to be '%s', found '%s'", expectedBody, bodyString)
	}
}

func TestALBController_StatusHandler(t *testing.T) {
	healthy := func() error { return nil }
	unhealthy := func() error { return fmt.Errorf("some failure") }
	statusHandlerTests := []struct {
		target             string
		ec2Check           func() error
		elbv2Check         func() error
		expectedStatusCode int
		expectedBody       string
	}{
		{"/healthz", healthy, healthy, 200, "{}\n"},
		{"/healthz?full=1", healthy, healthy, 200, "{\n    \"ec2\": \"OK\",\n    \"elbv2\": \"OK\"\n}\n"},
		{"/healthz", healthy, unhealthy, 503, "{}\n"},
		{"/healthz?full=1", healthy, unhealthy, 503, "{\n    \"ec2\": \"OK\",\n    \"elbv2\": \"some failure\"\n}\n"},
	}

	ac := albController{
		awsChecks: make(map[string]func() error),
	}
	for _, tt := range statusHandlerTests {
		ac.awsChecks["elbv2"] = tt.elbv2Check
		ac.awsChecks["ec2"] = tt.ec2Check
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", tt.target, nil)
		ac.StatusHandler(w, r)
		if w.Result().StatusCode != tt.expectedStatusCode {
			t.Errorf("Expected http status code to be %d, got %d", tt.expectedStatusCode, w.Result().StatusCode)
		}
		if w.Result().Header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Errorf("Expected header Content-Type: application/json; charset=utf-8, got %v", w.Result().Header.Get("Content-Type"))
		}
		responseBody := string(w.Body.Bytes()[:w.Body.Len()])
		if tt.expectedBody != responseBody {
			t.Errorf("Expected %v in response body, got %v", tt.expectedBody, responseBody)
		}
	}
}
