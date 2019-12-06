package v1

import (
	"reflect"
	"testing"
)

func TestCoordinate_Equal(t *testing.T) {
	c1 := Coordinate{
		Room:     "R-A",
		Cabinet:  "C-B",
		Host:     "H-1",
		Replicas: 0,
	}
	c2 := Coordinate{
		Room:     "R-A",
		Cabinet:  "C-B",
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
			Room:     "R-A",
			Cabinet:  "C-B",
			Host:     "H-1",
			Replicas: 0,
		},
		Coordinate{
			Room:     "R-A",
			Cabinet:  "C-B",
			Host:     "H-2",
			Replicas: 2,
		},
	}

	cs2 := Coordinates{
		Coordinate{
			Room:     "R-A",
			Cabinet:  "C-B",
			Host:     "H-1",
			Replicas: 0,
		},
		Coordinate{
			Room:     "R-C",
			Cabinet:  "C-B",
			Host:     "H-1",
			Replicas: 2,
		},
	}
	expectCs := Coordinates{
		Coordinate{
			Room:     "R-A",
			Cabinet:  "C-B",
			Host:     "H-2",
			Replicas: 2,
		},
		Coordinate{
			Room:     "R-C",
			Cabinet:  "C-B",
			Host:     "H-1",
			Replicas: 2,
		},
	}

	cs3 := Difference(cs1, cs2)
	if !reflect.DeepEqual(expectCs, cs3) {
		t.Fatal("expcet equal faild")
	}
}
