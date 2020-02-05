package go_timewheel

import (
	"testing"
)

func TestRun(t *testing.T) {
	var list timeoutList
	timeout1 := &timeout{}
	timeout1.prependLocked(&list)
	timeout2 := &timeout{}
	timeout2.prependLocked(&list)
}

//测试linkedList
func TestLinkedList(t *testing.T) {
	var list timeoutList

	timeout1 := &timeout{}
	timeout2 := &timeout{}

	timeout1.prependLocked(&list)
	timeout2.prependLocked(&list)

	if list.head != timeout2 {
		t.FailNow()
		return
	}

	timeout1.removeLocked()
	if list.head != timeout2 {
		t.FailNow()
		return
	}
	timeout2.removeLocked()
	timeout1.prependLocked(&list)
	timeout2.prependLocked(&list)

	timeout2.removeLocked()
	if list.head != timeout1 {
		t.FailNow()
		return
	}

	timeout1.removeLocked()
	if list.head != nil {
		t.FailNow()
		return
	}
}
