/*
Copyright 2019 yametech.
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatefulSetStatusUpdaterInterface is an interface used to update the StatefulSetStatus associated with a StatefulSet.
// For any use other than testing, clients should create an instance using NewRealStatefulSetStatusUpdater.
type StatefulSetStatusUpdaterInterface interface {
	// UpdateStatefulSetStatus sets the set's Status to status. Implementations are required to retry on conflicts,
	// but fail on other errors. If the returned error is nil set's Status has been successfully set to status.
	UpdateStatefulSetStatus(set *nuwav1.StatefulSet, status *nuwav1.StatefulSetStatus) error
}

// NewRealStatefulSetStatusUpdater returns a StatefulSetStatusUpdaterInterface that updates the Status of a StatefulSet,
// using the supplied client and setLister.
func NewRealStatefulSetStatusUpdater(
	client client.Client) StatefulSetStatusUpdaterInterface {
	return &realStatefulSetStatusUpdater{client}
}

type realStatefulSetStatusUpdater struct {
	client.Client
}

func (ssu *realStatefulSetStatusUpdater) UpdateStatefulSetStatus(
	set *nuwav1.StatefulSet,
	status *nuwav1.StatefulSetStatus) error {
	// don't wait due to limited number of clients, but backoff after the default number of steps
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		set.Status = *status
		//_, updateErr := ssu.client.AppsV1().StatefulSets(set.Namespace).UpdateStatus(set)
		updateErr := ssu.Client.Update(context.TODO(), set)
		if updateErr == nil {
			return nil
		}
		updated := &nuwav1.StatefulSet{}
		objKey := types.NamespacedName{Namespace: set.Namespace, Name: set.Name}
		if err := ssu.Client.Get(context.TODO(), objKey, updated); err == nil {
			// make a copy so we don't mutate the shared cache
			set = updated.DeepCopy()
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated StatefulSet %s/%s from lister: %v", set.Namespace, set.Name, err))
		}

		return updateErr
	})
}
