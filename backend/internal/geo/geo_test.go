package geo

import (
	"math"
	"testing"
)

func TestDistanceMetersZero(t *testing.T) {
	if distance := DistanceMeters(55.7558, 37.6173, 55.7558, 37.6173); distance != 0 {
		t.Fatalf("expected zero, got %f", distance)
	}
}

func TestDistanceMetersOneDegreeLatitude(t *testing.T) {
	distance := DistanceMeters(0, 0, 1, 0)
	if math.Abs(distance-111194.9) > 5 {
		t.Fatalf("unexpected distance: %f", distance)
	}
}

func TestDistanceMetersShort(t *testing.T) {
	distance := DistanceMeters(55.7558, 37.6173, 55.7568, 37.6173)
	if math.Abs(distance-111.2) > 2 {
		t.Fatalf("unexpected distance: %f", distance)
	}
}
