package builder

import (
	"io"
	"time"

	"github.com/clearlinux/mixer-tools/log"
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
			var d time.Duration
			for _, e := range sw.entries {
				d += e.d
			}
			log.Info(log.Mixer, "=> %s (%s elapsed since start)", name,
				d.Truncate(time.Millisecond))
		} else {
			log.Info(log.Mixer, "=> %s", name)
		}
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
	log.Info(log.Mixer, "TIMINGS")
	for _, e := range sw.entries {
		log.Info(log.Mixer, "  %-*s %s", max, e.name, e.d.Truncate(time.Millisecond))
		sum += e.d
	}
	log.Info(log.Mixer, "TOTAL: %s", sum.Truncate(time.Millisecond))
}
