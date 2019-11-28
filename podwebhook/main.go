package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"gomodules.xyz/jsonpatch/v2"
	"k8s.io/apimachinery/pkg/api/meta"

	v1 "github.com/yametech/nuwa/api/v1"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"k8s.io/apimachinery/pkg/runtime/serializer"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

const (
	annotationPrefix = "podpreset.admission.kubernetes.io"
)

var (
	scheme = runtime.NewScheme()
)

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile string
	KeyFile  string
}

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "tls-cert-file", c.CertFile, ""+
		"File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated "+
		"after server cert).")
	flag.StringVar(&c.KeyFile, "tls-private-key-file", c.KeyFile, ""+
		"File containing the default x509 private key matching --tls-cert-file.")
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func filterPodPresets(list []v1.Sidecar, pod *corev1.Pod) ([]*v1.Sidecar, error) {
	var matchingPPs []*v1.Sidecar

	for _, pp := range list {
		selector, err := metav1.LabelSelectorAsSelector(&pp.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("label selector conversion failed: %v for selector: %v", pp.Spec.Selector, err)
		}

		// check if the pod labels match the selector
		if !selector.Matches(labels.Set(pod.Labels)) {
			glog.V(6).Infof("PodPreset '%s' does NOT match pod '%s' labels", pp.GetName(), pod.GetName())
			continue
		}
		glog.V(4).Infof("PodPreset '%s' matches pod '%s' labels", pp.GetName(), pod.GetName())
		// create pointer to a non-loop variable
		newPP := pp
		matchingPPs = append(matchingPPs, &newPP)
	}
	return matchingPPs, nil
}

// safeToApplyPodPresetsOnPod determines if there is any conflict in information
// injected by given PodPresets in the Pod.
func safeToApplyPodPresetsOnPod(pod *corev1.Pod, podPresets []*v1.Sidecar) error {
	var errs []error

	// volumes attribute is defined at the Pod level, so determine if volumes
	// injection is causing any conflict.
	if _, err := mergeVolumes(pod.Spec.Volumes, podPresets); err != nil {
		errs = append(errs, err)
	}
	for _, ctr := range pod.Spec.Containers {
		if err := safeToApplyPodPresetsOnContainer(&ctr, podPresets); err != nil {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// safeToApplyPodPresetsOnContainer determines if there is any conflict in
// information injected by given PodPresets in the given container.
func safeToApplyPodPresetsOnContainer(ctr *corev1.Container, podPresets []*v1.Sidecar) error {
	var errs []error
	// check if it is safe to merge env vars and volume mounts from given podpresets and
	// container's existing env vars.
	if _, err := mergeEnv(ctr.Env, podPresets); err != nil {
		errs = append(errs, err)
	}
	if _, err := mergeVolumeMounts(ctr.VolumeMounts, podPresets); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

// mergeEnv merges a list of env vars with the env vars injected by given list podPresets.
// It returns an error if it detects any conflict during the merge.
func mergeEnv(envVars []corev1.EnvVar, podPresets []*v1.Sidecar) ([]corev1.EnvVar, error) {
	origEnv := map[string]corev1.EnvVar{}
	for _, v := range envVars {
		origEnv[v.Name] = v
	}

	mergedEnv := make([]corev1.EnvVar, len(envVars))
	copy(mergedEnv, envVars)

	var errs []error

	for _, pp := range podPresets {
		for _, v := range pp.Spec.Env {
			found, ok := origEnv[v.Name]
			if !ok {
				// if we don't already have it append it and continue
				origEnv[v.Name] = v
				mergedEnv = append(mergedEnv, v)
				continue
			}

			// make sure they are identical or throw an error
			if !reflect.DeepEqual(found, v) {
				errs = append(errs, fmt.Errorf("merging env for %s has a conflict on %s: \n%#v\ndoes not match\n%#v\n in container", pp.GetName(), v.Name, v, found))
			}
		}
	}

	err := utilerrors.NewAggregate(errs)
	if err != nil {
		return nil, err
	}

	return mergedEnv, err
}

func mergeEnvFrom(envSources []corev1.EnvFromSource, podPresets []*v1.Sidecar) ([]corev1.EnvFromSource, error) {
	var mergedEnvFrom []corev1.EnvFromSource

	mergedEnvFrom = append(mergedEnvFrom, envSources...)
	for _, pp := range podPresets {
		mergedEnvFrom = append(mergedEnvFrom, pp.Spec.EnvFrom...)
	}

	return mergedEnvFrom, nil
}

// mergeVolumeMounts merges given list of VolumeMounts with the volumeMounts
// injected by given podPresets. It returns an error if it detects any conflict during the merge.
func mergeVolumeMounts(volumeMounts []corev1.VolumeMount, podPresets []*v1.Sidecar) ([]corev1.VolumeMount, error) {

	origVolumeMounts := map[string]corev1.VolumeMount{}
	volumeMountsByPath := map[string]corev1.VolumeMount{}
	for _, v := range volumeMounts {
		origVolumeMounts[v.Name] = v
		volumeMountsByPath[v.MountPath] = v
	}

	mergedVolumeMounts := make([]corev1.VolumeMount, len(volumeMounts))
	copy(mergedVolumeMounts, volumeMounts)

	var errs []error

	for _, pp := range podPresets {
		for _, v := range pp.Spec.VolumeMounts {
			found, ok := origVolumeMounts[v.Name]
			if !ok {
				// if we don't already have it append it and continue
				origVolumeMounts[v.Name] = v
				mergedVolumeMounts = append(mergedVolumeMounts, v)
			} else {
				// make sure they are identical or throw an error
				// shall we throw an error for identical volumeMounts ?
				if !reflect.DeepEqual(found, v) {
					errs = append(errs, fmt.Errorf("merging volume mounts for %s has a conflict on %s: \n%#v\ndoes not match\n%#v\n in container", pp.GetName(), v.Name, v, found))
				}
			}

			found, ok = volumeMountsByPath[v.MountPath]
			if !ok {
				// if we don't already have it append it and continue
				volumeMountsByPath[v.MountPath] = v
			} else {
				// make sure they are identical or throw an error
				if !reflect.DeepEqual(found, v) {
					errs = append(errs, fmt.Errorf("merging volume mounts for %s has a conflict on mount path %s: \n%#v\ndoes not match\n%#v\n in container", pp.GetName(), v.MountPath, v, found))
				}
			}
		}
	}

	err := utilerrors.NewAggregate(errs)
	if err != nil {
		return nil, err
	}

	return mergedVolumeMounts, err
}

// mergeVolumes merges given list of Volumes with the volumes injected by given
// podPresets. It returns an error if it detects any conflict during the merge.
func mergeVolumes(volumes []corev1.Volume, podPresets []*v1.Sidecar) ([]corev1.Volume, error) {
	origVolumes := map[string]corev1.Volume{}
	for _, v := range volumes {
		origVolumes[v.Name] = v
	}

	mergedVolumes := make([]corev1.Volume, len(volumes))
	copy(mergedVolumes, volumes)

	var errs []error

	for _, pp := range podPresets {
		for _, v := range pp.Spec.Volumes {
			found, ok := origVolumes[v.Name]
			if !ok {
				// if we don't already have it append it and continue
				origVolumes[v.Name] = v
				mergedVolumes = append(mergedVolumes, v)
				continue
			}

			// make sure they are identical or throw an error
			if !reflect.DeepEqual(found, v) {
				errs = append(errs, fmt.Errorf("merging volumes for %s has a conflict on %s: \n%#v\ndoes not match\n%#v\n in container", pp.GetName(), v.Name, v, found))
			}
		}
	}

	err := utilerrors.NewAggregate(errs)
	if err != nil {
		return nil, err
	}

	if len(mergedVolumes) == 0 {
		return nil, nil
	}

	return mergedVolumes, err
}

// func recordConflictEvent(recorder record.EventRecorder, pod *corev1.Pod, message string) {
// 	// Event API doesn't support corv1.Pod object for strange reason,
// 	podRef := &corev1.ObjectReference{
// 		Kind:      "Pod",
// 		Name:      pod.GetName(),
// 		Namespace: pod.GetNamespace(),
// 	}
// 	recorder.Event(podRef, corev1.EventTypeWarning, "PodPreset", message)
// 	ref := metav1.GetControllerOf(pod)
// 	if ref != nil {
// 		// raise the event at the immediate parent controller as well
// 		ctrl := &corev1.ObjectReference{
// 			Kind:       ref.Kind,
// 			Name:       ref.Name,
// 			Namespace:  pod.GetNamespace(),
// 			UID:        ref.UID,
// 			APIVersion: ref.APIVersion,
// 		}
// 		recorder.Eventf(ctrl, corev1.EventTypeWarning, "PodPreset", message)
// 	}
// }

// applyPodPresetsOnPod updates the PodSpec with merged information from all the
// applicable PodPresets. It ignores the errors of merge functions because merge
// errors have already been checked in safeToApplyPodPresetsOnPod function.
func applyPodPresetsOnPod(pod *corev1.Pod, podPresets []*v1.Sidecar) {
	if len(podPresets) == 0 {
		return
	}

	volumes, _ := mergeVolumes(pod.Spec.Volumes, podPresets)
	pod.Spec.Volumes = volumes

	for i, ctr := range pod.Spec.Containers {
		applyPodPresetsOnContainer(&ctr, podPresets)
		pod.Spec.Containers[i] = ctr
	}

	if pod.ObjectMeta.Annotations == nil {
		pod.ObjectMeta.Annotations = map[string]string{}
	}

	// add annotation information to mark podpreset mutation has occurred
	for _, pp := range podPresets {
		pod.ObjectMeta.Annotations[fmt.Sprintf("%s/podpreset-%s", annotationPrefix, pp.GetName())] = pp.GetResourceVersion()
	}
}

// applyPodPresetsOnContainer injects envVars, VolumeMounts and envFrom from
// given podPresets in to the given container. It ignores conflict errors
// because it assumes those have been checked already by the caller.
func applyPodPresetsOnContainer(ctr *corev1.Container, podPresets []*v1.Sidecar) {
	envVars, _ := mergeEnv(ctr.Env, podPresets)
	ctr.Env = envVars

	volumeMounts, _ := mergeVolumeMounts(ctr.VolumeMounts, podPresets)
	ctr.VolumeMounts = volumeMounts

	envFrom, _ := mergeEnvFrom(ctr.EnvFrom, podPresets)
	ctr.EnvFrom = envFrom
}

func mutatePods(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
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
		if podAnnotations[fmt.Sprintf("%s/exclude", annotationPrefix)] == "true" {
			return &reviewResponse
		}
		if _, isMirrorPod := podAnnotations[corev1.MirrorPodAnnotationKey]; isMirrorPod {
			return &reviewResponse
		}
	}

	crdclient := getCrdClient()
	list := &v1.SidecarList{}
	err := crdclient.List(context.TODO(), list, &client.ListOptions{Namespace: pod.Namespace})
	if meta.IsNoMatchError(err) {
		glog.Errorf("%v (has the CRD been loaded?)", err)
		return toAdmissionResponse(err)
	} else if err != nil {
		glog.Errorf("error fetching podpresets: %v", err)
		return toAdmissionResponse(err)
	}

	glog.Infof("fetched %d podpreset(s) in namespace %s", len(list.Items), pod.Namespace)
	if len(list.Items) == 0 {
		glog.V(5).Infof("No pod presets created, so skipping pod %v", pod.Name)
		return &reviewResponse
	}

	matchingPPs, err := filterPodPresets(list.Items, &pod)
	if err != nil {
		glog.Errorf("filtering pod presets failed: %v", err)
		return toAdmissionResponse(err)
	}

	if len(matchingPPs) == 0 {
		glog.V(5).Infof("No matching pod presets, so skipping pod %v", pod.Name)
		return &reviewResponse
	}

	presetNames := make([]string, len(matchingPPs))
	for i, pp := range matchingPPs {
		presetNames[i] = pp.GetName()
	}

	glog.V(5).Infof("Matching PP detected of count %v, patching spec", len(matchingPPs))

	// detect merge conflict
	err = safeToApplyPodPresetsOnPod(&pod, matchingPPs)
	if err != nil {
		// conflict, ignore the error, but raise an event
		// TODO: investigate why GetGenerateName doesn't work, might be because it's too early to exist yet
		msg := fmt.Errorf("conflict occurred while applying podpresets: %s on pod: %v err: %v",
			strings.Join(presetNames, ","), pod.GetName(), err)
		//recordConflictEvent(recorder, &pod, msg)
		glog.Warning(msg)
		return toAdmissionResponse(msg)
	}

	applyPodPresetsOnPod(&pod, matchingPPs)

	// TODO: investigate why GetGenerateName doesn't work
	glog.Infof("applied podpresets: %s successfully on Pod: %+v ", strings.Join(presetNames, ","), pod.GetName())

	podCopyJSON, err := json.Marshal(podCopy)
	if err != nil {
		return toAdmissionResponse(err)
	}
	podJSON, err := json.Marshal(pod)
	if err != nil {
		return toAdmissionResponse(err)
	}
	jsonPatch, err := jsonpatch.CreatePatch(podCopyJSON, podJSON)
	if err != nil {
		return toAdmissionResponse(err)
	}
	jsonPatchBytes, _ := json.Marshal(jsonPatch)

	reviewResponse.Patch = jsonPatchBytes
	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt

	// if !reviewResponse.Allowed {
	// 	reviewResponse.Result = &metav1.Status{Message: strings.TrimSpace(msg)}
	// }
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

func serveMutatePods(w http.ResponseWriter, r *http.Request) {
	serve(w, r, mutatePods)
}

func getCrdClient() client.Client {
	config, err := config.GetConfig()
	if err != nil {
		glog.Fatal(err)
	}
	crdclient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		glog.Fatal(err)
	}

	return crdclient
}

func init() {
	addToScheme(scheme)
}

func addToScheme(scheme *runtime.Scheme) {
	corev1.AddToScheme(scheme)
	admissionregistrationv1beta1.AddToScheme(scheme)
	// defaulting with webhooks:
	// https://github.com/kubernetes/kubernetes/issues/57982
	_ = v1.AddToScheme(scheme)
	//apis.AddToScheme(scheme)
}

func configTLS(config Config) *tls.Config {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		glog.Fatalf("config=%#v Error: %v", config, err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
		// TODO: uses mutual tls after we agree on what cert the apiserver should use.
		//ClientAuth: tls.RequireAndVerifyClientCert,
	}
}

func main() {
	var config Config
	config.CertFile = "C:\\Users\\yamei\\Desktop\\OpenSource\\nuwa\\ssl\\tls.crt"
	config.KeyFile = "C:\\Users\\yamei\\Desktop\\OpenSource\\nuwa\\ssl\\tls.key"
	http.HandleFunc("/mutating-pods", serveMutatePods)
	server := &http.Server{
		Addr: ":443",
	}
	glog.Infof("About to start serving webhooks: %#v", server)
	server.ListenAndServeTLS(config.CertFile, config.KeyFile)
}
