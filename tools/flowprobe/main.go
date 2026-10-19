// Command flowprobe renders synthetic flow diagrams at 50/300/600 nodes and
// measures per-frame cost. Throwaway harness for PoC-4.
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components/flow"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/theme"
)

// Same fixed size the production node card uses (spec §9.3).
const (
	probeCardWidth  = 168
	probeCardHeight = 46
)

// sampleID is the node the brief's Step-3 drift print checks by name.
const sampleID = "cmp:example.com/probe/p0.T0"

// driftSnapshotFrame / driftCompareFrame reproduce the brief's Step 3:
// "Advance 等价操作" spaced >=500 ms apart, i.e. >=30 frames at ~60 Hz.
const (
	snapshotOffset = 5
	driftOffset    = 35
)

func main() {
	nodes := flag.Int("nodes", 600, "node count to render")
	frames := flag.Int("frames", 120, "frames to time")
	flag.Parse()

	_ = lucide.Box // keeps the onevra icons dependency used from Task 17

	data := syntheticData(*nodes)

	// The literal gate wording — 两次 tt.Advance 后节点位置不漂移 — is
	// checked deterministically in a headless Tester before the window run.
	headlessDriftCheck(data)

	var state flow.State
	_ = state.Ready() // zero State must be usable; it reports false
	var timings []time.Duration
	var snapshot map[string][2]float32
	frame, readyAt := 0, 0
	finished := false

	view := func(c *ui.Context) {
		frame++
		theme.Apply(c)
		start := time.Now()
		flow.View(c, &state, data, flowOptions(data))
		elapsed := time.Since(start)
		timings = append(timings, elapsed)
		fmt.Printf("frame %d: %s ready=%v\n", frame, elapsed.Round(time.Microsecond), state.Ready())
		if state.Ready() && readyAt == 0 {
			readyAt = frame
		}
		if readyAt > 0 {
			switch frame {
			case readyAt + snapshotOffset:
				snapshot = positions(&state, data)
				fmt.Printf("snapshot frame=%d nodes=%d zoom=%.4f\n", frame, len(snapshot), state.Zoom())
			case readyAt + driftOffset:
				maxAbs, sq, ok := drift(snapshot, positions(&state, data), sampleID)
				fmt.Printf("stable=%v drift=%v\n", ok, sq) // brief Step-3 print, node p0.T0
				fmt.Printf("liveDrift frames=%d..%d nodesCompared=%d maxAbsDelta=%.6f sampleSquaredDelta=%.6f\n",
					readyAt+snapshotOffset, frame, len(data.Nodes), maxAbs, sq)
			}
		}
		target := *frames
		if readyAt > 0 && readyAt+driftOffset > target {
			target = readyAt + driftOffset
		}
		if frame < target {
			c.AnimationFrame() // ask for the next real, vsync-paced frame
		} else if !finished {
			finished = true
			printSummary(*nodes, timings, readyAt, *frames)
			time.AfterFunc(0, mygo.App.Quit)
		}
	}

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:   "flowprobe",
			Width:   1400,
			Height:  900,
			Content: ui.View(view),
		})
		win.OnClosed(func() { mygo.App.Quit() })
		win.Show()
	})
	// Watchdog: if frames stop arriving (occluded window, hung backend) the
	// harness must not linger. Nothing worth keeping is in memory.
	watchdog := time.AfterFunc(60*time.Second, func() {
		fmt.Println("TIMEOUT after 60s without finishing")
		os.Exit(2)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	watchdog.Stop()
}

func flowOptions(data flow.Data) flow.Options {
	return flow.Options{
		Label:     fmt.Sprintf("%d nodes", len(data.Nodes)),
		Direction: flow.LeftToRight,
		NodeGap:   28,
		RankGap:   64,
	}
}

// headlessDriftCheck lays the diagram out in ui.NewTester, waits for
// state.Ready(), then applies exactly two tt.Advance(500ms) passes and
// compares every node's Position before and after each of them.
func headlessDriftCheck(data flow.Data) {
	var state flow.State
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		theme.Apply(c)
		flow.View(c, &state, data, flowOptions(data))
	}, 1400, 900)
	for i := 0; i < 200 && !state.Ready(); i++ {
		tt.Advance(16 * time.Millisecond)
	}
	if !state.Ready() {
		fmt.Printf("headlessDrift: NOT READY after %d advances\n", builds)
		return
	}
	before := positions(&state, data)
	found := 0
	for _, n := range data.Nodes {
		if _, ok := tt.Find(n.Label); ok {
			found++
		}
	}
	tt.Advance(500 * time.Millisecond) // Advance #1
	max1, sq1, ok1 := drift(before, positions(&state, data), sampleID)
	tt.Advance(500 * time.Millisecond) // Advance #2
	max2, sq2, ok2 := drift(before, positions(&state, data), sampleID)
	fmt.Printf("headlessDrift builds=%d ready=true labelsFound=%d/%d afterAdvance1 maxAbsDelta=%.6f sampleSquaredDelta=%.6f stable=%v; afterAdvance2 maxAbsDelta=%.6f sampleSquaredDelta=%.6f stable=%v\n",
		builds, found, len(data.Nodes), max1, sq1, ok1, max2, sq2, ok2)
}

func positions(state *flow.State, data flow.Data) map[string][2]float32 {
	m := make(map[string][2]float32, len(data.Nodes))
	for _, n := range data.Nodes {
		if x, y, ok := state.Position(n.ID); ok {
			m[n.ID] = [2]float32{x, y}
		}
	}
	return m
}

// drift reports the largest absolute per-axis delta over all nodes and the
// squared delta of the sample node; ok is false if any node is missing from
// one of the snapshots.
func drift(before, after map[string][2]float32, sample string) (maxAbs, sampleSq float64, ok bool) {
	ok = len(before) == len(after) && len(before) > 0
	for id, p := range before {
		q, found := after[id]
		if !found {
			ok = false
			continue
		}
		dx, dy := math.Abs(float64(q[0]-p[0])), math.Abs(float64(q[1]-p[1]))
		maxAbs = math.Max(maxAbs, math.Max(dx, dy))
		if id == sample {
			sampleSq = float64(q[0]-p[0])*float64(q[0]-p[0]) + float64(q[1]-p[1])*float64(q[1]-p[1])
		}
	}
	return maxAbs, sampleSq, ok
}

func printSummary(n int, timings []time.Duration, readyAt, framesFlag int) {
	fmt.Printf("first frame: %v\n", timings[0].Round(time.Microsecond))
	if readyAt == 0 || readyAt >= len(timings) {
		fmt.Printf("SUMMARY nodes=%d frames=%d readyAt=%d NOT-READY, all %d frames median=%v max=%v\n",
			n, len(timings), readyAt, len(timings), median(timings), maxD(timings))
		return
	}
	steady := timings[readyAt:] // every frame strictly after the ready one
	fmt.Printf("SUMMARY nodes=%d frames=%d readyAt=%d steadyFrames=%d steadyMedian=%v steadyP95=%v steadyMax=%v\n",
		n, len(timings), readyAt, len(steady),
		median(steady), percentile(steady, 0.95), maxD(steady))
}

func median(d []time.Duration) time.Duration { return percentile(d, 0.5) }

func percentile(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := make([]time.Duration, len(d))
	copy(s, d)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(p * float64(len(s)-1))
	return s[idx].Round(time.Microsecond)
}

func maxD(d []time.Duration) time.Duration {
	var m time.Duration
	for _, v := range d {
		if v > m {
			m = v
		}
	}
	return m.Round(time.Microsecond)
}

func syntheticData(n int) flow.Data {
	nodes := make([]flow.Node, 0, n)
	edges := make([]flow.Edge, 0, n*3)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("cmp:example.com/probe/p%d.T%d", i/10, i%10)
		nodes = append(nodes, flow.Node{
			ID:     id,
			Label:  fmt.Sprintf("Type%d", i),
			Width:  probeCardWidth,
			Height: probeCardHeight,
			Content: func(c *ui.Context) {
				ui.Row(c).Gap(6).Padding(8).Children(func() {
					ui.Text(c, fmt.Sprintf("Type%d", i)).FontSize(12)
				})
			},
		})
		if i > 9 {
			edges = append(edges, flow.Edge{Source: id, Target: fmt.Sprintf("cmp:example.com/probe/p%d.T%d", (i-7)/10, (i-7)%10)})
		}
	}
	return flow.Data{Nodes: nodes, Edges: edges}
}
