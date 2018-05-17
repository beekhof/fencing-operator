package stub

import (
	"fmt"
	"time"
	"github.com/beekhof/fencing-operator/pkg/apis/fencing/v1alpha1"
	"github.com/beekhof/fencing-operator/pkg/config"

	"github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1batch "k8s.io/api/batch/v1"

	"github.com/operator-framework/operator-sdk/pkg/sdk/action"
	"github.com/operator-framework/operator-sdk/pkg/sdk/query"
	"github.com/operator-framework/operator-sdk/pkg/sdk/types"
)

const (
	secretsDir = "/etc/fencing/secrets/"
)

var (
	fencingConfigs = map[string]*v1alpha1.FencingConfig{}
)

func (h *Handler) HandleConfigMap(ctx types.Context, configmap *v1.ConfigMap, deleted bool) error {
	// Maintain a list of FencingConfig objects for HandleFencingRequest() to use

	if deleted {
		logrus.Infof("Deleting %v", configmap.Name)
		fencingConfigs[configmap.Name] = nil
		return nil
	}

	if fencingConfigs[configmap.Name] != nil {
		logrus.Infof("Updating %v", configmap.Name)
	}

	methods := []v1alpha1.FencingMethod{}
	cfg, err :=  config.NewConfigFromString(configmap.Data["fencing-config"])
	if err != nil {
		return err
	}

	logrus.Infof("Creating %v", configmap.Name)
	for _, subcfg := range cfg.GetSubConfigArray("methods") {
		err, m := newFencingMethodFromConfig(subcfg)
		if err != nil {
			return err
		}
		methods = append(methods, *m)
	}

	fencingConfigs[configmap.Name] = &v1alpha1.FencingConfig{
		NodeSelector: cfg.GetMapOfStrings("nodeSelector"),
		Methods: methods,
	}
	logrus.Infof("Created %v: %v", configmap.Name, fencingConfigs[configmap.Name])
	return nil
}

func (h *Handler) HandleFencingRequest(ctx types.Context, req *v1alpha1.FencingRequest, deleted bool) error {
	if deleted {
		// By default Jobs associated with this request will
		// be deleted as well due to the ownerRef
		//
		// If they're still around its because the caller specified
		// --cascade=false which should be respected
		return nil
	}

	err, cfg := chooseFencingConfig(req)
	if err != nil {
		logrus.Errorf("No valid fencing configurations for %v (%v)", req.Spec.Target, req.Name)
		req.SetFinalResult(v1alpha1.RequestFailedNoConfig, err)
		return err
	}

	err, method := chooseFencingMethod(req, cfg)
	if err != nil {
		logrus.Errorf("All configured fencing methods have failed for %v (%v)", req.Spec.Target, req.Name)
		req.SetFinalResult(v1alpha1.RequestFailed, err)
		return err
	} else if isFencingMethodActive(req, *method) {
		logrus.Infof("Waiting until %v/%v completes for %v", cfg.Name, method.Name, req.Name)
		return nil
	}

	err, job := createFencingJob(req, cfg, *method)
	if err != nil {
		req.AddResult(method.Name, v1alpha1.RequestFailed, err)
		return err
	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

	err = wait.ExponentialBackoff(backoff, func() (bool, error) {
		if err := action.Create(job); err != nil && !errors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			return false, err
		}
		return true, nil
	})
	
	if err != nil {
		logrus.Errorf("Failed to schedule fencing job %v for %s (%v): %v", method.Name, req.Spec.Target, req.UID, err)
	} else {
		logrus.Infof("Scheduled fencing job %v for request %s (%v)", method.Name, req.Spec.Target, req.UID)
	}
	return err
}

func nodeInList(name string, nodes []v1.Node) bool {
	for _, node := range nodes {
		if node.Name == name {
			return true
		}
	}
	return false
}

func chooseFencingConfig(req *v1alpha1.FencingRequest) (error, *v1alpha1.FencingConfig) {
	labelThreshold := -1
	var chosen *v1alpha1.FencingConfig = nil

	if req.Status.Config != nil {
		chosen = fencingConfigs[*req.Status.Config]
		if chosen != nil {
			return nil, chosen
		}
	}

	req.Status.Config = nil

	for _, cfg := range fencingConfigs {
		if len(cfg.NodeSelector) > labelThreshold {
			err, nodes := ListNodes(cfg.NodeSelector)
			if err == nil && nodeInList(req.Spec.Target, nodes) {
				chosen = cfg
				labelThreshold = len(cfg.NodeSelector)
			}
		}
	}

	if chosen != nil {
		logrus.Infof("Chose %v for fencing %v (%v)", chosen.Name, req.Spec.Target, req.Name)
		req.Status.Config = &chosen.Name
		return nil, chosen
	}

	return fmt.Errorf("No valid config for %v", req.Spec.Target), nil
}

func isFencingMethodActive(req *v1alpha1.FencingRequest, method v1alpha1.FencingMethod) bool  {
	err, jobs := ListJobs(req, method.Name)
	if err != nil {
		logrus.Errorf("Error retrieving the list of jobs: %v", err)
		return false
	}
	for _, job := range jobs {
		if job.Status.Active > 0 {
			logrus.Errorf("Found job %v active (%v)", job.Name, job.UID)
			return true
		}
	}
	return false
}

func chooseFencingMethod(req *v1alpha1.FencingRequest, cfg *v1alpha1.FencingConfig) (error, *v1alpha1.FencingMethod) {
	// TODO: Verify that yaml ordering of methods is preserved
	next := false
	for _, method := range cfg.Methods {
		if req.Status.ActiveMethod == nil {
			return nil, &method
		} else if *req.Status.ActiveMethod == method.Name && isFencingMethodActive(req, method) {
			return nil, &method
			
		} else if *req.Status.ActiveMethod == method.Name {
			next = true
		} else if next {
			return nil, &method
		}			
	}

	return fmt.Errorf("No remaining methods available"), nil
}

func createFencingJob(req *v1alpha1.FencingRequest, cfg *v1alpha1.FencingConfig, method v1alpha1.FencingMethod) (error, *v1batch.Job) {
	// Create a Job with a container for each mechanism

	volumes := []v1.Volume{}
	containers := []v1.Container{}

	labels := req.JobLabels(method.Name)

	for _, mech := range method.Mechanisms {
		err, c := mech.CreateContainer(req.Spec.Target, secretsDir)
		if err != nil {
			return fmt.Errorf("Method %s aborted: %v", method.Name, err), nil
		}

		// Create volumes for any sensitive parameters that are stored as k8s secrets
		for key, s := range mech.Secrets {
			volumes = append(volumes, v1.Volume{
				Name: "secret-" + key,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: s,
					},
				},
			})
		}

		// Mount the secrets into the container so they can be easily retrieved
		for key, s := range mech.Secrets {
			c.VolumeMounts = append(c.VolumeMounts, v1.VolumeMount{
				Name:      "secret-" + key,
				ReadOnly:  true,
				MountPath: secretsDir + s,
			})
		}

		// Add the container to the PodSpec
		containers = append(containers, *c)
	}

	// Parallel Jobs with a fixed completion count
	// - https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	return nil, &v1batch.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%v-job-", req.Name),
			Namespace:    "default",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(req, schema.GroupVersionKind{
					Group:   v1alpha1.SchemeGroupVersion.Group,
					Version: v1alpha1.SchemeGroupVersion.Version,
					Kind:    "FencingRequest",
				}),
			},
			Labels: labels,
		},
		Spec: v1batch.JobSpec{
			BackoffLimit: &method.Retries,
			// Parallelism: 1,
			// Completions: 1, // len(containers),
			Template: v1.PodTemplateSpec {
				Spec: v1.PodSpec {
					Containers: containers,
					RestartPolicy: v1.RestartPolicyOnFailure,
					Volumes: volumes,
				},
			},
		},
	}
}

func ListNodes(selector map[string]string) (error, []v1.Node) {
	nodes := &v1.NodeList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
	}

	labelSelector := labels.SelectorFromSet(selector).String()
	listOptions := query.WithListOptions(&metav1.ListOptions{
		LabelSelector:        labelSelector,
		IncludeUninitialized: false,
	})

	err := query.List("--all-namespaces", nodes, listOptions)
	if err != nil {
		logrus.Errorf("failed to get node list: %v", err)
	}
	return err, nodes.Items
}

func ListJobs(req *v1alpha1.FencingRequest, method string) (error, []v1batch.Job) {
	
	jobs := &v1batch.JobList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "v1",
		},
	}

	labelSelector := labels.SelectorFromSet(req.JobLabels(method)).String()
	listOptions := query.WithListOptions(&metav1.ListOptions{
		LabelSelector:        labelSelector,
		IncludeUninitialized: false,
	})

	err := query.List("--all-namespaces", jobs, listOptions)
	if err != nil {
		logrus.Errorf("failed to get job list: %v", err)
	}
	return err, jobs.Items
}

func newFencingMethodFromConfig(cfg *config.Config) (error, *v1alpha1.FencingMethod)  {
	mechanisms := []v1alpha1.FencingMechanism{}
	seconds := int32(cfg.GetInt("requireAfterSeconds"))

	for _, subcfg := range cfg.GetSubConfigArray("mechanisms") {
		err, m := newFencingMechanismFromConfig(subcfg)
		if err != nil {
			return err, nil
		}
		mechanisms = append(mechanisms, *m)
	}

	return nil, &v1alpha1.FencingMethod{
		Name:       cfg.GetString("name"),
		Retries:    1,
		RequireAfterSeconds: &seconds,
		Mechanisms: mechanisms,
	}
//	method.GetSliceOfStrings("namespaces"),
//	method.GetBool("fail_on_error"),
//	method.GetString("openshift.namespace")
}

func newFencingMechanismFromConfig(cfg *config.Config) (error, *v1alpha1.FencingMechanism)  {
	dcs := []v1alpha1.FencingDynamicConfig{}
	seconds := int32(cfg.GetInt("timeoutSeconds"))

	for _, subcfg := range cfg.GetSubConfigArray("dynamicConfig") {
		err, d := newDynamicAttributeFromConfig(subcfg)
		if err != nil {
			return err, nil
		}
		dcs = append(dcs, *d)
	}

	return nil, &v1alpha1.FencingMechanism{
		Driver:         cfg.GetString("driver"),
		Module:         cfg.GetString("module"),
		Image:          cfg.GetString("image"),
		PassTargetAs:   cfg.GetString("passTargetAs"),
		TimeoutSeconds: &seconds,
		Config:         cfg.GetMapOfStrings("config"),
		DynamicConfig:  dcs,
		Secrets:        cfg.GetMapOfStrings("secrets"),
	}
}

func newDynamicAttributeFromConfig(cfg *config.Config) (error, *v1alpha1.FencingDynamicConfig)  {
	return nil, &v1alpha1.FencingDynamicConfig{
		Field:   cfg.GetString("field"),
		Default: cfg.GetString("default"),
		Values:  cfg.GetMapOfStrings("values"),
	}
}

