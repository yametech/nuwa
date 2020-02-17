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

// Coordinate defines the desired identity pod of node
// example
// coordinate:
//	-room: ZONE-01   / 机房区域
//  -cabinet: RACK-A1 /机柜名
//  -host: HOST-DELL-01 /主机名
//  -replicas: 2   /将发布多少个副本

const (
	NuwaZoneFlag = "nuwa.io/zone"
	NuwaRackFlag = "nuwa.io/rack"
	NuwaHostFlag = "nuwa.io/host"
)

// +kubebuilder:object:root=false
// +k8s:deepcopy-gen=false
type Coordinate struct {
	// +optional
	Zone string `json:"zone,omitempty"`
	// +optional
	Rack string `json:"rack,omitempty"`
	// +optional
	Host string `json:"host,omitempty"`
	// If not specified,default value 0 then the average distribution is sorted according to the above three fields
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
}

func (c Coordinate) Equal(c1 Coordinate) bool {
	if c.Zone != c1.Zone {
		return false
	} else if c.Rack != c1.Rack {
		return false
	} else if c.Host != c1.Host {
		return false
	}
	return true
}

// Coordinates defines the desired identity pod of nodes
type Coordinates []Coordinate

func (c *Coordinates) Len() int { return len((*c)) }

func (c *Coordinates) Less(i, j int) bool {
	if c.Len() <= 1 {
		return false
	} else if (*c)[i].Zone >= (*c)[j].Zone {
		return false
	}
	return true
}

func (c *Coordinates) Swap(i, j int) {
	(*c)[i], (*c)[j] = (*c)[j], (*c)[i]
}

func Difference(slice1 Coordinates, slice2 Coordinates) Coordinates {
	var diff Coordinates
	for i := 0; i < 2; i++ {
		for _, s1 := range slice1 {
			found := false
			for _, s2 := range slice2 {
				if s1.Equal(s2) {
					found = true
					break
				}
			}
			// Coordinate not found. We add it to return slice
			if !found {
				diff = append(diff, s1)
			}
		}
		// Swap the slices, only if it was the first loop
		if i == 0 {
			slice1, slice2 = slice2, slice1
		}
	}

	return diff
}

func In(slice Coordinates, c Coordinate) bool {
	for _, s := range slice {
		if s.Equal(c) {
			return true
		}
	}
	return false
}
