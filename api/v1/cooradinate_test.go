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
	"reflect"
	"testing"
)

func TestCoordinate_Equal(t *testing.T) {
	c1 := Coordinate{
		Zone:     "R-A",
		Rack:     "C-B",
		Host:     "H-1",
		Replicas: 0,
	}
	c2 := Coordinate{
		Zone:     "R-A",
		Rack:     "C-B",
		Host:     "H-1",
		Replicas: 2,
	}
	if !c1.Equal(c2) {
		t.Fatal("expcet equal faild")
	}
}

func TestDifference(t *testing.T) {
	cs1 := Coordinates{
		Coordinate{
			Zone:     "R-A",
			Rack:     "C-B",
			Host:     "H-1",
			Replicas: 0,
		},
		Coordinate{
			Zone:     "R-A",
			Rack:     "C-B",
			Host:     "H-2",
			Replicas: 2,
		},
	}

	cs2 := Coordinates{
		Coordinate{
			Zone:     "R-A",
			Rack:     "C-B",
			Host:     "H-1",
			Replicas: 0,
		},
		Coordinate{
			Zone:     "R-C",
			Rack:     "C-B",
			Host:     "H-1",
			Replicas: 2,
		},
	}
	expectCs := Coordinates{
		Coordinate{
			Zone:     "R-A",
			Rack:     "C-B",
			Host:     "H-2",
			Replicas: 2,
		},
		Coordinate{
			Zone:     "R-C",
			Rack:     "C-B",
			Host:     "H-1",
			Replicas: 2,
		},
	}

	cs3 := Difference(cs1, cs2)
	if !reflect.DeepEqual(expectCs, cs3) {
		t.Fatal("expcet equal faild")
	}
}
