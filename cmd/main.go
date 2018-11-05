/*
Copyright 2015 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/pprof"
	"os"
	"syscall"
	"time"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric/collectors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/ticketmaster/aws-sdk-go-cache/cache"
	"k8s.io/apiserver/pkg/server/healthz"

	"github.com/go-logr/glogr"
	"github.com/golang/glog"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/version"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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
	logf.SetLogger(glogr.New())
	rand.Seed(time.Now().UnixNano())
	fmt.Println(version.String())
	options, err := getOptions()
	if err != nil {
		glog.Fatal(err)
	}
	if options.ShowVersion {
		os.Exit(0)
	}

	restCfg, err := buildRestConfig(options)
	if err != nil {
		glog.Fatal(err)
	}
	mgr, err := manager.New(restCfg, manager.Options{
		Namespace:               options.WatchNamespace,
		SyncPeriod:              &options.SyncPeriod,
		LeaderElection:          options.LeaderElection,
		LeaderElectionID:        options.LeaderElectionID,
		LeaderElectionNamespace: options.LeaderElectionNamespace,
	})
	if err != nil {
		glog.Fatal(err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	cc := cache.NewConfig(5 * time.Minute)
	reg.MustRegister(cc.NewCacheCollector(collectors.PrometheusNamespace))
	mc, err := metric.NewCollector(reg, options.config.IngressClass)
	if err != nil {
		glog.Fatal(err)
	}
	mc.Start()

	cloud := aws.New(options.AWSAPIMaxRetries, options.AWSAPIDebug, options.config.ClusterName, mc, cc)
	if err := controller.Initialize(&options.config, mgr, mc, cloud); err != nil {
		glog.Fatal(err)
	}

	mux := http.NewServeMux()
	if options.ProfilingEnabled {
		registerProfiler(mux)
	}
	registerHealthz(mux, &aws.HealthChecker{Cloud: cloud})
	registerMetrics(mux, reg)
	registerHandlers(mux)
	go startHTTPServer(options.HealthzPort, mux)

	glog.Fatal(mgr.Start(signals.SetupSignalHandler()))
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

func registerHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/build", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		b, _ := json.Marshal(version.String())
		_, _ = w.Write(b)
	})

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
		if err != nil {
			glog.Errorf("Unexpected error: %v", err)
		}
	})
}

func registerHealthz(mux *http.ServeMux, awsChecker *aws.HealthChecker) {
	healthz.InstallHandler(mux, healthz.PingHealthz, awsChecker)
}

func registerMetrics(mux *http.ServeMux, reg *prometheus.Registry) {
	mux.Handle(
		"/metrics",
		promhttp.InstrumentMetricHandler(
			reg,
			promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		),
	)
}

func registerProfiler(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/heap", pprof.Index)
	mux.HandleFunc("/debug/pprof/mutex", pprof.Index)
	mux.HandleFunc("/debug/pprof/goroutine", pprof.Index)
	mux.HandleFunc("/debug/pprof/threadcreate", pprof.Index)
	mux.HandleFunc("/debug/pprof/block", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func startHTTPServer(port int, mux *http.ServeMux) {
	server := &http.Server{
		Addr:              fmt.Sprintf(":%v", port),
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	glog.Fatal(server.ListenAndServe())
}
