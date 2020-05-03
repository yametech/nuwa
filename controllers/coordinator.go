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
package controllers

import "C"
import (
	"fmt"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
)

type CoordinateErr error

var ErrNeedAtLeastZone CoordinateErr = fmt.Errorf("%s", "coordinate need to specify at least zone")

type coordinator struct {
	Index        int
	Name         string               `json:"name,omitempty"`
	Coordinate   *nuwav1.Coordinate   `json:"coordinate,omitempty"`
	NodeAffinity *corev1.NodeAffinity `json:"nodeAffinity,omitempty"`
	NodeNameList []string             `json:"nodeNameList,omitempty"`
}

func newLocalCoordinate(index int, coordinate nuwav1.Coordinate) *coordinator {
	return &coordinator{
		Index:        index,
		Name:         coordinateName(&coordinate),
		NodeAffinity: organizationNodeAffinity(&coordinate),
		Coordinate:   &coordinate,
	}
}

func makeLocalCoordinates(coordinates nuwav1.Coordinates) []*coordinator {
	coordinators := make([]*coordinator, 0)
	for index, item := range coordinates {
		coordinators = append(coordinators, newLocalCoordinate(index, item))
	}
	return coordinators
}

func coordinateName(c *nuwav1.Coordinate) string {
	return fmt.Sprintf("%s-%s", c.Zone, c.Rack)
}

func organizationNodeAffinity(c *nuwav1.Coordinate) *corev1.NodeAffinity {
	nodeSelectorRequirements := make([]corev1.NodeSelectorRequirement, 0)
	nodeSelectorRequirements = append(nodeSelectorRequirements,
		corev1.NodeSelectorRequirement{
			Key:      nuwav1.NuwaZoneFlag,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{c.Zone},
		},
	)
	if c.Rack != "" {
		nodeSelectorRequirements = append(nodeSelectorRequirements,
			corev1.NodeSelectorRequirement{
				Key:      nuwav1.NuwaRackFlag,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{c.Rack},
			},
		)
	}
	if c.Host != "" {
		nodeSelectorRequirements = append(nodeSelectorRequirements,
			corev1.NodeSelectorRequirement{
				Key:      nuwav1.NuwaHostFlag,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{c.Host},
			},
		)
	}

	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      nuwav1.NuwaZoneFlag,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{c.Zone},
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
					MatchExpressions: nodeSelectorRequirements,
				},
			},
		},
	}
}
