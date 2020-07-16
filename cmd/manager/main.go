/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"math/rand"
	"os"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/backend"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	ebctl "sigs.k8s.io/aws-alb-ingress-controller/pkg/controller/endpointbinding"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/controller/ingress"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/controller/service"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/version"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/apis"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const (
	// High enough QPS to fit all expected use cases. QPS=0 is not set here, because
	// client code is overriding it.
	defaultQPS = 1e6
	// High enough Burst to fit all expected use cases. Burst=0 is not set here, because
	// client code is overriding it.
	defaultBurst = 1e6
)

func main() {
	rand.Seed(time.Now().UnixNano())
	fmt.Println(version.String())
	log.SetLogger(log.ZapLogger(false))
	logger := log.Log.WithName("entrypoint")

	options, err := getOptions()
	if err != nil {
		logger.Error(err, "unable to parse options")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	logger.Info("setting up client for manager")
	cfg, err := buildRestConfig(options)
	if err != nil {
		logger.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	logger.Info("setting up manager")
	mgr, err := manager.New(cfg, manager.Options{
		Namespace:               options.WatchNamespace,
		SyncPeriod:              &options.SyncPeriod,
		LeaderElection:          options.LeaderElection,
		LeaderElectionID:        options.LeaderElectionID,
		LeaderElectionNamespace: options.LeaderElectionNamespace,
		MetricsBindAddress:      ":8080",
	})
	if err != nil {
		logger.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	logger.Info("Registering Components.")

	// Setup Scheme for all resources
	logger.Info("setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		logger.Error(err, "unable add APIs to scheme")
		os.Exit(1)
	}

	cloud, err := cloud.New(options.cloudConfig)
	if err != nil {
		logger.Error(err, "unable to initialize aws cloud")
		os.Exit(1)
	}

	ebRepoIndexers := map[string]backend.IndexFunc{
		backend.EndpointBindingRepoIndexStack:   backend.RepoIndexFuncStack,
		backend.EndpointBindingRepoIndexService: backend.RepoIndexFuncService,
	}
	ebRepo := backend.NewEndpointBindingRepo(ebRepoIndexers)
	if err := ingress.Initialize(mgr, cloud, ebRepo, options.ingressConfig); err != nil {
		logger.Error(err, "unable to initialize ingress controller")
		os.Exit(1)
	}
	if err := ebctl.Initialize(mgr, cloud, ebRepo); err != nil {
		logger.Error(err, "unable to initialize endpoint-binding controller")
		os.Exit(1)
	}
	if err := service.Initialize(mgr, cloud, ebRepo, options.ingressConfig); err != nil {
		logger.Error(err, "Unable to initialize service controller")
		os.Exit(1)
	}
	logger.Info("setting up webhooks")
	if err := webhook.AddToManager(mgr); err != nil {
		logger.Error(err, "unable to register webhooks to the manager")
		os.Exit(1)
	}

	// Start the Cmd
	logger.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		logger.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}

// buildRestConfig creates a new Kubernetes REST configuration. apiserverHost is
// the URL of the API server in the format protocol://address:port/pathPrefix,
// kubeConfig is the location of a kubeconfig file. If defined, the kubeconfig
// file is loaded first, the URL of the API server read from the file is then
// optionally overridden by the value of apiserverHost.
// If neither apiserverHost nor kubeConfig are passed in, we assume the
// controller runs inside Kubernetes and fallback to the in-cluster config. If
// the in-cluster config is missing or fails, we fallback to the default config.
func buildRestConfig(options *Options) (*rest.Config, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags(options.APIServerHost, options.KubeConfigFile)
	if err != nil {
		return nil, err
	}
	restCfg.QPS = defaultQPS
	restCfg.Burst = defaultBurst
	return restCfg, nil
}
