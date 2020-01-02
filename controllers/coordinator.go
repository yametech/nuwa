package controllers

import (
	"context"
	"fmt"
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nuwaRoomFlag    = "nuwa.io/room"
	nuwaCabinetFlag = "nuwa.io/cabinet"
	nuwaHostFlag    = "nuwa.io/host"
)

type CoordinateErr error

var ErrNeedAtLeastRoom CoordinateErr = fmt.Errorf("%s", "coordinate need to specify at least room")

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
	ctx := context.TODO()
	if err = cli.List(ctx, nodes, hostLabels, crdsLabels); err != nil {
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
	if c.Room == "" {
		return nil, ErrNeedAtLeastRoom
	}
	cms := make(client.MatchingLabels)
	cms[nuwaRoomFlag] = c.Room

	if c.Cabinet != "" {
		cms[nuwaCabinetFlag] = c.Cabinet
	}

	return cms, nil
}

func hostMatchLabels(c *nuwav1.Coordinate) (client.MatchingLabels, error) {
	cms := make(client.MatchingLabels)
	if c.Host != "" {
		cms[nuwaHostFlag] = c.Host
	}
	return cms, nil
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

func organizationNodeAffinity(c *nuwav1.Coordinate, nodeList []string) *corev1.NodeAffinity {
	nodeSelectorRequirements := make([]corev1.NodeSelectorRequirement, 0)
	nodeSelectorRequirements = append(nodeSelectorRequirements,
		corev1.NodeSelectorRequirement{
			Key:      nuwaRoomFlag,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{c.Room},
		},
	)
	if c.Cabinet != "" {
		nodeSelectorRequirements = append(nodeSelectorRequirements,
			corev1.NodeSelectorRequirement{
				Key:      nuwaCabinetFlag,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{c.Cabinet},
			},
		)
	}
	if c.Host != "" {
		nodeSelectorRequirements = append(nodeSelectorRequirements,
			corev1.NodeSelectorRequirement{
				Key:      nuwaHostFlag,
				Operator: corev1.NodeSelectorOpIn,
				Values:   nodeList,
			},
		)
	}

	return &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						corev1.NodeSelectorRequirement{
							Key:      nuwaRoomFlag,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{c.Room},
						}}}}},
		PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
			corev1.PreferredSchedulingTerm{Weight: 100,
				Preference: corev1.NodeSelectorTerm{MatchExpressions: nodeSelectorRequirements},
			}}}
}
