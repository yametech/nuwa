/*
Copyright 2019 yametech Authors.
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
	"bytes"
	"context"
	"fmt"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type realHistory struct {
	client.Client
}

func (rh *realHistory) ListControllerRevisions(parent metav1.Object, selector labels.Selector) ([]*apps.ControllerRevision, error) {
	// List all revisions in the namespace that match the selector
	//history, err := rh.lister.ControllerRevisions(parent.GetNamespace()).List(selector)
	crls := &apps.ControllerRevisionList{}
	err := rh.Client.List(context.TODO(), crls, client.InNamespace(parent.GetNamespace()), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}
	var owned []*apps.ControllerRevision
	for i := range crls.Items {
		ref := metav1.GetControllerOf(&crls.Items[i])
		if ref == nil || ref.UID == parent.GetUID() {
			owned = append(owned, &crls.Items[i])
		}

	}
	return owned, err
}

func (rh *realHistory) CreateControllerRevision(parent metav1.Object, revision *apps.ControllerRevision, collisionCount *int32) (*apps.ControllerRevision, error) {
	if collisionCount == nil {
		return nil, fmt.Errorf("collisionCount should not be nil")
	}
	clone := revision.DeepCopy()
	// Continue to attempt to create the revision updating the name with a new hash on each iteration
	for {
		hash := history.HashControllerRevision(revision, collisionCount)
		// Update the revisions name
		clone.Name = history.ControllerRevisionName(parent.GetName(), hash)
		ns := parent.GetNamespace()
		clone.Namespace = ns
		ctx := context.TODO()
		err := rh.Client.Create(ctx, clone)
		if errors.IsAlreadyExists(err) {
			exists := &apps.ControllerRevision{}
			err := rh.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: clone.Name}, exists)
			if err != nil {
				return nil, err
			}
			if bytes.Equal(exists.Data.Raw, clone.Data.Raw) {
				return exists, nil
			}
			*collisionCount++
			continue
		}
		return clone, err
	}
}

func (rh *realHistory) UpdateControllerRevision(revision *apps.ControllerRevision, newRevision int64) (*apps.ControllerRevision, error) {
	clone := revision.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if clone.Revision == newRevision {
			return nil
		}
		clone.Revision = newRevision
		ctx := context.Background()
		updateErr := rh.Client.Update(ctx, clone)
		if updateErr == nil {
			return nil
		}
		return nil
	})
	return clone, err
}

func (rh *realHistory) DeleteControllerRevision(revision *apps.ControllerRevision) error {
	return rh.Client.Delete(context.TODO(), revision)
}

func (rh *realHistory) AdoptControllerRevision(parent metav1.Object, parentKind schema.GroupVersionKind, revision *apps.ControllerRevision) (*apps.ControllerRevision, error) {
	// Return an error if the parent does not own the revision
	if owner := metav1.GetControllerOf(revision); owner != nil {
		return nil, fmt.Errorf("attempt to adopt revision owned by %v", owner)
	}
	data := []byte(fmt.Sprintf(
		`{"metadata":{"ownerReferences":[{"apiVersion":"%s","kind":"%s","name":"%s","uid":"%s","controller":true,"blockOwnerDeletion":true}],"uid":"%s"}}`,
		parentKind.GroupVersion().String(), parentKind.Kind,
		parent.GetName(), parent.GetUID(), revision.UID))

	ctx := context.TODO()
	if err := rh.Client.Patch(ctx, revision, client.ConstantPatch(types.StrategicMergePatchType, data)); err != nil {
		return nil, err
	}
	newRevision := &apps.ControllerRevision{}
	if err := rh.Client.Get(ctx, types.NamespacedName{Namespace: parent.GetNamespace(), Name: revision.GetName()}, newRevision); err != nil {
		return nil, err
	}
	return newRevision, nil
}

func (rh *realHistory) ReleaseControllerRevision(parent metav1.Object, revision *apps.ControllerRevision) (*apps.ControllerRevision, error) {
	// Use strategic merge patch to add an owner reference indicating a controller ref
	data := []byte(fmt.Sprintf(`{"metadata":{"ownerReferences":[{"$patch":"delete","uid":"%s"}],"uid":"%s"}}`, parent.GetUID(), revision.UID))
	ctx := context.TODO()
	err := rh.Client.Patch(ctx, revision, client.ConstantPatch(types.StrategicMergePatchType, data))
	if err != nil {
		if errors.IsInvalid(err) {
			// We ignore cases where the parent no longer owns the revision or where the revision has no
			// owner.
			return nil, nil
		}
	}
	released := &apps.ControllerRevision{}
	if err := rh.Client.Get(ctx, types.NamespacedName{Namespace: parent.GetNamespace(), Name: revision.GetName()}, released); err != nil {
		if errors.IsNotFound(err) {
			// We ignore deleted revisions
			return nil, nil
		}
		return nil, err
	}

	return released, nil
}
