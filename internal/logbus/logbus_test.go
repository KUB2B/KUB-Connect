package logbus

import (
	"reflect"
	"testing"
)

func TestAppendCapsAtCapacity(t *testing.T) {
	b := New(3)
	for _, l := range []string{"a", "b", "c", "d"} {
		b.Append(l)
	}
	got := b.Lines()
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lines = %v, want %v", got, want)
	}
}

func TestSubscriberReceivesNewLines(t *testing.T) {
	b := New(10)
	var got []string
	cancel := b.Subscribe(func(line string) { got = append(got, line) })
	b.Append("one")
	b.Append("two")
	cancel()
	b.Append("three") // after cancel: must not be delivered
	want := []string{"one", "two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("subscriber got %v, want %v", got, want)
	}
}

func TestLinesReturnsCopy(t *testing.T) {
	b := New(10)
	b.Append("x")
	snap := b.Lines()
	snap[0] = "mutated"
	if b.Lines()[0] != "x" {
		t.Error("Lines() must return a copy; internal state was mutated")
	}
}
