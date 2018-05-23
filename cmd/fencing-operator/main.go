package main

import (
	"os"
	"net"
	"flag"
	"time"
	"context"
	"runtime"

	stub "github.com/beekhof/fencing-operator/pkg/stub"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	
	"github.com/sirupsen/logrus"
	"github.com/beekhof/fencing-operator/pkg/constants"
)

var (
	mode       string

	chaosLevel int

	printVersion bool

	createCRD bool
)

func init() {
	//flag.StringVar(&debug.DebugFilePath, "debug-logfile-path", "", "only for a self hosted cluster, the path where the debug logfile will be written, recommended to be under: /var/tmp/etcd-operator/debug/ to avoid any issue with lack of write permissions")
	flag.StringVar(&mode, "mode", "watcher", "Possible values: node watcher, executioner, [all]")
	// chaos level will be removed once we have a formal tool to inject failures.
	flag.IntVar(&chaosLevel, "chaos-level", -1, "DO NOT USE IN PRODUCTION - level of chaos injected into the etcd clusters created by the operator.")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")
	flag.BoolVar(&createCRD, "create-crd", true, "The operator will not create the EtcdCluster CRD when this flag is set to false.")
	flag.Parse()
}

func Version() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	namespace := os.Getenv(constants.EnvOperatorPodNamespace)
	if len(namespace) == 0 {
		logrus.Fatalf("must set env (%s)", constants.EnvOperatorPodNamespace)
	}
	name := os.Getenv(constants.EnvOperatorPodName)
	if len(name) == 0 {
		logrus.Fatalf("must set env (%s)", constants.EnvOperatorPodName)
	}
	image := os.Getenv(constants.EnvOperatorPodImage)
	if len(image) == 0 {
		logrus.Fatalf("must set env (%s)", constants.EnvOperatorPodImage)
	}

	Version()
 	if printVersion {
		os.Exit(0)		
	}

	id, err := os.Hostname()
	if err != nil {
		logrus.Fatalf("failed to get hostname: %v", err)
	}

	kubecli, _ := mustNewKubeClientAndConfig()

	rl, err := resourcelock.New(resourcelock.EndpointsResourceLock,
		namespace,
		"fencing-operator",
		kubecli.CoreV1(),
		resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: createRecorder(kubecli, name, namespace),
		})
	if err != nil {
		logrus.Fatalf("error creating lock: %v", err)
	}

	leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				logrus.Fatalf("leader election lost")
			},
		},
	})
	
	panic("unreachable")
}


func run(stop <-chan struct{}) {
	switch mode {
	case "watcher":
		sdk.Watch("v1", "Node", "default", 5)
		sdk.Watch("v1", "Event", "default", 5)
	case "executioner":
		sdk.Watch("v1", "ConfigMap", "default", 5)
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingRequest", "default", 5)
	case "all":
		sdk.Watch("v1", "Node", "default", 5)
		sdk.Watch("v1", "Event", "default", 5)
		sdk.Watch("v1", "ConfigMap", "default", 5)
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingRequest", "default", 5)
	}
	sdk.Handle(stub.NewHandler())
	sdk.Run(context.TODO())
}

// mustNewKubeClientAndConfig returns the in-cluster config and kubernetes client
// or if KUBERNETES_CONFIG is given an out of cluster config and client
func mustNewKubeClientAndConfig() (kubernetes.Interface, *rest.Config) {
	var cfg *rest.Config
	var err error
	if os.Getenv(k8sutil.KubeConfigEnvVar) != "" {
		cfg, err = outOfClusterConfig()
	} else {
		cfg, err = inClusterConfig()
	}
	if err != nil {
		panic(err)
	}
	return kubernetes.NewForConfigOrDie(cfg), cfg
}

// inClusterConfig returns the in-cluster config accessible inside a pod
func inClusterConfig() (*rest.Config, error) {
	// Work around https://github.com/kubernetes/kubernetes/issues/40973
	// See https://github.com/coreos/etcd-operator/issues/731#issuecomment-283804819
	if len(os.Getenv("KUBERNETES_SERVICE_HOST")) == 0 {
		addrs, err := net.LookupHost("kubernetes.default.svc")
		if err != nil {
			return nil, err
		}
		os.Setenv("KUBERNETES_SERVICE_HOST", addrs[0])
	}
	if len(os.Getenv("KUBERNETES_SERVICE_PORT")) == 0 {
		os.Setenv("KUBERNETES_SERVICE_PORT", "443")
	}
	return rest.InClusterConfig()
}

func outOfClusterConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv(k8sutil.KubeConfigEnvVar)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	return config, err
}

func createRecorder(kubecli kubernetes.Interface, name, namespace string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubecli.Core().RESTClient()).Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: name})
}
