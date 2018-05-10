package stub

import (
	"fmt"
	
	"github.com/beekhof/fencing-operator/pkg/apis/fencing/v1alpha1"

	"github.com/operator-framework/operator-sdk/pkg/sdk/action"
	"github.com/operator-framework/operator-sdk/pkg/sdk/query"
	"github.com/operator-framework/operator-sdk/pkg/sdk/handler"
	"github.com/operator-framework/operator-sdk/pkg/sdk/types"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/util/sets"

//	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
//	"k8s.io/apimachinery/pkg/fields"
)

var (
	// TODO read supported source from node problem detector config - this issue is still WIP
	supportedNodeProblemSources = sets.NewString("abrt-notification", "abrt-adaptor", "docker-monitor", "kernel-monitor", "kernel")
)


func NewHandler() handler.Handler {
	return &Handler{}
}

type Handler struct {
	// Fill me
}

func (h *Handler) Handle(ctx types.Context, event types.Event) error {
	switch o := event.Object.(type) {
	case *v1.Node:
		h.HandleNode(ctx, o, event.Deleted)
		
	case *v1.Event:
		h.HandleEvent(ctx, o, event.Deleted)
		
	case *v1alpha1.FencingSet:
		h.HandleFencingSet(ctx, o, event.Deleted)
	}
	return nil
}

func (h *Handler) HandleNode(ctx types.Context, node *v1.Node, deleted bool) error {
	if deleted {
		logrus.Errorf("Node deleted : %v ", node)
	} else  {
		for _, condition := range node.Status.Conditions {
			if h.isNodeDirty(node, condition) {
				// If no pending fencing request, create a new one
//				h.createNewNodeFenceObject(node, nil)
			}
		}
	}
	return nil
}

func (h *Handler) HandleEvent(ctx types.Context, event *v1.Event, deleted bool) error {
	if event.Type != v1.EventTypeWarning || !supportedNodeProblemSources.Has(string(event.Source.Component)) {
		return nil
	}

	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      event.Source.Host,
		},
	}
	
	err := query.Get(node)
	if err != nil {
		logrus.Errorf("Failed to get node '%s': %s", event.Source.Host, err)
		return err
	}
//	h.createNewNodeFenceObject(node, nil)
	return nil
}

func (h *Handler) HandleFencingSet(ctx types.Context, set *v1alpha1.FencingSet, deleted bool) error {
	// Anything to actually do?
	err := action.Create(newbusyBoxPod(set))
	if err != nil && !errors.IsAlreadyExists(err) {
		logrus.Errorf("Failed to create busybox pod : %v", err)
		return err
	}
	return nil
}

func (h *Handler) listPods(node *v1.Node) (error, []v1.Pod) {
	pods := &v1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}

	sel := fmt.Sprintf("spec.nodeName=%s", node.Name)
	opt := &metav1.ListOptions{FieldSelector: sel}
	err := query.List("--all-namespaces", pods, query.WithListOptions(opt))
	if err != nil {
		logrus.Errorf("failed to get pod list: %v", err)
	}
	return err, pods.Items
}

func (h *Handler) dirtyVolumes(pod v1.Pod) []v1.Volume {
	volumes := []v1.Volume{}
	for _, vol := range pod.Spec.Volumes {
		if vol.VolumeSource.PersistentVolumeClaim != nil {
			volumes = append(volumes, vol)
		}
	}
	return volumes
}

func (h *Handler) isPodDirty(pod v1.Pod) bool {
	dirty := false
	volumes := h.dirtyVolumes(pod)

	if len(volumes) > 0 {
		dirty = true
		logrus.Infof("Pod: %s/%s on %s", pod.Namespace, pod.Name, pod.Spec.NodeName)
		for _, vol := range volumes {
			claim := vol.VolumeSource.PersistentVolumeClaim.ClaimName
			logrus.Infof("\tpvc: %v", claim)


			pvc := &v1.PersistentVolumeClaim{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PersistentVolumeClaim",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      vol.VolumeSource.PersistentVolumeClaim.ClaimName,
				},
			}

			err := query.Get(pvc)
			if err != nil {
				logrus.Errorf("failed to get persistent volume claim: %v", err)
			} else {
				// if len(pvc.Spec.VolumeName) != 0 {
				logrus.Infof("\tpv: %v", pvc.Spec.VolumeName)
				// pv, err := c.client.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
			}
		}
	}
	return dirty
}


func (h *Handler) isNodeDirty(node *v1.Node, condition v1.NodeCondition) bool {
	dirty := false
	if v1.NodeReady == condition.Type && v1.ConditionUnknown == condition.Status {
		// https://kubernetes.io/docs/concepts/architecture/nodes/#condition
		_, pods := h.listPods(node)
		for _, pod := range pods {
			if h.isPodDirty(pod) {
				dirty = true
			}
		}
	}
	if dirty {
		logrus.Warningf("Node %s is lost with attached persistent volumes", node.Name)
	} else {
		logrus.Warningf("Node %s is lost", node.Name)
	}
	return dirty
}

func newFencingRequest(node *v1.Node) *v1alpha1.FencingRequest {
	labels := map[string]string{
		"app": "busy-box",
	}
	// volumes := h.dirtyVolumes(pod) // Do anything with these perhaps?  Disk fencing?
	return &v1alpha1.FencingRequest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "FencingRequest",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "fence-all-the-things",
			Namespace:    "default",
			Labels: labels,
		},
		Spec: v1alpha1.FencingRequestSpec{},
	}
}

// newbusyBoxPod demonstrates how to create a busybox pod
func newbusyBoxPod(cr *v1alpha1.FencingSet) *v1.Pod {
	labels := map[string]string{
		"app": "busy-box",
	}
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "busy-box",
			Namespace:    "default",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cr, schema.GroupVersionKind{
					Group:   v1alpha1.SchemeGroupVersion.Group,
					Version: v1alpha1.SchemeGroupVersion.Version,
					Kind:    "FencingSet",
				}),
			},
			Labels: labels,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}
/*
func (h *Handler) createNewNodeFenceObject(node *v1.Node, pv *v1.PersistentVolume) {
	nfName := fmt.Sprintf("node-fence-%s", node.Name)

	var result crdv1.NodeFence
	err := c.crdClient.Get().Resource(crdv1.NodeFenceResourcePlural).Body(nfName).Do().Into(&result)
	// If no error means the resource already exists
	if err == nil {
		glog.Infof("nodefence CRD for node %s already exists", node.Name)
		return
	}

	nodeFencing := &crdv1.NodeFence{
		Metadata: metav1.ObjectMeta{
			Name: nfName,
		},
		Retries:  0,
		Step:     crdv1.NodeFenceStepIsolation,
		NodeName: node.Name,
		Status:   crdv1.NodeFenceConditionNew,
	}

	backoff := wait.Backoff{
		Duration: crdPostInitialDelay,
		Factor:   crdPostFactor,
		Steps:    crdPostSteps,
	}

	err = wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := c.crdClient.Post().
			Resource(crdv1.NodeFenceResourcePlural).
			Body(nodeFencing).
			Do().Into(&result)
		if err != nil {
			// Re-Try it as errors writing to the API server are common
			return false, err
		}
		return true, nil
	})
	if err != nil {
		glog.Warningf("failed to post NodeFence CRD object: %v", err)
	} else {
		glog.Infof("Posted NodeFence CRD object for node %s", node.Name)
	}
}
*/
