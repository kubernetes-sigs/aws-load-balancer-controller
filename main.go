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
	"flag"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	"sigs.k8s.io/aws-alb-ingress-controller/controllers/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/controllers/service"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/aws/throttle"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/networking"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/targetgroupbinding"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	elbv2v1alpha1 "sigs.k8s.io/aws-alb-ingress-controller/apis/elbv2/v1alpha1"
	elbv2controller "sigs.k8s.io/aws-alb-ingress-controller/controllers/elbv2"
	// +kubebuilder:scaffold:imports
)

const (
	flagMetricsAddr          = "metrics-addr"
	flagEnableLeaderElection = "enable-leader-election"
	flagK8sClusterName       = "cluster-name"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = elbv2v1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var k8sClusterName string
	awsCloudConfig := aws.CloudConfig{ThrottleConfig: throttle.NewDefaultServiceOperationsThrottleConfig()}
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	fs.StringVar(&metricsAddr, flagMetricsAddr, ":8080", "The address the metric endpoint binds to.")
	fs.BoolVar(&enableLeaderElection, flagEnableLeaderElection, false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&k8sClusterName, flagK8sClusterName, "", "Kubernetes cluster name")
	awsCloudConfig.BindFlags(fs)
	fs.AddGoFlagSet(flag.CommandLine)
	if err := fs.Parse(os.Args); err != nil {
		setupLog.Error(err, "invalid flags")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	if len(k8sClusterName) == 0 {
		setupLog.Info("Kubernetes cluster name must be specified")
		os.Exit(1)
	}

	cloud, err := aws.NewCloud(awsCloudConfig, metrics.Registry)
	if err != nil {
		setupLog.Error(err, "Unable to initialize AWS cloud")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
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
		podENIResolver, nodeENIResolver, sgManager, sgReconciler, cloud.VpcID(), k8sClusterName, ctrl.Log)

	subnetResolver := networking.NewSubnetsResolver(cloud.EC2(), cloud.VpcID(), k8sClusterName, ctrl.Log.WithName("subnets-resolver"))
	ingGroupReconciler := ingress.NewGroupReconciler(mgr.GetClient(), mgr.GetEventRecorderFor("ingress"), cloud.EC2(), cloud.ELBV2(),
		sgManager, sgReconciler, cloud.VpcID(), k8sClusterName, subnetResolver, ctrl.Log)
	tgbReconciler := elbv2controller.NewTargetGroupBindingReconciler(mgr.GetClient(), mgr.GetFieldIndexer(), finalizerManager, tgbResManager,
		ctrl.Log.WithName("controllers").WithName("TargetGroupBinding"))
	svcReconciler := service.NewServiceReconciler(
		mgr.GetClient(),
		cloud.EC2(),
		cloud.ELBV2(),
		sgManager,
		sgReconciler,
		cloud.VpcID(),
		k8sClusterName,
		subnetResolver,
		ctrl.Log.WithName("controllers").WithName("Service"))
	if err = ingGroupReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Ingress")
		os.Exit(1)
	}
	if err := tgbReconciler.SetupWithManager(context.Background(), mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TargetGroupBinding")
		os.Exit(1)
	}
	if err = svcReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Service")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
