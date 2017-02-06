package controller

import (
	"flag"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/api"
	client "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	clientcmdapi "k8s.io/kubernetes/pkg/client/unversioned/clientcmd/api"
)

const (
	// High enough QPS to fit all expected use cases. QPS=0 is not set here, because
	// client code is overriding it.
	defaultQPS = 1e6
	// High enough Burst to fit all expected use cases. Burst=0 is not set here, because
	// client code is overriding it.
	defaultBurst = 1e6
)

// KubeClient re-parses the parameters since the core package does not expose the client information we need
func KubeClient() *client.Clientset {
	var (
		flags = pflag.NewFlagSet("", pflag.ExitOnError)

		apiserverHost = flags.String("apiserver-host", "", "The address of the Kubernetes Apiserver "+
			"to connect to in the format of protocol://address:port, e.g., "+
			"http://localhost:8080. If not specified, the assumption is that the binary runs inside a "+
			"Kubernetes cluster and local discovery is attempted.")
		kubeConfigFile = flags.String("kubeconfig", "", "Path to kubeconfig file with authorization and master location information.")

		_ = flags.String("default-backend-service", "",
			`Service used to serve a 404 page for the default backend. Takes the form
    	namespace/name. The controller uses the first node port of this Service for
    	the default backend.`)

		_ = flags.String("ingress-class", "",
			`Name of the ingress class to route through this controller.`)

		_ = flags.String("configmap", "",
			`Name of the ConfigMap that contains the custom configuration to use`)

		_ = flags.String("publish-service", "",
			`Service fronting the ingress controllers. Takes the form
 		namespace/name. The controller will set the endpoint records on the
 		ingress objects to reflect those on the service.`)

		_ = flags.String("tcp-services-configmap", "",
			`Name of the ConfigMap that contains the definition of the TCP services to expose.
		The key in the map indicates the external port to be used. The value is the name of the
		service with the format namespace/serviceName and the port of the service could be a 
		number of the name of the port.
		The ports 80 and 443 are not allowed as external ports. This ports are reserved for the backend`)

		_ = flags.String("udp-services-configmap", "",
			`Name of the ConfigMap that contains the definition of the UDP services to expose.
		The key in the map indicates the external port to be used. The value is the name of the
		service with the format namespace/serviceName and the port of the service could be a 
		number of the name of the port.`)

		_ = flags.Duration("sync-period", 60*time.Second,
			`Relist and confirm cloud resources this often.`)

		_ = flags.String("watch-namespace", api.NamespaceAll,
			`Namespace to watch for Ingress. Default is to watch all namespaces`)

		_ = flags.Int("healthz-port", 10254, "port for healthz endpoint.")

		_ = flags.Bool("profiling", true, `Enable profiling via web interface host:port/debug/pprof/`)

		_ = flags.String("default-ssl-certificate", "", `Name of the secret 
		that contains a SSL certificate to be used as default for a HTTPS catch-all server`)

		_ = flags.String("health-check-path", "/healthz", `Defines 
		the URL to be used as health check inside in the default server in NGINX.`)

		_ = flags.Bool("update-status", true, `Indicates if the 
		ingress controller should update the Ingress status IP/hostname. Default is true`)
	)

	flags.Usage = func() {}

	flags.AddGoFlagSet(flag.CommandLine)
	flags.Parse(os.Args)

	kubeClient, err := createApiserverClient(*apiserverHost, *kubeConfigFile)
	if err != nil {
		handleFatalInitError(err)
	}

	return kubeClient
}

// createApiserverClient creates new Kubernetes Apiserver client. When kubeconfig or apiserverHost param is empty
// the function assumes that it is running inside a Kubernetes cluster and attempts to
// discover the Apiserver. Otherwise, it connects to the Apiserver specified.
//
// apiserverHost param is in the format of protocol://address:port/pathPrefix, e.g.http://localhost:8001.
// kubeConfig location of kubeconfig file
func createApiserverClient(apiserverHost string, kubeConfig string) (*client.Clientset, error) {

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfig},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: apiserverHost}})

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	cfg.QPS = defaultQPS
	cfg.Burst = defaultBurst
	cfg.ContentType = "application/vnd.kubernetes.protobuf"

	glog.Infof("Creating API server client for %s", cfg.Host)

	client, err := client.NewForConfig(cfg)

	if err != nil {
		return nil, err
	}
	return client, nil
}

/**
 * Handles fatal init error that prevents server from doing any work. Prints verbose error
 * message and quits the server.
 */
func handleFatalInitError(err error) {
	glog.Fatalf("Error while initializing connection to Kubernetes apiserver. "+
		"This most likely means that the cluster is misconfigured (e.g., it has "+
		"invalid apiserver certificates or service accounts configuration). Reason: %s\n"+
		"Refer to the troubleshooting guide for more information: "+
		"https://github.com/kubernetes/ingress/blob/master/docs/troubleshooting.md", err)
}
