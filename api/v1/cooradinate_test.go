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
