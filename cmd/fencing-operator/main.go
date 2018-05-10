package main

import (
	"context"
	"runtime"

	stub "github.com/beekhof/fencing-operator/pkg/stub"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

	"github.com/sirupsen/logrus"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	printVersion()
	sdk.Watch("fencing.clusterlabs.org/v1alpha1", "FencingSet", "default", 5)
	sdk.Watch("k8s.io.api.core/v1", "Node", "default", 5)
	sdk.Watch("k8s.io.api.core/v1", "Event", "default", 5)
	sdk.Handle(stub.NewHandler())
	sdk.Run(context.TODO())
}
