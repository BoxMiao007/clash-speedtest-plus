package speedtester

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestInstantSpeed(t *testing.T) {
	if InstantSpeed(0, time.Second) != 0 {
		t.Fatal("zero bytes should be 0")
	}
	if InstantSpeed(100, 0) != 0 {
		t.Fatal("zero duration should be 0")
	}
	got := InstantSpeed(100, time.Second)
	if got != 100 {
		t.Fatalf("got %v, want 100", got)
	}
}

func TestCountingReaderAddsBytes(t *testing.T) {
	var counter byteCounter
	src := bytes.NewReader([]byte("hello world"))
	n, err := io.Copy(io.Discard, &countingReader{r: src, c: &counter})
	if err != nil {
		t.Fatal(err)
	}
	if n != 11 || counter.load() != 11 {
		t.Fatalf("got n=%d count=%d", n, counter.load())
	}
}
