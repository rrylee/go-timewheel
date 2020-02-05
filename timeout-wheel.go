package go_timewheel

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	defaultTickInterval = time.Millisecond
	defaultNumBuckets   = 2048
	bitsInUint64        = 64
	cacheline           = 64
)

var (
	ErrSystemStopped = errors.New("Timeout System is stopped")

	defaultPoolSize = uint64(1 << uint(findMSB(uint64(runtime.NumCPU()))+2))
)

const (
	// states of the TimeoutWheel
	stopped = iota
	stopping
	running
)

type paddedMutex struct {
	sync.Mutex
	_ [cacheline - unsafe.Sizeof(sync.Mutex{})]byte
}

type TimeoutWheel struct {
	ticks        uint64
	muPool       []paddedMutex
	buckets      []timeoutList
	freelists    []timeoutList
	bucketMask   uint64
	poolMask     uint64
	tickInterval time.Duration
	done         chan struct{}
	calloutCh    chan timeoutList
	state        int8
}

type Option func(*opts)
type opts struct {
	tickInterval time.Duration
	size         uint64
	poolsize     uint64
}

func NewTimeoutWheel(options ...Option) *TimeoutWheel {
	opts := &opts{
		tickInterval: defaultTickInterval,
		size:         defaultNumBuckets,
		poolsize:     defaultPoolSize,
	}

	for _, option := range options {
		option(opts)
	}

	poolsize := opts.poolsize
	if opts.size < opts.poolsize {
		poolsize = opts.size
	}

	t := &TimeoutWheel{
		muPool:       make([]paddedMutex, poolsize),
		freelists:    make([]timeoutList, poolsize),
		buckets:      make([]timeoutList, opts.size),
		ticks:        0,
		poolMask:     poolsize - 1,
		bucketMask:   opts.size - 1,
		state:        stopped,
		tickInterval: opts.tickInterval,
	}

	t.Start()
	return t
}

func (tw *TimeoutWheel) Start() {
	tw.lockAllBuckets()
	defer tw.unlockAllBuckets()

	for tw.state != stopped {
		switch tw.state {
		case stopping:
			tw.unlockAllBuckets()
			<-tw.done
			tw.lockAllBuckets()
		case running:
			panic("Tried to start a running TimeoutWhee")
		}
	}

	tw.state = running
	tw.done = make(chan struct{})
	tw.calloutCh = make(chan timeoutList)

	go tw.doTick()
	go tw.doExpired()
}

func (tw *TimeoutWheel) Stop() {
	tw.lockAllBuckets()
	if tw.state == running {
		tw.state = stopping
		close(tw.calloutCh)
		for i := range tw.buckets {
			tw.freeBucketLocked(tw.buckets[i])
		}
	}
	tw.unlockAllBuckets()
	<-tw.done
}

func (tw *TimeoutWheel) lockAllBuckets() {
	for i := len(tw.muPool) - 1; i >= 0; i-- {
		tw.muPool[i].Lock()
	}
}

func (tw *TimeoutWheel) unlockAllBuckets() {
	for i := len(tw.muPool) - 1; i >= 0; i-- {
		tw.muPool[i].Unlock()
	}
}

func (tw *TimeoutWheel) doTick() {
	var expiredList timeoutList
	ticker := time.NewTicker(tw.tickInterval)

	for range ticker.C {
		atomic.AddUint64(&tw.ticks, 1)
		mu := tw.LockBucket(tw.ticks)
		if tw.state != running {
			mu.Unlock()
			break
		}

		bucket := &tw.buckets[tw.ticks&tw.bucketMask]
		timeout := bucket.head
		bucket.lastTick = tw.ticks

		for timeout != nil {
			next := timeout.next
			if timeout.deadline <= tw.ticks {
				timeout.state = timeoutExpired
				timeout.removeLocked()
				timeout.prependLocked(&expiredList)
			}
			timeout = next
		}

		mu.Unlock()
		if expiredList.head == nil {
			continue
		}

		select {
		case tw.calloutCh <- expiredList:
			expiredList.head = nil
		default:
		}
	}

	ticker.Stop()
}

func (tw *TimeoutWheel) freeBucketLocked(head timeoutList) {
	timeout := head.head
	for timeout != nil {
		next := timeout.next
		timeout.removeLocked()
		tw.putTimeoutLocked(timeout)
		timeout = next
	}
}

func (tw *TimeoutWheel) putTimeoutLocked(timeout *timeout) {
	freelist := &tw.freelists[timeout.deadline&tw.poolMask]
	timeout.state = timeoutInactive
	timeout.generation++
	timeout.prependLocked(freelist)
}

func (tw *TimeoutWheel) LockBucket(ticks uint64) *paddedMutex {
	mu := &tw.muPool[tw.poolMask&ticks]
	mu.Lock()
	return mu
}

func (tw *TimeoutWheel) doExpired() {
	for list := range tw.calloutCh {
		timeout := list.head
		for timeout != nil {
			timeout.mu.Lock()
			next := timeout.next
			expireCb := timeout.expireCb
			expireArg := timeout.expireArg
			tw.putTimeoutLocked(timeout)
			timeout.mu.Unlock()

			if expireCb != nil {
				expireCb(expireArg)
			}
			timeout = next
		}
	}
	tw.lockAllBuckets()
	tw.state = stopped
	tw.unlockAllBuckets()
	close(tw.done)
}

func (tw *TimeoutWheel) Schedule(d time.Duration, expireCb func(interface{}), arg interface{}) (Timeout, error) {
	dTicks := (d + tw.tickInterval - 1) / tw.tickInterval
	deadline := atomic.LoadUint64(&tw.ticks) + uint64(dTicks)
	timeout := tw.getTimeoutLocked(deadline)

	if tw.state != running {
		tw.putTimeoutLocked(timeout)
		timeout.mu.Unlock()
		return Timeout{}, ErrSystemStopped
	}

	bucket := &tw.buckets[deadline & tw.bucketMask]
	timeout.expireCb = expireCb
	timeout.expireArg = arg
	timeout.deadline = deadline
	timeout.state = timeoutActive
	out := Timeout{timeout: timeout, generation: timeout.generation}

	if bucket.lastTick >= deadline {
		tw.putTimeoutLocked(timeout)
		timeout.mu.Unlock()
		expireCb(arg)
		return out, nil
	}

	timeout.prependLocked(bucket)
	timeout.mu.Unlock()
	return out, nil
}

func (tw *TimeoutWheel) getTimeoutLocked(deadline uint64) *timeout {
	mu := &tw.muPool[deadline&tw.poolMask]
	mu.Lock()
	freelist := &tw.freelists[deadline&tw.poolMask]
	if freelist.head == nil {
		timeout := &timeout{mu: mu, wheel: tw}
		return timeout
	}
	timeout := freelist.head
	timeout.removeLocked()
	return timeout
}

func findMSB(value uint64) int {
	for i := bitsInUint64 - 1; i >= 0; i-- {
		if value&(1<<uint(i)) != 0 {
			return int(i)
		}
	}
	return -1
}
