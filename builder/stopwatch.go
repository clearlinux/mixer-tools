package builder

import (
	"fmt"
	"io"
	"time"
)

// stopWatch keeps track of a sequence of durations. Use Start and Stop to mark the sections, then
// write the final result with WriteSummary.
type stopWatch struct {
	entries []stopWatchEntry
	t       time.Time
	w       io.Writer
}

type stopWatchEntry struct {
	name string
	d    time.Duration
	used bool
}

func (sw *stopWatch) Start(name string) {
	if sw.w != nil {
		if len(sw.entries) > 0 {
			fmt.Println()
		}
		fmt.Fprintf(sw.w, "=> %s\n", name)
	}
	sw.entries = append(sw.entries, stopWatchEntry{name: name})
	sw.t = time.Now()
}

func (sw *stopWatch) Stop() {
	if len(sw.entries) == 0 {
		return
	}
	last := len(sw.entries) - 1
	if sw.entries[last].used {
		sw.entries = append(sw.entries, sw.entries[last])
		last++
	}
	e := &sw.entries[last]
	e.used = true
	e.d = time.Since(sw.t)
}

func (sw *stopWatch) WriteSummary(w io.Writer) {
	if len(sw.entries) == 0 {
		return
	}
	max := 0
	for _, e := range sw.entries {
		if len(e.name) > max {
			max = len(e.name)
		}
	}
	var sum time.Duration
	fmt.Fprintf(w, "\nTIMINGS\n")
	for _, e := range sw.entries {
		fmt.Fprintf(w, "  %-*s %s\n", max, e.name, e.d.Truncate(time.Millisecond))
		sum += e.d
	}
	fmt.Fprintf(w, "TOTAL: %s\n", sum.Truncate(time.Millisecond))
}
