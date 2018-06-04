package stub

import (
	"fmt"
	"time"

	"github.com/beekhof/fencing-operator/pkg/apis/fencing/v1alpha1"
	"github.com/beekhof/fencing-operator/pkg/config"
	"github.com/beekhof/fencing-operator/pkg/util"

	"github.com/sirupsen/logrus"

	v1batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

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

func (h *Handler) HandleFencingJob(ctx types.Context, req *v1alpha1.FencingRequest, job *v1batch.Job, deleted bool) error {
	if job.Status.Active == 0 && job.Status.Failed == 0 && job.Status.Succeeded == 0 {
		return nil // Not running yet
	} else if req.Status.Complete {
		logrus.Infof("Ignoring job %v for old %v request", job.Name, req.Name)
		return nil
	} else if req.Status.ActiveJob != nil && *req.Status.ActiveJob != job.Name {
		logrus.Infof("Ignoring old job %v for %v", job.Name, req.Name)
		return nil
	} else if job.Status.Active == 0 && job.Status.Failed != 0 && job.Status.Succeeded != 0 {
		req.AddResult(*req.Status.ActiveMethod, v1alpha1.MethodProgress, fmt.Errorf("%v complete", job.Name))
	}

	util.JsonLogObject("Updating", req)

	if job.Status.Active == 0 && job.Status.Failed == 0 {
		// Delete the relevant nodes/pods
		req.SetFinalResult(v1alpha1.MethodComplete, fmt.Errorf("%v complete", job.Name))
	}

	return h.HandleFencingRequest(ctx, req, false)
}

func (h *Handler) HandleJob(ctx types.Context, job *v1batch.Job, deleted bool) error {
	target := job.Labels["target"]
	owner := job.OwnerReferences[0].Name
	logrus.Infof("Updating %v/%v/%v: active=%v, failed=%v, succeeded=%v", job.Name, target, owner, job.Status.Active, job.Status.Failed, job.Status.Succeeded)

	util.JsonLogObject("Updated", job)
	node, err := getNode(target)
	if err != nil {
		return err
	}

	_, requests := listFencingRequests(node, owner)
	for _, request := range requests {
		return h.HandleFencingJob(ctx, &request, job, deleted)
	}

	return nil
}

func (h *Handler) HandleConfigMap(ctx types.Context, configmap *v1.ConfigMap, deleted bool) error {
	// Maintain a list of FencingConfig objects for HandleFencingRequest() to use
	if _, ok := configmap.Data["fencing-config"]; !ok {
		return nil
	}

	if deleted {
		logrus.Infof("Deleting %v", configmap.Name)
		fencingConfigs[configmap.Name] = nil
		return nil
	}

	if _, ok := fencingConfigs[configmap.Name]; ok {
		logrus.Infof("Updating %v", configmap.Name)
		fencingConfigs[configmap.Name] = nil // In case the new version is invalid

	} else {
		logrus.Infof("Creating %v", configmap.Name)
	}

	methods := []v1alpha1.FencingMethod{}
	//logrus.Infof("Data: %v", configmap.Data["fencing-config"])

	cfg, err := config.NewConfigFromString(configmap.Data["fencing-config"])
	if err != nil {
		logrus.Errorf("Bad config %v: %v", configmap.Name, err)
		return err
	}

	for _, subcfg := range cfg.GetSubConfigArray("methods") {
		err, m := v1alpha1.NewFencingMethodFromConfig(subcfg)
		if err != nil {
			logrus.Errorf("Bad fencing method: %v", err)
			return err
		}
		methods = append(methods, *m)
	}

	fencingConfigs[configmap.Name] = &v1alpha1.FencingConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: configmap.Name,
		},
		NodeSelector: cfg.GetMapOfStrings("nodeSelector"),
		Methods:      methods,
	}
	return nil
}

func (h *Handler) HandleFencingRequestError(req *v1alpha1.FencingRequest, method *v1alpha1.FencingMethod, err error, last bool) bool {
	if err != nil && last {
		logrus.Errorf("Failed to schedule last configured fencing job %v for %s (%v): %v", method.Name, req.Spec.Target, req.UID, err)
		req.SetFinalResult(v1alpha1.RequestFailed, err)
		return true

	} else if err != nil {
		logrus.Errorf("Failed to schedule fencing job %v for %s (%v): %v", method.Name, req.Spec.Target, req.UID, err)
		req.AddResult(method.Name, v1alpha1.RequestFailed, err)
		return true
	}
	return false
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

	if req.Status.Complete {
		return nil
	}

	err, cfg := chooseFencingConfig(req)
	if err != nil {
		logrus.Errorf("No valid fencing configurations for %v (%v)", req.Spec.Target, req.Name)
		req.SetFinalResult(v1alpha1.RequestFailedNoConfig, err)
		return err
	}

	err, method, last := chooseFencingMethod(req, cfg)
	if h.HandleFencingRequestError(req, method, err, last) {
		logrus.Errorf("All configured fencing methods have failed for %v (%v)", req.Spec.Target, req.Name)
		return err
	}

	if isFencingMethodActive(req, *method) {
		logrus.Infof("Waiting until %v/%v completes for %v", cfg.Name, method.Name, req.Name)
		return nil
	}

	err, job := createFencingJob(req, cfg, *method)
	if h.HandleFencingRequestError(req, method, err, last) {
		logrus.Errorf("All configured fencing methods have failed for %v (%v)", req.Spec.Target, req.Name)
		return err
	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

	util.JsonLogObject("Created Job", job)
	err = wait.ExponentialBackoff(backoff, func() (bool, error) {
		if err := action.Create(job); err != nil && !errors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			logrus.Infof("Creation failed: %v", err)
			return false, err
		}
		logrus.Infof("Job creation passed: %v/%v", job.Name, job.UID)
		req.Status.ActiveJob = &job.Name
		return true, nil
	})

	if h.HandleFencingRequestError(req, method, err, last) {
		return err
	}

	logrus.Infof("Scheduled fencing job %v for request %s (%v)", method.Name, req.Spec.Target, req.UID)
	req.AddResult(method.Name, v1alpha1.MethodProgress, fmt.Errorf("%v initiated as %v", method.Name, job.Name))
	return nil
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

func isFencingMethodActive(req *v1alpha1.FencingRequest, method v1alpha1.FencingMethod) bool {
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

func chooseFencingMethod(req *v1alpha1.FencingRequest, cfg *v1alpha1.FencingConfig) (error, *v1alpha1.FencingMethod, bool) {
	// TODO: Verify that yaml ordering of methods is preserved
	next := false
	max := len(cfg.Methods)
	for lpc, method := range cfg.Methods {
		last := (lpc + 1) == max
		if req.Status.ActiveMethod == nil {
			return nil, &method, last
		} else if *req.Status.ActiveMethod == method.Name && isFencingMethodActive(req, method) {
			return nil, &method, last

		} else if *req.Status.ActiveMethod == method.Name {
			next = true
		} else if next {
			return nil, &method, last
		}
	}

	return fmt.Errorf("No remaining methods available"), nil, true
}

func processSecrets(mech v1alpha1.FencingMechanism, c *v1.Container) []v1.Volume {
	volumes := []v1.Volume{}
	for key, s := range mech.Secrets {
		// Create volumes for any sensitive parameters that are stored as k8s secrets
		volumes = append(volumes, v1.Volume{
			Name: "secret-" + key,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: s,
				},
			},
		})

		// Relies on an ENTRYPOINT that looks for SECRETPATH_field=/path/to/file and add: --field=$(cat /path/to/file) to the command line
		c.Env = append(c.Env, v1.EnvVar{
			Name:  fmt.Sprintf("SECRETPATH_%s", key),
			Value: fmt.Sprintf("%s/%s", secretsDir, s),
		})

		// Mount the secrets into the container so they can be easily retrieved
		c.VolumeMounts = append(c.VolumeMounts, v1.VolumeMount{
			Name:      "secret-" + key,
			ReadOnly:  true,
			MountPath: secretsDir + s,
		})
	}
	return volumes
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

		for _, v := range processSecrets(mech, c) {
			volumes = append(volumes, v)
		}

		// Add the container to the PodSpec
		containers = append(containers, *c)
	}

	// Parallel Jobs with a fixed completion count
	// - https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	return nil, &v1batch.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%v-job-", req.Name),
			Namespace:    req.Namespace,
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
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers:    containers,
					RestartPolicy: v1.RestartPolicyOnFailure,
					Volumes:       volumes,
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
			APIVersion: "batch/v1",
		},
	}

	labelSelector := labels.SelectorFromSet(req.JobLabels(method)).String()
	listOptions := query.WithListOptions(&metav1.ListOptions{
		LabelSelector:        labelSelector,
		IncludeUninitialized: false,
	})

	//namespace := "--all-namespaces"
	err := query.List(req.Namespace, jobs, listOptions)
	if err != nil {
		logrus.Errorf("failed to get job list: %v", err)
	}
	return err, jobs.Items
}
