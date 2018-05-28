package main

import (
	"os"
	"fmt"
	"flag"
	"context"
	"runtime"

	stub "github.com/beekhof/fencing-operator/pkg/stub"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	
	"github.com/sirupsen/logrus"
	"github.com/beekhof/fencing-operator/pkg/constants"
	"github.com/beekhof/fencing-operator/pkg/k8sutil"
)

var (
	mode       string
	namespace  string
	chaosLevel int

	printVersion bool

	createCRD bool
)

func init() {
	//flag.StringVar(&debug.DebugFilePath, "debug-logfile-path", "", "only for a self hosted cluster, the path where the debug logfile will be written, recommended to be under: /var/tmp/etcd-operator/debug/ to avoid any issue with lack of write permissions")
	flag.StringVar(&mode, "mode", "all", "Possible values: node watcher, executioner, [all]")
	// chaos level will be removed once we have a formal tool to inject failures.
	flag.IntVar(&chaosLevel, "chaos-level", -1, "DO NOT USE IN PRODUCTION - level of chaos injected into the etcd clusters created by the operator.")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")
	flag.BoolVar(&createCRD, "create-crd", true, "The operator will not create the FencingRequest CRD when this flag is set to false.")
	flag.Parse()
}

func Version() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	namespace = os.Getenv(constants.EnvOperatorPodNamespace)
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

	envmode := os.Getenv(constants.EnvOperatorPodMode)
	if len(envmode) != 0 {
		mode = envmode
	}

	Version()
 	if printVersion {
		os.Exit(0)		
	}

	id, err := os.Hostname()
	if err != nil {
		logrus.Fatalf("failed to get hostname: %v", err)
	}


	k8sutil.SingletonWith(id, name, namespace, fmt.Sprintf("fencing-operator-%s", mode), run)
	panic("unreachable")
}


func run(stop <-chan struct{}) {
	resyncInterval := 0 // Non-zero results in duplicate update notifications, even if nothing changed
	if createCRD {
		k8sutil.RegisterCRD()
	}

	switch mode {
	case "watcher":
		sdk.Watch("v1", "Node", "default", resyncInterval)
		sdk.Watch("v1", "Event", "default", resyncInterval)
	case "executioner":
		sdk.Watch("v1", "ConfigMap", namespace, resyncInterval)
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingRequest", namespace, resyncInterval)
	case "all":
		sdk.Watch("v1", "Node", "default", resyncInterval)
		sdk.Watch("v1", "Event", "default", resyncInterval)
		sdk.Watch("v1", "ConfigMap", namespace, resyncInterval)
		sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingRequest", namespace, resyncInterval)
	}
	sdk.Handle(stub.NewHandler())
	sdk.Run(context.TODO())
}
