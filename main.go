/*


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

package main

import (
	"context"
	"github.com/spf13/pflag"
	zapraw "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	elbv2controller "sigs.k8s.io/aws-load-balancer-controller/controllers/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/ingress"
	"sigs.k8s.io/aws-load-balancer-controller/controllers/service"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	inject "sigs.k8s.io/aws-load-balancer-controller/pkg/inject"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/targetgroupbinding"
	corewebhook "sigs.k8s.io/aws-load-balancer-controller/webhooks/core"
	elbv2webhook "sigs.k8s.io/aws-load-balancer-controller/webhooks/elbv2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = elbv2api.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	awsCloudConfig := aws.CloudConfig{ThrottleConfig: throttle.NewDefaultServiceOperationsThrottleConfig()}
	injectConfig := inject.Config{}
	controllerConfig := config.NewControllerConfig(scheme)

	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	awsCloudConfig.BindFlags(fs)
	injectConfig.BindFlags(fs)
	controllerConfig.BindFlags(fs)

	if err := fs.Parse(os.Args); err != nil {
		setupLog.Error(err, "invalid flags")
		os.Exit(1)
	}
	if err := controllerConfig.Validate(); err != nil {
		setupLog.Error(err, "Failed to validate controller configuration")
		os.Exit(1)
	}

	logLevel := zapraw.NewAtomicLevelAt(0)
	if controllerConfig.LogLevel == "debug" {
		logLevel = zapraw.NewAtomicLevelAt(-1)
	}
	ctrl.SetLogger(zap.New(zap.UseDevMode(false), zap.Level(&logLevel)))

	cloud, err := aws.NewCloud(awsCloudConfig, metrics.Registry)
	if err != nil {
		setupLog.Error(err, "Unable to initialize AWS cloud")
		os.Exit(1)
	}
	restCfg, err := controllerConfig.BuildRestConfig()
	if err != nil {
		setupLog.Error(err, "Unable to build REST config")
	}
	mgr, err := ctrl.NewManager(restCfg, controllerConfig.BuildRuntimeOptions())
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	podENIResolver := networking.NewDefaultPodENIInfoResolver(cloud.EC2(), cloud.VpcID(), ctrl.Log)
	nodeENIResolver := networking.NewDefaultNodeENIInfoResolver(cloud.EC2(), ctrl.Log)
	sgManager := networking.NewDefaultSecurityGroupManager(cloud.EC2(), ctrl.Log)
	sgReconciler := networking.NewDefaultSecurityGroupReconciler(sgManager, ctrl.Log)
	finalizerManager := k8s.NewDefaultFinalizerManager(mgr.GetClient(), ctrl.Log)
	tgbResManager := targetgroupbinding.NewDefaultResourceManager(mgr.GetClient(), cloud.ELBV2(),
		podENIResolver, nodeENIResolver, sgManager, sgReconciler, cloud.VpcID(), controllerConfig.ClusterName, ctrl.Log)

	subnetResolver := networking.NewSubnetsResolver(cloud.EC2(), cloud.VpcID(), controllerConfig.ClusterName, ctrl.Log.WithName("subnets-resolver"))
	ingGroupReconciler := ingress.NewGroupReconciler(cloud, mgr.GetClient(), mgr.GetEventRecorderFor("ingress"),
		sgManager, sgReconciler, controllerConfig, subnetResolver,
		ctrl.Log.WithName("controllers").WithName("Ingress"))
	svcReconciler := service.NewServiceReconciler(cloud, mgr.GetClient(), mgr.GetEventRecorderFor("service"),
		sgManager, sgReconciler, controllerConfig, subnetResolver,
		ctrl.Log.WithName("controllers").WithName("Service"))
	tgbReconciler := elbv2controller.NewTargetGroupBindingReconciler(mgr.GetClient(), finalizerManager, tgbResManager, controllerConfig,
		ctrl.Log.WithName("controllers").WithName("TargetGroupBinding"))

	ctx := context.Background()
	if err = ingGroupReconciler.SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Ingress")
		os.Exit(1)
	}
	if err = svcReconciler.SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Service")
		os.Exit(1)
	}
	if err := tgbReconciler.SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TargetGroupBinding")
		os.Exit(1)
	}

	podReadinessGateInjector := inject.NewPodReadinessGate(injectConfig, mgr.GetClient(), ctrl.Log.WithName("pod-readiness-gate-injector"))
	corewebhook.NewPodMutator(podReadinessGateInjector).SetupWithManager(mgr)
	elbv2webhook.NewTargetGroupBindingMutator(cloud.ELBV2(), ctrl.Log).SetupWithManager(mgr)
	elbv2webhook.NewTargetGroupBindingValidator(ctrl.Log).SetupWithManager(mgr)
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
