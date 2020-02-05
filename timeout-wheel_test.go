package go_timewheel

import (
	"testing"
	"time"
)

func TestStartStop(t *testing.T) {
	tw := NewTimeoutWheel()
	for i := 0; i < 3; i++ {
		tw.Stop()
		time.Sleep(1 * time.Second)
		tw.Start()
	}
	tw.Stop()
}

func TestStartStopConcurrent(t *testing.T) {
	tw := NewTimeoutWheel()

	for i := 0; i < 10; i++ {
		tw.Stop()
		go tw.Start()
		time.Sleep(1 * time.Millisecond)
	}
	tw.Stop()
}

func TestExpire(t *testing.T) {
	tw := NewTimeoutWheel()
	ch := make(chan int, 3)

	_, err := tw.Schedule(20*time.Millisecond, func(_ interface{}) { ch <- 20 }, nil)
	if err != nil {
		t.Error(err)
		return
	}

	_, err = tw.Schedule(10*time.Millisecond, func(_ interface{}) { ch <- 10 }, nil)
	if err != nil {
		t.Error(err)
		return
	}

	_, err = tw.Schedule(5*time.Millisecond, func(_ interface{}) { ch <- 5 }, nil)
	if err != nil {
		t.Error(err)
		return
	}

	output := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		d := time.Duration(((i+1)*5)+1) * time.Millisecond * 2
		select {
		case d := <-ch:
			output = append(output, d)
		case <-time.After(d):
			t.Error("after timeout")
			return
		}
	}

	if output[0] != 5 || output[1] != 10 || output[2] != 20 {
		t.Fail()
	}
	tw.Stop()
}

func TestTimeoutStop(t *testing.T) {
	tw := NewTimeoutWheel()
	ch := make(chan struct{})

	timeout, err := tw.Schedule(20*time.Millisecond, func(_ interface{}) { close(ch) }, nil)
	if err != nil {
		t.Fail()
		return
	}
	timeout.Stop()

	select {
	case <-time.After(30 * time.Millisecond):
	case <-ch:
		t.FailNow()
	}

	if timeout.generation == timeout.timeout.generation {
		t.Fail()
	}
}

func TestScheduleExpired(t *testing.T) {
	ch := make(chan struct{})
	tw := NewTimeoutWheel()
	tw.Stop()
	tw.ticks = 0
	tw.state = running

	tw.buckets[0].lastTick = 1
	timeout, _ := tw.Schedule(0, func(_ interface{}) { close(ch) }, nil)

	select {
	case <-ch:
	default:
		t.Fail()
	}

	if timeout.generation == timeout.timeout.generation {
		t.Fail()
	}
}
