package lib

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/cookiejar"
	"sync"

	"github.com/oxtoacart/bpool"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"go.k6.io/k6/metrics"
)

// DialContexter is an interface that can dial with a context
type DialContexter interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// State provides the volatile state for a VU.
type State struct {
	// Global options and built-in metrics.
	//
	// TODO: remove them from here, the built-in metrics and the script options
	// are not part of a VU's unique "state", they are global and the same for
	// all VUs. Figure out how to thread them some other way, e.g. through the
	// TestPreInitState. The Samples channel might also benefit from that...
	Options        Options
	BuiltinMetrics *metrics.BuiltinMetrics

	// Logger. Avoid using the global logger.
	// TODO: change to logrus.FieldLogger when there is time to fix all the tests
	Logger *logrus.Logger

	// Current group; all emitted metrics are tagged with this.
	Group *Group

	// Networking equipment.
	Dialer DialContexter

	// TODO: move a lot of the things below to the k6/http ModuleInstance, see
	// https://github.com/grafana/k6/issues/2293.
	Transport http.RoundTripper
	CookieJar *cookiejar.Jar
	TLSConfig *tls.Config

	// Rate limits.
	RPSLimit *rate.Limiter

	// Sample channel, possibly buffered
	Samples chan<- metrics.SampleContainer

	// Buffer pool; use instead of allocating fresh buffers when possible.
	// TODO: maybe use https://golang.org/pkg/sync/#Pool ?
	BPool *bpool.BufferPool

	VUID, VUIDGlobal uint64
	Iteration        int64
	Tags             *TagMap
	// These will be assigned on VU activation.
	// Returns the iteration number of this VU in the current scenario.
	GetScenarioVUIter func() uint64
	// Returns the iteration number across all VUs in the current scenario
	// unique to this single k6 instance.
	// TODO: Maybe this doesn't belong here but in ScenarioState?
	GetScenarioLocalVUIter func() uint64
	// Returns the iteration number across all VUs in the current scenario
	// unique globally across k6 instances (taking into account execution
	// segments).
	GetScenarioGlobalVUIter func() uint64
}

// TagMap is a safe-concurrent Tags lookup.
type TagMap struct {
	mutex sync.RWMutex
	tags  *metrics.TagSet
}

// NewTagMap creates a TagMap,
// if a not-nil map is passed then it will be used as the internal map
// otherwise a new one will be created.
func NewTagMap(tags *metrics.TagSet) *TagMap {
	if tags == nil {
		panic("the metrics.TagSet must be initialized for creating a new lib.TagMap")
	}
	return &TagMap{
		tags:  tags,
		mutex: sync.RWMutex{},
	}
}

// BranchOut creates a TagSet starting from the state of the current Set.
func (tg *TagMap) BranchOut() *metrics.TagSet {
	tg.mutex.RLock()
	defer tg.mutex.RUnlock()
	return tg.tags.BranchOut()
}

// Set sets a Tag.
func (tg *TagMap) Set(k, v string) {
	tg.mutex.Lock()
	defer tg.mutex.Unlock()
	tg.tags.AddTag(k, v)
}

// Get returns the Tag value and true
// if the provided key has been found.
func (tg *TagMap) Get(k string) (string, bool) {
	tg.mutex.RLock()
	defer tg.mutex.RUnlock()
	return tg.tags.Get(k)
}

// Len returns the number of the set keys.
func (tg *TagMap) Len() int {
	tg.mutex.RLock()
	defer tg.mutex.RUnlock()
	return tg.tags.Len()
}

// Delete deletes a map's item based on the provided key.
func (tg *TagMap) Delete(k string) {
	tg.mutex.Lock()
	defer tg.mutex.Unlock()
	tg.tags.Delete(k)
}

// Clone returns a map with the entire set of items.
func (tg *TagMap) Clone() map[string]string {
	tg.mutex.RLock()
	defer tg.mutex.RUnlock()

	return tg.tags.Map()
}
