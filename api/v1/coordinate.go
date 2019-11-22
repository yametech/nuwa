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

// Coordinate defines the desired identity pod of node
// example
// coordinate:
//	-room: ROOM-01   / 机房名
//  -cabinet: CABINET-A1 /机柜名
//  -host: HOST-DELL-01 /主机名
//  -replicas: 2   /将发布多少个副本
type Coordinate struct {
	// +optional
	Room string `json:"room,omitempty"`
	// +optional
	Cabinet string `json:"cabinet,omitempty"`
	// +optional
	Host string `json:"host,omitempty"`
	// If not specified,default value 0 then the average distribution is sorted according to the above three fields
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
}

// Coordinates defines the desired identity pod of nodes
type Coordinates struct {
	Coordinates []Coordinate
}

func (c *Coordinates) Len() int { return len(c.Coordinates) }

func (c *Coordinates) Less(i, j int) bool {
	if c.Len() <= 1 {
		return false
	} else if c.Coordinates[i].Replicas == 0 {
		return false
	} else if c.Coordinates[i].Replicas >= c.Coordinates[j].Replicas {
		return false
	}
	return true
}

func (c *Coordinates) Swap(i, j int) {
	c.Coordinates[i], c.Coordinates[j] = c.Coordinates[j], c.Coordinates[i]
}
