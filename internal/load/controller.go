package load

import (
	"context"
	"errors"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const (
	defaultTick        = 100 * time.Millisecond
	defaultMaxMemoryMB = 4096
)

var (
	ErrCPUOutOfRange    = errors.New("cpu_percent must be between 0 and 100")
	ErrMemoryOutOfRange = errors.New("memory_mb must be between 0 and max_memory_mb")
)

// Config is the desired synthetic resource usage for this process.
type Config struct {
	CPUPercent int `json:"cpu_percent"`
	MemoryMB   int `json:"memory_mb"`
}

// Snapshot describes the current synthetic workload state.
type Snapshot struct {
	Config
	Workers     int `json:"workers"`
	MaxMemoryMB int `json:"max_memory_mb"`
}

// Controller owns CPU workers and retained memory buffers.
type Controller struct {
	mu          sync.Mutex
	cancel      context.CancelFunc
	memory      []byte
	workers     int
	maxMemoryMB int
	cfg         Config
	tick        time.Duration
}

// NewController creates a load controller. workers <= 0 uses GOMAXPROCS.
func NewController(workers, maxMemoryMB int) *Controller {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if maxMemoryMB <= 0 {
		maxMemoryMB = defaultMaxMemoryMB
	}

	return &Controller{
		workers:     workers,
		maxMemoryMB: maxMemoryMB,
		tick:        defaultTick,
	}
}

// Apply replaces the currently active synthetic workload.
func (c *Controller) Apply(cfg Config) (Snapshot, error) {
	if cfg.CPUPercent < 0 || cfg.CPUPercent > 100 {
		return Snapshot{}, ErrCPUOutOfRange
	}
	if cfg.MemoryMB < 0 || cfg.MemoryMB > c.maxMemoryMB {
		return Snapshot{}, ErrMemoryOutOfRange
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}

	c.memory = allocateMemory(cfg.MemoryMB)
	c.cfg = cfg
	debug.FreeOSMemory()

	if cfg.CPUPercent > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		for i := 0; i < c.workers; i++ {
			go runCPUWorker(ctx, cfg.CPUPercent, c.tick)
		}
	}

	return c.snapshotLocked(), nil
}

// Snapshot returns the current synthetic workload state.
func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

// Stop cancels CPU workers and releases retained memory.
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.memory = nil
	c.cfg = Config{}
	debug.FreeOSMemory()
}

func (c *Controller) snapshotLocked() Snapshot {
	return Snapshot{
		Config:      c.cfg,
		Workers:     c.workers,
		MaxMemoryMB: c.maxMemoryMB,
	}
}

func allocateMemory(memoryMB int) []byte {
	if memoryMB == 0 {
		return nil
	}

	buf := make([]byte, memoryMB*1024*1024)
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = byte(i)
	}
	buf[len(buf)-1] = 1
	return buf
}

func runCPUWorker(ctx context.Context, cpuPercent int, tick time.Duration) {
	busyFor := time.Duration(int64(tick) * int64(cpuPercent) / 100)
	idleFor := tick - busyFor

	for {
		start := time.Now()
		for time.Since(start) < busyFor {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}

		if idleFor <= 0 {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		timer := time.NewTimer(idleFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
