package v1alpha1

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/operator-framework/operator-sdk/pkg/sdk/action"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

)


func (fr *FencingRequest) AddResult(method string, result FencingResult, err error) {
	fr.Status.Updates = append(fr.Status.Updates, FencingRequestStatusUpdate {
		Timestamp: time.Now().String(),
		Method: method,
		Error: fmt.Sprintf("%v", err),
		Message: fmt.Sprintf("%v", result),
	})
	fr.Update("AddResult")
}

func (fr *FencingRequest) SetFinalResult(result FencingResult, err error) {
	fr.Status.Complete = true
	fr.Status.Result = fmt.Sprintf("%v", result)
	fr.AddResult("", result, err)
}

func (fr *FencingRequest) Update(prefix string) error {
	// Do we need to modify a copy so we can test for changes before doing an update?
	// Eg.
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


func (fr *FencingRequest)JobLabels(method string) map[string]string {
	labels := map[string]string{
		"app": "fencing-operator",
		"target": fr.Spec.Target,
		"request": fr.Name,
	}
	if fr.Status.Config != nil {
		labels["config"] = *fr.Status.Config
	}
	if len(method) > 0 {
		labels["method"] = method
	}
	return labels
}
