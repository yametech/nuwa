/*
Copyright 2019 yametech.

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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"gomodules.xyz/jsonpatch/v2"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//const (
//	annotationPrefix = "sidecar.admission.nuwav1.io"
//)

var (
	schemePod = runtime.NewScheme()
)

// +kubebuilder:object:root=false
// +k8s:deepcopy-gen=false
type Pod struct {
	KubeClient client.Client
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func filterInjectorPod(list []Injector, pod *corev1.Pod) ([]*Injector, error) {
	var matchingIPs []*Injector

	for _, sp := range list {
		selector, err := metav1.LabelSelectorAsSelector(&sp.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("label selector conversion failed: %v for selector: %v", sp.Spec.Selector, err)
		}

		// check if the pod labels match the selector
		if !selector.Matches(labels.Set(pod.Labels)) {
			glog.V(6).Infof("InjectorPod '%s' does NOT match pod '%s' labels", sp.GetName(), pod.GetName())
			continue
		}
		glog.V(4).Infof("InjectorPod '%s' matches pod '%s' labels", sp.GetName(), pod.GetName())
		// create pointer to a non-loop variable
		newIP := sp
		matchingIPs = append(matchingIPs, &newIP)
	}
	return matchingIPs, nil
}

//TODO: Only support Create Event,Not Support Update Event.Next version will Support it
func (p *Pod) mutatePods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	glog.V(2).Info("Entering mutatePods in mutating webhook")
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if ar.Request.Resource != podResource {
		glog.Errorf("expect resource to be %s", podResource)
		return nil
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	deserializer := serializer.NewCodecFactory(
		runtime.NewScheme(),
	).UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}
	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true
	podCopy := pod.DeepCopy()
	glog.V(6).Infof("Examining pod: %v\n", pod.GetName())

	// Ignore if exclusion annotation is present
	if podAnnotations := pod.GetAnnotations(); podAnnotations != nil {
		glog.V(5).Infof("Looking at pod annotations, found: %v", podAnnotations)
		//TODO: when Update Event,should be check it
		//if podAnnotations[fmt.Sprintf("%s/exclude", annotationPrefix)] == "true" {
		//	return &reviewResponse
		//}
		if _, isMirrorPod := podAnnotations[corev1.MirrorPodAnnotationKey]; isMirrorPod {
			return &reviewResponse
		}
	}

	list := &InjectorList{}
	err := p.KubeClient.List(context.TODO(), list, &client.ListOptions{Namespace: pod.Namespace})
	if meta.IsNoMatchError(err) {
		glog.Errorf("%v (has the CRD been loaded?)", err)
		return toAdmissionResponse(err)
	} else if err != nil {
		glog.Errorf("error fetching injector: %v", err)
		return toAdmissionResponse(err)
	}

	glog.Infof("fetched %d injector(s) in namespace %s", len(list.Items), pod.Namespace)
	if len(list.Items) == 0 {
		glog.V(5).Infof("No pod injector created, so skipping pod %v", pod.Name)
		return &reviewResponse
	}

	matchingIPods, err := filterInjectorPod(list.Items, &pod)
	if err != nil {
		glog.Errorf("filtering pod injector  failed: %v", err)
		return toAdmissionResponse(err)
	}

	if len(matchingIPods) == 0 {
		glog.V(5).Infof("No matching pod injector, so skipping pod %v", pod.Name)
		return &reviewResponse
	}

	sidecarNames := make([]string, len(matchingIPods))
	for i, sp := range matchingIPods {
		sidecarNames[i] = sp.GetName()
	}

	glog.V(5).Infof("Matching Injector Pod detected of count %v, patching spec", len(matchingIPods))
	if matchingIPods[0].Spec.PreContainers != nil && matchingIPods[0].Spec.AfterContainers == nil {
		var pods []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.PreContainers {
				//if the pod name same, not update it.
				if podCopyContainer.Name == matchingContainer.Name {
					continue
				}
				pods = append(pods, matchingContainer)
			}
		}
		//podCopy.Spec.Containers = make([]corev1.Container, len(pods))
		podCopy.Spec.Containers = append(pods, podCopy.Spec.Containers...)
	}
	if matchingIPods[0].Spec.AfterContainers != nil && matchingIPods[0].Spec.PreContainers == nil {
		var pods []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.AfterContainers {
				//if the pod name same, not update it.
				if podCopyContainer.Name == matchingContainer.Name {
					continue
				}
				pods = append(pods, matchingContainer)
			}
		}
		podCopy.Spec.Containers = make([]corev1.Container, len(pods))
		podCopy.Spec.Containers = append(podCopy.Spec.Containers, pods...)
	}

	if matchingIPods[0].Spec.AfterContainers != nil && matchingIPods[0].Spec.PreContainers != nil {
		var podPres []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.PreContainers {
				//if the pod name same, not update it.
				if podCopyContainer.Name == matchingContainer.Name {
					continue
				}
				podPres = append(podPres, matchingContainer)
			}
		}
		podCopy.Spec.Containers = make([]corev1.Container, len(podPres))
		podCopy.Spec.Containers = append(podPres, podCopy.Spec.Containers...)
		var podAfters []corev1.Container
		for _, podCopyContainer := range podCopy.Spec.Containers {
			for _, matchingContainer := range matchingIPods[0].Spec.AfterContainers {
				//if the pod name same, not update it.
				if podCopyContainer.Name == matchingContainer.Name {
					continue
				}
				podAfters = append(podAfters, matchingContainer)
			}
		}
		podCopy.Spec.Containers = append(podCopy.Spec.Containers, podAfters...)
	}
	var volumes []corev1.Volume
	if matchingIPods[0].Spec.Volumes != nil {
		if podCopy.Spec.Volumes != nil {
			for _, podCopyv := range podCopy.Spec.Volumes {
				for _, matchingIPodv := range matchingIPods[0].Spec.Volumes {
					//if the volume name same, not update it.
					if podCopyv.Name == matchingIPodv.Name {
						continue
					}
					volumes = append(volumes, matchingIPodv)
				}
			}
		} else {
			volumes = append(podCopy.Spec.Volumes, matchingIPods[0].Spec.Volumes...)
		}

	}
	podCopy.Spec.Volumes = volumes

	// TODO: investigate why GetGenerateName doesn't work
	glog.Infof("applied injector: %s successfully on Pod: %+v ", strings.Join(sidecarNames, ","), pod.GetName())

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

	return &reviewResponse
}

type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
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
		glog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	deserializer := serializer.NewCodecFactory(
		runtime.NewScheme(),
	).UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Error(err)
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
	// reset the Object and OldObject, they are not needed in a response.
	//ar.Request.Object = runtime.RawExtension{}
	//ar.Request.OldObject = runtime.RawExtension{}

	resp, err := json.Marshal(response)
	if err != nil {
		glog.Error(err)
	}
	if _, err := w.Write(resp); err != nil {
		glog.Error(err)
	}
}

func (p *Pod) ServeMutatePods(w http.ResponseWriter, r *http.Request) {
	serve(w, r, p.mutatePods)
}

func init() {
	addToSchemea(schemePod)
}

func addToSchemea(scheme *runtime.Scheme) {
	_ = extapi.AddToScheme(scheme)
	_ = kscheme.AddToScheme(scheme)
	_ = AddToScheme(scheme)
}
