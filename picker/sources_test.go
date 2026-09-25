package picker

import (
	"reflect"
	"testing"
)

func TestSplitSubscriptionTextKeepsCommaInsideURL(t *testing.T) {
	got := SplitSubscriptionText("https://example.com/sub?token=secret&flag=meta,ss, https://example.com/b")

	want := []string{
		"https://example.com/sub?token=secret&flag=meta,ss",
		"https://example.com/b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestSplitSubscriptionTextSplitsLinesAndDropsBlanks(t *testing.T) {
	got := SplitSubscriptionText(" https://example.com/a \n\nhttps://example.com/b\r\n")

	want := []string{
		"https://example.com/a",
		"https://example.com/b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
