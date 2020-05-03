/*
Copyright 2019 yametech Authors.

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

package v1

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	"gomodules.xyz/jsonpatch/v2"
	"io/ioutil"
	"k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var schemePod = runtime.NewScheme()

// +kubebuilder:object:root=false
// +k8s:deepcopy-gen=false
type WebhookServer struct {
	Client client.Client
	Log    logr.Logger
}

var ingoredList []string = []string{
	"kube-system",
	"default",
	"kube-public",
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func ignoredRequired(namespace string) bool {
	// skip special kubernetes system namespaces
	for _, ignoreNamespace := range ingoredList {
		if namespace == ignoreNamespace {
			return false
		}
	}
	return true
}

var (
	podGvr = metav1.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}

	deserializer = serializer.NewCodecFactory(
		runtime.NewScheme(),
	).UniversalDeserializer()
)

type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

//type validateFunc func(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

// TODO: Only support Create Event,Not Support Update Event.Next version will Support it
func (p *WebhookServer) serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	defer r.Body.Close()

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		p.Log.Error(
			fmt.Errorf(
				"context type is non expect error, value: %v", contentType),
			"",
		)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}

	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		reviewResponse = toAdmissionResponse(err)
	} else {
		reviewResponse = admit(ar)
	}

	response := v1beta1.AdmissionReview{}
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = ar.Request.UID
	}

	// must add api version and kind on kube16.2
	response.APIVersion = "admission.k8s.io/v1"
	response.Kind = "AdmissionReview"

	resp, err := json.Marshal(response)
	if err != nil {
		p.Log.Error(err, "")
	}

	if _, err := w.Write(resp); err != nil {
		p.Log.Error(err, "")
	}
}

func (p *WebhookServer) ServeInjectorMutatePods(w http.ResponseWriter, r *http.Request) {
	p.serve(w, r, p.injectorMutatePods)
}

func (p *WebhookServer) ServeNamespaceMutateResource(w http.ResponseWriter, r *http.Request) {
	p.serve(w, r, p.namespaceMutateResource)
}

// Enhanced namespace annotation nuwa tagged for resources /Deployment/StatefulSet(k8s)/ReplicaSet/ReplicationController
func (p *WebhookServer) namespaceMutateResource(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	request := ar.Request
	// query namesapce labels nuwa.kubernetes.io

	mutateNamespaceName := ar.Request.Namespace
	var srcObject runtime.Object
	var destObject runtime.Object
	switch request.Kind.Kind {
	case "Deployment":
		deployment := &appsv1.Deployment{}
		if _, _, err := deserializer.Decode(ar.Request.Object.Raw, nil, deployment); err != nil {
			return toAdmissionResponse(err)
		}
		if len(deployment.OwnerReferences) > 0 {
			return ar.Response
		}
		srcObject = deployment.DeepCopyObject()
		if err := p.updateNamespaceForResourceLimitRange(mutateNamespaceName, deployment); err != nil {
			return toAdmissionResponse(err)
		}
		destObject = deployment
	case "StatefulSet":
		statefulset := &appsv1.StatefulSet{}
		if _, _, err := deserializer.Decode(ar.Request.Object.Raw, nil, statefulset); err != nil {
			return toAdmissionResponse(err)
		}
		if len(statefulset.OwnerReferences) > 0 {
			return ar.Response
		}
		srcObject = statefulset.DeepCopyObject()
		if err := p.updateNamespaceForResourceLimitRange(mutateNamespaceName, statefulset); err != nil {
			return toAdmissionResponse(err)
		}
		destObject = statefulset
	case "ReplicationController":
		replicationController := &corev1.ReplicationController{}
		if _, _, err := deserializer.Decode(ar.Request.Object.Raw, nil, replicationController); err != nil {
			return toAdmissionResponse(err)
		}
		if len(replicationController.OwnerReferences) > 0 {
			return ar.Response
		}
		srcObject = replicationController.DeepCopyObject()
		if err := p.updateNamespaceForResourceLimitRange(mutateNamespaceName, replicationController); err != nil {
			return toAdmissionResponse(err)
		}
		destObject = replicationController
	case "ReplicaSet":
		replicaSet := &appsv1.ReplicaSet{}
		if _, _, err := deserializer.Decode(ar.Request.Object.Raw, nil, replicaSet); err != nil {
			return toAdmissionResponse(err)
		}
		if len(replicaSet.OwnerReferences) > 0 {
			return ar.Response
		}
		srcObject = replicaSet.DeepCopyObject()
		if err := p.updateNamespaceForResourceLimitRange(mutateNamespaceName, replicaSet); err != nil {
			return toAdmissionResponse(err)
		}
		destObject = replicaSet
	case "Job":
		job := &batchv1.Job{}
		if _, _, err := deserializer.Decode(ar.Request.Object.Raw, nil, job); err != nil {
			return toAdmissionResponse(err)
		}
		if len(job.OwnerReferences) > 0 {
			return ar.Response
		}
		srcObject = job.DeepCopyObject()
		if err := p.updateNamespaceForResourceLimitRange(mutateNamespaceName, job); err != nil {
			return toAdmissionResponse(err)
		}
		destObject = job
	case "CronJob":
		cronjob := &batchv1beta1.CronJob{}
		if _, _, err := deserializer.Decode(ar.Request.Object.Raw, nil, cronjob); err != nil {
			return toAdmissionResponse(err)
		}
		if len(cronjob.OwnerReferences) > 0 {
			return ar.Response
		}
		srcObject = cronjob.DeepCopyObject()
		if err := p.updateNamespaceForResourceLimitRange(mutateNamespaceName, cronjob); err != nil {
			return toAdmissionResponse(err)
		}
		destObject = cronjob
	default:
		return ar.Response
	}

	// Construct an effective response
	reviewResponse := &v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	srcObjectJSON, err := json.Marshal(srcObject)
	if err != nil {
		return toAdmissionResponse(err)
	}

	destObjectJSON, err := json.Marshal(destObject)
	if err != nil {
		return toAdmissionResponse(err)
	}

	jsonPatch, err := jsonpatch.CreatePatch(srcObjectJSON, destObjectJSON)
	if err != nil {
		return toAdmissionResponse(err)
	}

	jsonPatchBytes, _ := json.Marshal(jsonPatch)

	reviewResponse.Patch = jsonPatchBytes
	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt

	return reviewResponse
}

func (p *WebhookServer) updateNamespaceForResourceLimitRange(namespaceName string, obj runtime.Object) error {
	namespace := &corev1.Namespace{}
	ctx := context.Background()
	err := p.Client.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace)
	if err != nil {
		return err
	}

	annotateMap := namespace.GetAnnotations()
	limitContent, exist := annotateMap[NuwaLimitFlag]
	if !exist {
		return nil
	}
	var coordinates = make(Coordinates, 0)
	if err := json.Unmarshal([]byte(limitContent), &coordinates); err != nil {
		p.Log.Info(
			"deserialization nuwa enhancement failure",
			"content", limitContent,
			"namespace", namespaceName,
		)
		return err
	}

	switch obj.(type) {
	case *appsv1.Deployment:
		realObj := obj.(*appsv1.Deployment)
		if realObj.Spec.Template.Spec.Affinity == nil {
			realObj.Spec.Template.Spec.Affinity = &corev1.Affinity{
				NodeAffinity: p.convertToNodeSelectorAffinity(coordinates),
			}
		}
	case *appsv1.StatefulSet:
		realObj := obj.(*appsv1.StatefulSet)
		if realObj.Spec.Template.Spec.Affinity == nil {
			realObj.Spec.Template.Spec.Affinity = &corev1.Affinity{
				NodeAffinity: p.convertToNodeSelectorAffinity(coordinates),
			}
		}
	case *corev1.ReplicationController:
		realObj := obj.(*corev1.ReplicationController)
		if realObj.Spec.Template.Spec.Affinity == nil {
			realObj.Spec.Template.Spec.Affinity = &corev1.Affinity{
				NodeAffinity: p.convertToNodeSelectorAffinity(coordinates),
			}
		}
	case *appsv1.ReplicaSet:
		realObj := obj.(*appsv1.ReplicaSet)
		if realObj.Spec.Template.Spec.Affinity == nil {
			realObj.Spec.Template.Spec.Affinity = &corev1.Affinity{
				NodeAffinity: p.convertToNodeSelectorAffinity(coordinates),
			}
		}

	case *batchv1.Job:
		realObj := obj.(*batchv1.Job)
		if realObj.Spec.Template.Spec.Affinity == nil {
			realObj.Spec.Template.Spec.Affinity = &corev1.Affinity{
				NodeAffinity: p.convertToNodeSelectorAffinity(coordinates),
			}
		}

	case *batchv1beta1.CronJob:
		realObj := obj.(*batchv1beta1.CronJob)
		if realObj.Spec.JobTemplate.Spec.Template.Spec.Affinity == nil {
			realObj.Spec.JobTemplate.Spec.Template.Spec.Affinity = &corev1.Affinity{
				NodeAffinity: p.convertToNodeSelectorAffinity(coordinates),
			}
		}
	}

	return nil
}

func removeRepeatedElement(arr []string) (newArr []string) {
	newArr = make([]string, 0)
	for i := 0; i < len(arr); i++ {
		repeat := false
		for j := i + 1; j < len(arr); j++ {
			if arr[i] == arr[j] {
				repeat = true
				break
			}
		}
		if !repeat {
			newArr = append(newArr, arr[i])
		}
	}
	return
}

func (p *WebhookServer) convertToNodeSelectorAffinity(coordinates Coordinates) *corev1.NodeAffinity {
	var zones []string
	var racks []string
	var hosts []string
	for _, item := range coordinates {
		zones = append(zones, item.Zone)
		racks = append(racks, item.Rack)
		hosts = append(hosts, item.Host)
	}
	// return the NodeAffinity
	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      NuwaZoneFlag,
							Operator: corev1.NodeSelectorOpIn,
							Values:   removeRepeatedElement(zones),
						},
						{
							Key:      NuwaRackFlag,
							Operator: corev1.NodeSelectorOpIn,
							Values:   racks,
						},
					},
				},
			},
		},
		PreferredDuringSchedulingIgnoredDuringExecution:
		[]corev1.PreferredSchedulingTerm{
			{
				Weight: 100,
				Preference: corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      NuwaHostFlag,
							Operator: corev1.NodeSelectorOpIn,
							Values:   hosts,
						},
					},
				},
			},
		},
	}
}

// Enhanced injector for waters/stones resources
func (p *WebhookServer) injectorMutatePods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	reviewResponse := &v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	if ar.Request.Resource != podGvr {
		err := fmt.Errorf("expect resource to be %s", podGvr)
		p.Log.Error(err, "")
		return toAdmissionResponse(err)
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}

	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		p.Log.Error(err, "")
		return toAdmissionResponse(err)
	}

	if !ignoredRequired(ar.Request.Namespace) {
		return reviewResponse
	}

	podCopy := pod.DeepCopy()

	// Ignore if exclusion annotation is present
	if podAnnotations := pod.GetAnnotations(); podAnnotations != nil {
		if _, isMirrorPod := podAnnotations[corev1.MirrorPodAnnotationKey]; isMirrorPod {
			return reviewResponse
		}
	}

	list := &InjectorList{}
	err := p.Client.List(context.TODO(), list, &client.ListOptions{Namespace: pod.Namespace})
	if meta.IsNoMatchError(err) {
		p.Log.Error(fmt.Errorf("%v (has the CRD been loaded?)", err), "")
		return toAdmissionResponse(err)
	}

	if err != nil {
		p.Log.Error(err, "error fetching injector")
		return toAdmissionResponse(err)
	}

	if len(list.Items) == 0 {
		return reviewResponse
	}

	matchingIPods, err := filterInjectorPod(list.Items, &pod)
	if err != nil {
		return toAdmissionResponse(err)
	}

	if len(matchingIPods) == 0 {
		return reviewResponse
	}

	sidecarNames := make([]string, len(matchingIPods))
	for i, sp := range matchingIPods {
		sidecarNames[i] = sp.GetName()
	}

	if matchingIPods[0].Spec.PreContainers != nil && matchingIPods[0].Spec.PostContainers == nil {
		var pods []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.PreContainers {
				//if the pod name same, add random str
				if podCopyContainer.Name == matchingContainer.Name {
					uid, _ := random()
					matchingContainer.Name = matchingContainer.Name + "-" + uid
				}
				pods = append(pods, matchingContainer)
			}
		}

		podCopy.Spec.Containers = append(pods, podCopy.Spec.Containers...)
	}

	if matchingIPods[0].Spec.PostContainers != nil && matchingIPods[0].Spec.PreContainers == nil {
		var pods []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.PostContainers {
				//if the pod name same, add random str
				if podCopyContainer.Name == matchingContainer.Name {
					uid, _ := random()
					matchingContainer.Name = matchingContainer.Name + "-" + uid
				}
				pods = append(pods, matchingContainer)
			}
		}

		podCopy.Spec.Containers = append(podCopy.Spec.Containers, pods...)
	}

	if matchingIPods[0].Spec.PostContainers != nil && matchingIPods[0].Spec.PreContainers != nil {
		var podPres []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.PreContainers {
				//if the pod name same, add random str
				if podCopyContainer.Name == matchingContainer.Name {
					uid, _ := random()
					matchingContainer.Name = matchingContainer.Name + "-" + uid
				}
				podPres = append(podPres, matchingContainer)
			}
		}

		//podCopy.Spec.Containers = append(podPres, podCopy.Spec.Containers...)
		var podAfters []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.PostContainers {
				//if the pod name same, add random str
				if podCopyContainer.Name == matchingContainer.Name {
					uid, _ := random()
					matchingContainer.Name = matchingContainer.Name + "-" + uid
				}
				podAfters = append(podAfters, matchingContainer)
			}
		}

		podCopy.Spec.Containers = append(podPres, podCopy.Spec.Containers...)
		podCopy.Spec.Containers = append(podCopy.Spec.Containers, podAfters...)
	}

	var volumes []corev1.Volume
	if matchingIPods[0].Spec.Volumes != nil {
		if podCopy.Spec.Volumes != nil {
			for _, podCopyValue := range podCopy.Spec.Volumes {
				for _, matchingIPodValue := range matchingIPods[0].Spec.Volumes {
					//if the volume name same, not update it.
					if podCopyValue.Name == matchingIPodValue.Name {
						continue
					}
					volumes = append(volumes, matchingIPodValue)
				}
			}
		} else {
			volumes = append(podCopy.Spec.Volumes, matchingIPods[0].Spec.Volumes...)
		}

	}

	podCopy.Spec.Volumes = volumes

	// TODO: investigate why GetGenerateName doesn't work
	podCopyJSON, err := json.Marshal(podCopy)
	if err != nil {
		return toAdmissionResponse(err)
	}

	podJSON, err := json.Marshal(pod)
	if err != nil {
		return toAdmissionResponse(err)
	}

	jsonPatch, err := jsonpatch.CreatePatch(podJSON, podCopyJSON)
	if err != nil {
		return toAdmissionResponse(err)
	}

	jsonPatchBytes, _ := json.Marshal(jsonPatch)

	reviewResponse.Patch = jsonPatchBytes
	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt

	return reviewResponse
}

func filterInjectorPod(list []Injector, pod *corev1.Pod) ([]*Injector, error) {
	var matchingIPs []*Injector
	for _, sp := range list {
		selector, err := metav1.LabelSelectorAsSelector(&sp.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf(
				"label selector conversion failed: %v for selector: %v",
				sp.Spec.Selector,
				err,
			)
		}
		// check if the pod labels match the selector
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		// create pointer to a non-loop variable
		newIP := sp
		matchingIPs = append(matchingIPs, &newIP)
	}

	return matchingIPs, nil
}

func random() (s string, err error) {
	b := make([]byte, 8)
	_, err = rand.Read(b)
	if err != nil {
		return
	}
	s = fmt.Sprintf("%x", b)

	return
}

func init() {
	_ = apiextensionsapi.AddToScheme(schemePod)
	_ = kscheme.AddToScheme(schemePod)
	_ = AddToScheme(schemePod)
}
