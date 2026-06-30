package load

import (
	"errors"
	"testing"
)

func TestControllerApplyAndSnapshot(t *testing.T) {
	c := NewController(2, 64)
	t.Cleanup(c.Stop)

	got, err := c.Apply(Config{CPUPercent: 25, MemoryMB: 8})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if got.CPUPercent != 25 {
		t.Fatalf("CPUPercent = %d, want 25", got.CPUPercent)
	}
	if got.MemoryMB != 8 {
		t.Fatalf("MemoryMB = %d, want 8", got.MemoryMB)
	}
	if got.Workers != 2 {
		t.Fatalf("Workers = %d, want 2", got.Workers)
	}
}

func TestControllerRejectsInvalidCPU(t *testing.T) {
	c := NewController(1, 64)
	t.Cleanup(c.Stop)

	_, err := c.Apply(Config{CPUPercent: 101})
	if !errors.Is(err, ErrCPUOutOfRange) {
		t.Fatalf("error = %v, want ErrCPUOutOfRange", err)
	}
}

func TestControllerRejectsInvalidMemory(t *testing.T) {
	c := NewController(1, 64)
	t.Cleanup(c.Stop)

	_, err := c.Apply(Config{MemoryMB: 65})
	if !errors.Is(err, ErrMemoryOutOfRange) {
		t.Fatalf("error = %v, want ErrMemoryOutOfRange", err)
	}
}
