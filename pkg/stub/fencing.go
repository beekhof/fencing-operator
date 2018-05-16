package stub

import (
	"github.com/beekhof/fencing-operator/pkg/apis/fencing/v1alpha1"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/operator-framework/operator-sdk/pkg/sdk/action"
)

const {
	secretsDir := "/etc/fencing/secrets/"
}

var {
	configList := map[string]*FencingConfig{}
}

func (h *Handler) HandleConfigMap(ctx types.Context, node *v1.Node, deleted bool) error {
	// TODO: Maintain a list of FencingConfig objects for HandleFencingRequest() to use
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

	err, config := chooseFencingConfig(req)
	if err != nil {
		logrus.Errorf("No valid fencing configurations for %v (%v)", req.Target, req.UID)
		req.SetFinalResult(v1alpha1.RequestFailedNoConfig, err)
		return err
	}

	err, method := chooseFencingMethod(req, config)
	if err != nil {
		logrus.Errorf("All configured fencing methods have failed for %v (%v)", req.Target, req.UID)
		req.SetFinalResult(v1alpha1.RequestFailed, err)
		return err
	} else if h.isFencingMethodActive(req, config, method) {
		return nil
	}

	err, job := createFencingJob(req, config, method)

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

	err = wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := action.Create(job)
		if err != nil && !errors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			return false, err
		}
		return true, nil
	})
	
	if err != nil {
		logrus.Errorf("Failed to schedule fencing job %v for %s (%v): %v", method.Name, req.Target, req.UID, err)
	} else {
		logrus.Infof("Scheduled fencing job %v for request %s (%v)", method.Name, req.Target, req.UID)
	}
	return err
}

func nodeInList(name string, nodes []*v1.Node) bool {
	for _, node := range nodes {
		if node.Name == req.Target {
			return true
		}
	}
	return false
}

func (h *Handler) chooseFencingConfig(req *v1alpha1.FencingRequest) error, *v1alpha1.FencingConfig {
	labelThreshold := -1
	var chosen *v1alpha1.FencingConfig = nil

	if req.Status.Config != nil {
		chosen = configList[req.Status.Config]
		if chosen != nil {
			return nil, chosen
		}
	}

	req.Status.Config = nil

	for name, config := range configList {
		if len(config.NodeSelector) > labelThreshold {
			err, nodes := ListNodes(config.NodeSelector)
			if err == nil && nodeInList(req.Target, nodes) {
				chosen = config
				labelThreshold = len(config.NodeSelector)
			}
		}
	}

	if chosen != nil {
		logrus.Infof("Chose %v for fencing %v (%v)", config.Name, req.Target, req.Name)
		req.Status.Config = chosen.Name
		return nil, chosen
	}

	return fmt.Errorf("No valid config for %v", req.Target), nil
}

func (h *Handler) isFencingMethodActive(req *v1alpha1.FencingRequest, config *v1alpha1.FencingConfig, method *v1alpha1.FencingMethod) bool  {
	err, jobs := ListJobs(req, method.Name)
	for _, job := range jobs {
		if job.Status.Active > 0 {
			return true
		}
	}
	return false
}

func (h *Handler) chooseFencingMethod(req *v1alpha1.FencingRequest, config *v1alpha1.FencingConfig) error, *v1alpha1.FencingMethod {
	// TODO: Verify that yaml ordering of methods is preserved
	next := false
	for _, method := range config.Methods {
		if req.Status.ActiveMethod == nil {
			return nil, method
		} else if req.Status.ActiveMethod == method.Name && h.isFencingMethodActive(req, config, method) {
			return nil, method
			
		} else if req.Status.ActiveMethod == method.Name {
			next = true
		} else if next {
			return nil, method
		}			
	}

	return fmt.Errorf("No remaining methods available"), nil
}

func (h *Handler) createFencingJob(req *v1alpha1.FencingRequest, config *v1alpha1.FencingConfig, method *v1alpha1.FencingMethod) error {
	// Create a Job with a container for each mechanism

	labels := req.JobLabels(method.Name)
	volumes := []v1.Volumes{}

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

	containers := []v1.Container{}
	for _, mech := range method.Mechanisms {
		c := containerFromMechanism(mech)

		// Mount the secrets into the container so they can be easily retrieved
		for key, s := range mech.Secrets {
			c.VolumeMounts = append(c.VolumeMounts, v1.VolumeMount{
				Name:      "secret-" + key,
				ReadOnly:  true,
				MountPath: fencingSecretsDir + s,
			})
		}

		// Add the container to the PodSpec
		containers = append(containers, c)
	}

	// Parallel Jobs with a fixed completion count
	// - https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/
	return &v1.Pod{
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
		Spec: &v1.JobSpec{
			BackoffLimit: method.Retries,
			// Parallelism: 1,
			// Completions: 1, // len(containers),
			Template: &v1.PodSpec {
				Containers: containers,
				RestartPolicy: OnFailure,
				Volumes: volumes,
			},
		},
	}
}

func containerFromMechanism(mechanism *v1alpha1.FencingMethod) *v1.Pod {
	switch mechanism.Driver {
	case "baremetal":
		return &v1.Container{
			{
				Name:    "busybox",
				Image:   "busybox",
				Command: []string{"echo", fmt.Sprintf("/sbin/fence_%v --some-args=and --values", mechanism.Module)},
			},
		}
	}
	return &v1.Container{
		{
			Name:    "busybox",
			Image:   "busybox",
			Command: []string{"echo", fmt.Sprintf("%v/%v complete", mechanism.Driver, mechanism.Module)},
		},
	}
}

func (fr *FencingRequest) AddResult(result int32, err error) {
	fr.Updates = append(fr.Updates, &v1alpha1.FencingRequestStatusUpdate {
		Timestamp: time.Now(),
		Message: fmt.Sprintf("%v %v", result, err),
	})
	fr.Update("AddResult")
}

func (fr *FencingRequest) SetFinalResult(result int32, err error) {
	fr.Status.Complete = true
	fr.Status.Result = result
	fr.AddResult(result, err)
}

func (fr *FencingRequest) Update(prefix string) error {
// Do we need to modify a copy so we can test for changes before doing an update?
//	if reflect.DeepEqual(fr.Status, saved.status) {
//		return nil
//	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.2,
		Steps:    5,
	}

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := action.Update(fr)
		if err != nil && !errors.IsAlreadyExists(err) {
			// Retry it as errors writing to the API server are common
			return false, err
		}
		return true, nil
	})
	
	if err != nil {
		logrus.Errorf("%v: failed to update CR %v: %v", prefix, fr.Name, err)
	} else {
		logrus.Debugf("%v: updated CR %v", prefix, fr.Name)
	}

//	saved = fr
	return err
}


func (fr *FencingRequest)JobLabels(method *string) map[string]string {
	labels := map[string]string{
		"app": "fencing-operator",
		"target": req.Target,
		"request": req.Name,
	}
	if fr.Status.Config != nil {
		labels["config"] = fr.Status.Config
	}
	if method != nil {
		labels["method"] = method
	}
	return labels
}

func ListNodes(selector map[string]string) (error, []v1.Job) {
	nodes := &v1.NodeList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
	}

	opt := &metav1.ListOptions{MatchLabels: selector}
	err := query.List("--all-namespaces", nodes, query.WithListOptions(opt))
	if err != nil {
		logrus.Errorf("failed to get node list: %v", err)
	}
	return err, nodes.Items
}

func ListJobs(req *v1alpha1.FencingRequest, method *string) (error, []v1.Job) {
	
	jobs := &v1.JobList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "v1",
		},
	}

	sel := fmt.Sprintf("spec.nodeName=%s", node.Name)
//	opt := &metav1.ListOptions{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},}
	opt := &metav1.ListOptions{MatchLabels: req.JobLabels(method)}
	err := query.List("--all-namespaces", jobs, query.WithListOptions(opt))
	if err != nil {
		logrus.Errorf("failed to get job list: %v", err)
	}
	return err, jobs.Items
}