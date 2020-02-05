package go_timewheel

const (
	// states of a Timeout
	timeoutInactive = iota
	timeoutExpired
	timeoutActive
)

// timeoutList is linkedList
type timeoutList struct {
	lastTick uint64
	head     *timeout
}

type Timeout struct {
	generation uint64
	timeout    *timeout
}

type timeout struct {
	mu         *paddedMutex
	expireCb   func(interface{})
	expireArg  interface{}
	next       *timeout
	prev       **timeout
	state      int8
	deadline   uint64
	generation uint64
	wheel      *TimeoutWheel
}

func (t *Timeout) Stop() bool {
	if t.timeout == nil {
		return false
	}
	t.timeout.mu.Lock()
	if t.timeout.generation != t.generation || t.timeout.state != timeoutActive {
		t.timeout.mu.Unlock()
		return false
	}
	t.timeout.removeLocked()
	t.timeout.wheel.putTimeoutLocked(t.timeout)
	t.timeout.mu.Unlock()
	return true
}

func (t *timeout) prependLocked(list *timeoutList) {
	if list.head == nil {
		t.prev = &list.head
	} else {
		t.prev = list.head.prev
		list.head.prev = &t.next
	}
	t.next = list.head
	list.head = t
}

func (t *timeout) removeLocked() {
	if t.next != nil {
		t.next.prev = t.prev
	}
	*t.prev = t.next
	t.next = nil
	t.prev = nil
}
