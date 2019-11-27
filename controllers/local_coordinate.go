package controllers

import (
	nuwav1 "github.com/yametech/nuwa/api/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nuwaRoomFlag    = "nuwa.io/room"
	nuwaCabinetFlag = "nuwa.io/cabinet"
)

type localCoordinate struct {
	Name                 string `json:"name,omitempty"`
	*nuwav1.Coordinate   `json:"coordinate,omitempty"`
	*corev1.NodeAffinity `json:"nodeAffinity,omitempty"`
	*corev1.NodeList     `json:"nodeList,omitempty"`
}

func (l *localCoordinate) generateCoordinateMatchLabels() (labs client.MatchingLabels, err error) {
	if l.Coordinate != nil && l.Coordinate.Room == "" {
		return nil, LeastNeedRoomErr
	}
	labs = make(client.MatchingLabels)
	labs[nuwaRoomFlag] = l.Coordinate.Room
	if l.Coordinate.Cabinet != "" {
		labs[nuwaCabinetFlag] = l.Coordinate.Cabinet
	}
	return labs, nil
}

func makeLocalCoordinates(coordinates nuwav1.Coordinates) (res []*localCoordinate) {
	for i := range coordinates {
		coordinate := coordinates[i]
		res = append(res, &localCoordinate{
			Name:       coordinateName(coordinate),
			Coordinate: &coordinate,
		})
	}
	return
}

func coordinateName(coordinate nuwav1.Coordinate) string {
	res := ""
	if coordinate.Room != "" {
		res += coordinate.Room
		res += "-"
		if coordinate.Cabinet != "" {
			res += coordinate.Cabinet
		} else {
			res += "Non"
		}
	}
	return res
}
