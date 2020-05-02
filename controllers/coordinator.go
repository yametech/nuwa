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

import (
	"context"
	"fmt"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CoordinateErr error

var ErrNeedAtLeastZone CoordinateErr = fmt.Errorf("%s", "coordinate need to specify at least zone")

type coordinator struct {
	Client       client.Client
	Index        int
	Name         string               `json:"name,omitempty"`
	Coordinate   *nuwav1.Coordinate   `json:"coordinate,omitempty"`
	NodeAffinity *corev1.NodeAffinity `json:"nodeAffinity,omitempty"`
	NodeNameList []string             `json:"nodeNameList,omitempty"`
}

func newLocalCoordinate(index int, client client.Client, coordinate nuwav1.Coordinate) (*coordinator, error) {
	if client == nil {
		return nil, fmt.Errorf("client is nil")
	}
	localCoordinator := &coordinator{
		Index:      index,
		Client:     client,
		Coordinate: &coordinate,
	}

	crdsName, err := coordinateName(localCoordinator.Coordinate)
	if err != nil {
		return nil, err
	}

	localCoordinator.Name = crdsName

	coordinateLabels, err := coordinateMatchLabels(&coordinate)
	if err != nil {
		return nil, err
	}

	hostLabels, err := hostMatchLabels(&coordinate)
	if err != nil {
		return nil, err
	}

	nodeList, err := findNodeWithLabels(client, hostLabels, coordinateLabels)
	if err != nil {
		return nil, err
	}

	localCoordinator.NodeNameList = nodeList

	nodeAffinity := organizationNodeAffinity(localCoordinator.Coordinate, localCoordinator.NodeNameList)

	localCoordinator.NodeAffinity = nodeAffinity

	return localCoordinator, nil
}

func makeLocalCoordinates(client client.Client, coordinates nuwav1.Coordinates) (cds []*coordinator, err error) {
	for i := range coordinates {
		c, newErr := newLocalCoordinate(i, client, coordinates[i])
		if newErr != nil {
			err = newErr
			return
		}
		cds = append(cds, c)
	}

	return
}

func findNodeWithLabels(cli client.Client, hostLabels, crdsLabels client.MatchingLabels) (nodeList []string, err error) {
	nodes := &corev1.NodeList{}
	if err = cli.List(context.TODO(), nodes, hostLabels, crdsLabels); err != nil {
		return
	}
	for i := range nodes.Items {
		nodeList = append(
			nodeList,
			nodes.Items[i].Name,
		)
	}

	return
}

func coordinateMatchLabels(c *nuwav1.Coordinate) (client.MatchingLabels, error) {
	if c.Zone == "" {
		return nil, ErrNeedAtLeastZone
	}
	cms := make(client.MatchingLabels)
	cms[nuwav1.NuwaZoneFlag] = c.Zone

	if c.Rack != "" {
		cms[nuwav1.NuwaRackFlag] = c.Rack
	}

	return cms, nil
}

func hostMatchLabels(c *nuwav1.Coordinate) (client.MatchingLabels, error) {
	cms := make(client.MatchingLabels)
	if c.Host != "" {
		cms[nuwav1.NuwaHostFlag] = c.Host
	}
	return cms, nil
}

func coordinateName(c *nuwav1.Coordinate) (string, error) {
	res := ""
	if c.Zone == "" {
		return "", fmt.Errorf("%s", "coordinate need to specify at least zone")
	}
	res += c.Zone
	res += "-"
	if c.Rack != "" {
		res += c.Rack
	} else {
		res += "Non"
	}
	return res, nil
}

func organizationNodeAffinity(c *nuwav1.Coordinate, nodeList []string) *corev1.NodeAffinity {
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
	if c.Host != "" && len(nodeList) > 0 {
		nodeSelectorRequirements = append(nodeSelectorRequirements,
			corev1.NodeSelectorRequirement{
				Key:      nuwav1.NuwaHostFlag,
				Operator: corev1.NodeSelectorOpIn,
				Values:   nodeList,
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
