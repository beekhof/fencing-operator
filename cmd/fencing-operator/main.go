package main

import (
	"os"
	"flag"
	"context"
	"runtime"

	stub "github.com/beekhof/fencing-operator/pkg/stub"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

	"github.com/sirupsen/logrus"
)

var (
	namespace  string
	name       string
	mode       string

	chaosLevel int

	printVersion bool

	createCRD bool
)

func init() {
	//flag.StringVar(&debug.DebugFilePath, "debug-logfile-path", "", "only for a self hosted cluster, the path where the debug logfile will be written, recommended to be under: /var/tmp/etcd-operator/debug/ to avoid any issue with lack of write permissions")
	flag.StringVar(&mode, "mode", "executioner", "Possible values: node watcher, executioner, [all]")
	// chaos level will be removed once we have a formal tool to inject failures.
	flag.IntVar(&chaosLevel, "chaos-level", -1, "DO NOT USE IN PRODUCTION - level of chaos injected into the etcd clusters created by the operator.")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")
	flag.BoolVar(&createCRD, "create-crd", true, "The operator will not create the EtcdCluster CRD when this flag is set to false.")
	flag.Parse()
}

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	namespace = os.Getenv(constants.EnvOperatorPodNamespace)
	if len(namespace) == 0 {
		logrus.Fatalf("must set env (%s)", constants.EnvOperatorPodNamespace)
	}
	name = os.Getenv(constants.EnvOperatorPodName)
	if len(name) == 0 {
		logrus.Fatalf("must set env (%s)", constants.EnvOperatorPodName)
	}

	printVersion()
 	if printVersion {
		os.Exit(0)		
	}

	switch mode {
	case "watcher":
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingSet", "default", 5)
		sdk.Watch("k8s.io.api.core/v1", "Node", "default", 5)
		sdk.Watch("k8s.io.api.core/v1", "Event", "default", 5)
	case "executioner":
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingRequest", "default", 5)
	case "all":
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingRequest", "default", 5)
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingSet", "default", 5)
		sdk.Watch("k8s.io.api.core/v1", "Node", "default", 5)
		sdk.Watch("k8s.io.api.core/v1", "Event", "default", 5)
	}
	sdk.Handle(stub.NewHandler())
	sdk.Run(context.TODO())
}
