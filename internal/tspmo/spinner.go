package tspmo

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type status int

const (
	pending status = iota
	active
	done
	skipped
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type line struct {
	name   string
	status status
}

// progress renders real-time session creation feedback.
// In TTY mode it animates a spinner and overwrites lines in place.
// In non-TTY mode it prints each line once when completed.
type progress struct {
	mu       sync.Mutex
	lines    []line
	total    int
	done_    int // count of done+skipped
	isTTY    bool
	frame    int
	rendered bool
	stop     chan struct{}
}

func newProgress(names []string, tty bool) *progress {
	lines := make([]line, len(names))
	for i, n := range names {
		lines[i] = line{name: n, status: pending}
	}
	p := &progress{
		lines: lines,
		total: len(names),
		isTTY: tty,
		stop:  make(chan struct{}),
	}
	if tty {
		p.render()
		go p.spin()
	}
	return p
}

func (p *progress) indexOf(name string) int {
	for i, l := range p.lines {
		if l.name == name {
			return i
		}
	}
	return -1
}

func (p *progress) start(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i := p.indexOf(name); i >= 0 {
		p.lines[i].status = active
	}
	if p.isTTY {
		p.render()
	}
}

func (p *progress) markDone(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i := p.indexOf(name); i >= 0 {
		p.lines[i].status = done
		p.done_++
	}
	if p.isTTY {
		p.render()
	} else {
		fmt.Printf("[%d/%d] %s ✓\n", p.done_, p.total, name)
	}
}

func (p *progress) skip(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i := p.indexOf(name); i >= 0 {
		p.lines[i].status = skipped
		p.done_++
	}
	if p.isTTY {
		p.render()
	} else {
		fmt.Printf("[%d/%d] %s ✓ (already exists)\n", p.done_, p.total, name)
	}
}

// halt stops the spinner goroutine and does a final render.
func (p *progress) halt() {
	if p.isTTY {
		close(p.stop)
		p.mu.Lock()
		p.render()
		p.mu.Unlock()
	}
}

// spin animates the spinner in a goroutine.
func (p *progress) spin() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.mu.Lock()
			p.frame = (p.frame + 1) % len(frames)
			p.render()
			p.mu.Unlock()
		}
	}
}

// render draws all lines, moving the cursor up to overwrite previous output.
// Must be called with p.mu held.
func (p *progress) render() {
	// Move cursor up to overwrite previous lines (except on first render).
	if p.rendered {
		fmt.Printf("\033[%dA", p.total)
	}
	p.rendered = true
	seq := 0
	for _, l := range p.lines {
		seq++
		prefix := fmt.Sprintf("[%d/%d]", seq, p.total)
		switch l.status {
		case pending:
			fmt.Printf("\033[2K%s %s …\n", prefix, l.name)
		case active:
			fmt.Printf("\033[2K%s %s %s\n", prefix, l.name, frames[p.frame])
		case done:
			fmt.Printf("\033[2K%s %s ✓\n", prefix, l.name)
		case skipped:
			fmt.Printf("\033[2K%s %s ✓ (already exists)\n", prefix, l.name)
		}
	}
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
