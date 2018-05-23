package k8sutil

import (
	"os"
	"net"
	"fmt"
	"time"
	
	"github.com/sirupsen/logrus"
	
	"github.com/operator-framework/operator-sdk/pkg/sdk/query"
	"github.com/operator-framework/operator-sdk/pkg/sdk/action"
	sdkTypes "github.com/operator-framework/operator-sdk/pkg/sdk/types"
	sdkutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"

	api "github.com/beekhof/fencing-operator/pkg/apis/fencing/v1alpha1"
	

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

var (
	defaultBackoff = wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

)

// mustNewKubeClientAndConfig returns the in-cluster config and kubernetes client
// or if KUBERNETES_CONFIG is given an out of cluster config and client
func mustNewKubeClientAndConfig() (kubernetes.Interface, *rest.Config) {
	var cfg *rest.Config
	var err error
	if os.Getenv(sdkutil.KubeConfigEnvVar) != "" {
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
	kubeconfig := os.Getenv(sdkutil.KubeConfigEnvVar)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	return config, err
}

func createRecorder(kubecli kubernetes.Interface, name, namespace string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubecli.Core().RESTClient()).Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: name})
}

func SingletonWith(id string, name string, namespace string, role string, startedLeadingFunc func(stop <-chan struct{})) {
	kubecli, _ := mustNewKubeClientAndConfig()

	rl, err := resourcelock.New(resourcelock.EndpointsResourceLock,
		namespace,
		role,
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
			OnStartedLeading: startedLeadingFunc,
			OnStoppedLeading: func() {
				logrus.Fatalf("leader election lost")
			},
		},
	})
}

func RegisterCRD() error {
	kubeconfig, _ := inClusterConfig()
	extcli := apiextensionsclient.NewForConfigOrDie(kubeconfig)

	crd := CreateFencingCRD()

	err := wait.ExponentialBackoff(defaultBackoff, func() (bool, error) {
		_, err := extcli.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
		if err != nil && !errors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			return false, err
		}
		return true, nil
	})
	
	if err != nil {
		logrus.Errorf("Failed to create %v: %v", crd.ObjectMeta.Name, err)
		return err

	} else {
		logrus.Infof("Created %s", crd.ObjectMeta.Name)
	}

	return WaitCRDReady(extcli, api.FencingRequestCRDName)
}

func CreateFencingCRD() *apiextensionsv1beta1.CustomResourceDefinition {
	return &apiextensionsv1beta1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: apiextensionsv1beta1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: api.FencingRequestCRDName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   api.SchemeGroupVersion.Group,
			Version: api.SchemeGroupVersion.Version,
			// The objects are namespace scoped but CRDs are always cluster scoped
			Scope: apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: api.FencingRequestResourcePlural,
				Kind:   api.FencingRequestResourceKind,
				ShortNames: []string{api.FencingRequestResourceShort},
				//				Singular: "repl",
			},
		},
	}
}

func WaitCRDReady(extcli apiextensionsclient.Interface, crdName string) error {

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: apiextensionsv1beta1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
		},
	}

	err := wait.ExponentialBackoff(defaultBackoff, func() (bool, error) {
		err := query.Get(crd)
		if err != nil {
                        return false, err
                }
                for _, cond := range crd.Status.Conditions {
                        switch cond.Type {
                        case apiextensionsv1beta1.Established:
                                if cond.Status == apiextensionsv1beta1.ConditionTrue {
                                        return true, nil
                                }
                        case apiextensionsv1beta1.NamesAccepted:
                                if cond.Status == apiextensionsv1beta1.ConditionFalse {
                                        return false, fmt.Errorf("Name conflict: %v", cond.Reason)
                                }
                        }
                }
		return false, nil
	})
	
	if err != nil {
		return fmt.Errorf("CRD creation failed: %v", err)
	}
        return nil
}

func CreateObject(object sdkTypes.Object, cause string) error {

	err := wait.ExponentialBackoff(defaultBackoff, func() (bool, error) {
		err := action.Create(object)
		if err != nil && !errors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			return false, err
		}
		return true, nil
	})
	
	return err
}
