package controllers

import (
	"context"
	"fmt"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatusUpdaterInterface is an interface used to update the StatefulSetStatus associated with a StatefulSet.
// For any use other than testing, clients should create an instance using NewRealStatefulSetStatusUpdater.
type StatusUpdaterInterface interface {
	// UpdateStatefulSetStatus sets the set's Status to status. Implementations are required to retry on conflicts,
	// but fail on other errors. If the returned error is nil set's Status has been successfully set to status.
	UpdateStatefulSetStatus(set *nuwav1.StatefulSet, status *nuwav1.StatefulSetStatus) error
}

// NewRealStatefulSetStatusUpdater returns a StatusUpdaterInterface that updates the Status of a StatefulSet,
// using the supplied client and setLister.
func NewRealStatefulSetStatusUpdater(client client.Client) StatusUpdaterInterface {
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
		ctx := context.Background()
		updateErr := ssu.Client.Status().Update(ctx, set)
		if updateErr != nil {
			return updateErr
		}
		updated := &nuwav1.StatefulSet{}
		objKey := client.ObjectKey{Namespace: set.Namespace, Name: set.Name}
		if err := ssu.Client.Get(ctx, objKey, updated); err == nil {
			// make a copy so we don't mutate the shared cache
			set = updated.DeepCopy()
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated StatefulSet %s/%s from lister: %v", set.Namespace, set.Name, err))
		}

		return updateErr
	})
}

var _ StatusUpdaterInterface = &realStatefulSetStatusUpdater{}
