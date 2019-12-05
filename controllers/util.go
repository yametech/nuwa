package controllers

import (
	"fmt"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"
)

func ownerReference(obj metav1.Object, kindName string) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(obj,
			schema.GroupVersionKind{
				Group:   nuwav1.GroupVersion.Group,
				Version: nuwav1.GroupVersion.Version,
				Kind:    kindName,
			}),
	}
}

func deploymentName(coordinateName string, instance *nuwav1.Water) string {
	return fmt.Sprintf("%s-%s", instance.Name, strings.ToLower(coordinateName))
}

func coordinateName(c *nuwav1.Coordinate) (string, error) {
	res := ""
	if c.Room == "" {
		return "", ErrNeedAtLeastRoom
	}
	res += c.Room
	res += "-"
	if c.Cabinet != "" {
		res += c.Cabinet
	} else {
		res += "Non"
	}
	return res, nil
}
