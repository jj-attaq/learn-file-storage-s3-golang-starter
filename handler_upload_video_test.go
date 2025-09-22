package main

import (
	"testing"
)

func TestFindGCD(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"1920x1080", 1920, 1080, 120},
		{"1280x720", 1280, 720, 80},
		{"9x16", 9, 16, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findGCD(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("findGCD(%d,%d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
