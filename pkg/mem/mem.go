// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mem

import (
	"runtime"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/prometheus/client_golang/prometheus"
)

var startOnce sync.Once

const (
	namespace = "vmservice"
	subsystem = "memory"

	keyNow           = "now"
	keyBuckHashSys   = "buckHashSys"
	keyDebugGC       = "debugGC"
	keyEnableGC      = "enableGC"
	keyFrees         = "frees"
	keyGcCPUFraction = "gcCPUFraction"
	keyGcSys         = "gcSys"
	keyHeapAlloc     = "heapAlloc"
	keyHeapIdle      = "heapIdle"
	keyHeapInUse     = "heapInUse"
	keyHeapObjects   = "heapObjects"
	keyHeapReleased  = "heapReleased"
	keyHeapSys       = "heapSys"
	keyLastGC        = "lastGC"
	keyLive          = "live"
	keyLookups       = "lookups"
	keyMCacheInUse   = "mCacheInUse"
	keyMCacheSys     = "mCacheSys"
	keyMSpanInUse    = "mSpanInUse"
	keyMSpanSys      = "mSpanSys"
	keyMallocs       = "mallocs"
	keyNextGC        = "nextGC"
	keyNumForcedGC   = "numForcedGC"
	keyNumGC         = "numGC"
	keyOtherSys      = "otherSys"
	keyPauseTotalNs  = "pauseTotalNs"
	keyStackInUse    = "stackInUse"
	keyStackSys      = "stackSys"
	keySys           = "sys"
	keyTotalAlloc    = "totalAlloc"
)

var gauges struct {
	buckHashSys   prometheus.Gauge
	frees         prometheus.Gauge
	gcCPUFraction prometheus.Gauge
	gcSys         prometheus.Gauge
	heapAlloc     prometheus.Gauge
	heapIdle      prometheus.Gauge
	heapInUse     prometheus.Gauge
	heapObjects   prometheus.Gauge
	heapReleased  prometheus.Gauge
	heapSys       prometheus.Gauge
	live          prometheus.Gauge
	lookups       prometheus.Gauge
	mCacheInUse   prometheus.Gauge
	mCacheSys     prometheus.Gauge
	mSpanInUse    prometheus.Gauge
	mSpanSys      prometheus.Gauge
	mallocs       prometheus.Gauge
	nextGC        prometheus.Gauge
	numForcedGC   prometheus.Gauge
	numGC         prometheus.Gauge
	otherSys      prometheus.Gauge
	pauseTotalNs  prometheus.Gauge
	stackInUse    prometheus.Gauge
	stackSys      prometheus.Gauge
	sys           prometheus.Gauge
	totalAlloc    prometheus.Gauge
}

func ng(name, help string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})
}

// Start begins the goroutine that periodically logs the results of
// runtime.ReadMemStats(*runtime.MemStats).
func Start(
	logger logr.Logger,
	interval time.Duration,
	mustRegisterMetricsFn func(c ...prometheus.Collector)) {

	startOnce.Do(func() {
		if mustRegisterMetricsFn != nil {
			initMetrics(mustRegisterMetricsFn)
		}
		go start(logger, interval, mustRegisterMetricsFn != nil)
	})
}

func initMetrics(fn func(c ...prometheus.Collector)) {
	gauges.buckHashSys = ng(
		keyBuckHashSys,
		"Bytes of memory in profiling bucket hash tables",
	)
	gauges.frees = ng(
		keyFrees,
		"Cumulative count of heap objects freed",
	)
	gauges.gcCPUFraction = ng(
		keyGcCPUFraction,
		"Fraction of this program's available CPU time used by the GC since the program started",
	)
	gauges.gcSys = ng(
		keyGcSys,
		"Bytes of memory in garbage collection metadata",
	)
	gauges.heapAlloc = ng(
		keyHeapAlloc,
		"Bytes of allocated heap objects",
	)
	gauges.heapIdle = ng(
		keyHeapIdle,
		"Bytes in idle (unused) spans",
	)
	gauges.heapInUse = ng(
		keyHeapInUse,
		"Bytes in in-use spans",
	)
	gauges.heapObjects = ng(
		keyHeapObjects,
		"Number of allocated heap objects",
	)
	gauges.heapReleased = ng(
		"heapReleased",
		"Bytes of physical memory returned to the OS",
	)
	gauges.heapSys = ng(
		keyHeapSys,
		"Bytes of heap memory obtained from the OS",
	)
	gauges.live = ng(
		keyLive,
		"Number of live objects, i.e. mallocs - frees",
	)
	gauges.lookups = ng(
		keyLookups,
		"Number of pointer lookups performed by the runtime",
	)
	gauges.mCacheInUse = ng(
		keyMCacheInUse,
		"Bytes of allocated mcache structures",
	)
	gauges.mCacheSys = ng(
		keyMCacheSys,
		"Bytes of memory obtained from the OS for mcache structures",
	)
	gauges.mSpanInUse = ng(
		keyMSpanInUse,
		"Bytes of allocated mspan structures",
	)
	gauges.mSpanSys = ng(
		keyMSpanSys,
		"Bytes of memory obtained from the OS for mspan structures",
	)
	gauges.mallocs = ng(
		keyMallocs,
		"Cumulative count of heap objects allocated",
	)
	gauges.nextGC = ng(
		keyNextGC,
		"Target heap size of the next GC cycle",
	)
	gauges.numForcedGC = ng(
		keyNumForcedGC,
		"Number of GC cycles that were forced by the application calling the GC function",
	)
	gauges.numGC = ng(
		keyNumGC,
		"Number of completed GC cycles",
	)
	gauges.otherSys = ng(
		keyOtherSys,
		"Bytes of memory in miscellaneous off-heap runtime allocations.",
	)
	gauges.pauseTotalNs = ng(
		keyPauseTotalNs,
		"Cumulative nanoseconds in GC stop-the-world pauses since the program started",
	)
	gauges.stackInUse = ng(
		keyStackInUse,
		"Bytes in stack spans",
	)
	gauges.stackSys = ng(
		keyStackSys,
		"Bytes of stack memory obtained from the OS",
	)
	gauges.sys = ng(
		keySys,
		"Total bytes of memory obtained from the OS",
	)
	gauges.totalAlloc = ng(
		keyTotalAlloc,
		"Cumulative bytes allocated for heap objects",
	)

	// Register the metrics.
	fn(
		gauges.buckHashSys,
		gauges.frees,
		gauges.gcCPUFraction,
		gauges.gcSys,
		gauges.heapAlloc,
		gauges.heapIdle,
		gauges.heapInUse,
		gauges.heapObjects,
		gauges.heapReleased,
		gauges.heapSys,
		gauges.live,
		gauges.lookups,
		gauges.mCacheInUse,
		gauges.mCacheSys,
		gauges.mSpanInUse,
		gauges.mSpanSys,
		gauges.mallocs,
		gauges.nextGC,
		gauges.numForcedGC,
		gauges.numGC,
		gauges.otherSys,
		gauges.pauseTotalNs,
		gauges.stackInUse,
		gauges.stackSys,
		gauges.sys,
		gauges.totalAlloc,
	)
}

func start(
	logger logr.Logger,
	interval time.Duration,
	metricsEnabled bool) {

	t := time.NewTicker(interval)

	for {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		liveObjs := m.Mallocs - m.Frees

		logger.Info(
			"MemStats",
			"now", time.Now().UnixNano(), // use same scale as lastGC
			keyBuckHashSys, m.BuckHashSys,
			keyDebugGC, m.DebugGC,
			keyEnableGC, m.EnableGC,
			keyFrees, m.Frees,
			keyGcCPUFraction, m.GCCPUFraction,
			keyGcSys, m.GCSys,
			keyHeapAlloc, m.HeapAlloc,
			keyHeapIdle, m.HeapIdle,
			keyHeapInUse, m.HeapInuse,
			keyHeapObjects, m.HeapObjects,
			keyHeapReleased, m.HeapReleased,
			keyHeapSys, m.HeapSys,
			keyLastGC, m.LastGC,
			keyLive, liveObjs,
			keyLookups, m.Lookups,
			keyMCacheInUse, m.MCacheInuse,
			keyMCacheSys, m.MCacheSys,
			keyMSpanInUse, m.MSpanInuse,
			keyMSpanSys, m.MSpanSys,
			keyMallocs, m.Mallocs,
			keyNextGC, m.NextGC,
			keyNumForcedGC, m.NumForcedGC,
			keyNumGC, m.NumGC,
			keyOtherSys, m.OtherSys,
			keyPauseTotalNs, m.PauseTotalNs,
			keyStackInUse, m.StackInuse,
			keyStackSys, m.StackSys,
			keySys, m.Sys,
			keyTotalAlloc, m.TotalAlloc,
		)

		if metricsEnabled {
			gauges.buckHashSys.Set(float64(m.BuckHashSys))
			gauges.frees.Set(float64(m.Frees))
			gauges.gcCPUFraction.Set(m.GCCPUFraction)
			gauges.gcSys.Set(float64(m.GCSys))
			gauges.heapAlloc.Set(float64(m.HeapAlloc))
			gauges.heapIdle.Set(float64(m.HeapIdle))
			gauges.heapInUse.Set(float64(m.HeapInuse))
			gauges.heapObjects.Set(float64(m.HeapObjects))
			gauges.heapReleased.Set(float64(m.HeapReleased))
			gauges.heapSys.Set(float64(m.HeapSys))
			gauges.live.Set(float64(liveObjs))
			gauges.lookups.Set(float64(m.Lookups))
			gauges.mCacheInUse.Set(float64(m.MCacheInuse))
			gauges.mCacheSys.Set(float64(m.MCacheSys))
			gauges.mSpanInUse.Set(float64(m.MSpanInuse))
			gauges.mSpanSys.Set(float64(m.MSpanSys))
			gauges.mallocs.Set(float64(m.Mallocs))
			gauges.nextGC.Set(float64(m.NextGC))
			gauges.numForcedGC.Set(float64(m.NumForcedGC))
			gauges.numGC.Set(float64(m.NumGC))
			gauges.otherSys.Set(float64(m.OtherSys))
			gauges.pauseTotalNs.Set(float64(m.PauseTotalNs))
			gauges.stackInUse.Set(float64(m.StackInuse))
			gauges.stackSys.Set(float64(m.StackSys))
			gauges.sys.Set(float64(m.Sys))
			gauges.totalAlloc.Set(float64(m.TotalAlloc))
		}

		// Wait for the next tick.
		<-t.C
	}
}
