package speedtester

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNormalizeSubscriptionRetriesWithFlagMeta(t *testing.T) {
	const raw = "not yaml"
	const clash = "proxies:\n  - name: a\n    type: ss\n"
	var requested []string
	body, used, err := NormalizeSubscription("https://example.com/sub?token=secret", func(url string) (string, error) {
		requested = append(requested, url)
		if strings.Contains(url, "flag=meta") {
			return clash, nil
		}
		return raw, nil
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !strings.Contains(body, "name: a") {
		t.Fatalf("body = %q", body)
	}
	if used != "https://example.com/sub?token=secret&flag=meta" {
		t.Fatalf("used = %q", used)
	}
	if len(requested) != 2 || requested[0] != "https://example.com/sub?token=secret" {
		t.Fatalf("requested = %#v", requested)
	}
}

func TestNormalizeSubscriptionAcceptsBase64WithoutRefetch(t *testing.T) {
	clash := "proxies:\n  - {name: a, type: ss}\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(clash))
	var fetches int
	body, used, err := NormalizeSubscription("https://example.com/sub", func(url string) (string, error) {
		fetches++
		return encoded, nil
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !strings.Contains(body, "name: a") {
		t.Fatalf("body = %q", body)
	}
	if used != "https://example.com/sub" || fetches != 1 {
		t.Fatalf("used = %q, fetches = %d", used, fetches)
	}
}

func TestNormalizeSubscriptionDoesNotOverrideExistingFlag(t *testing.T) {
	var fetches int
	_, _, err := NormalizeSubscription("https://example.com/sub?flag=clash", func(url string) (string, error) {
		fetches++
		return "not yaml", nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}
}

func TestNormalizeSubscriptionStopsWhenOnlyProvidersExist(t *testing.T) {
	body := "proxy-providers:\n  airport:\n    type: http\n    url: https://example.com/p\n"
	var fetches int
	got, used, err := NormalizeSubscription("https://example.com/sub", func(url string) (string, error) {
		fetches++
		return body, nil
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !strings.Contains(got, "airport") || used != "https://example.com/sub" || fetches != 1 {
		t.Fatalf("got success=%v used=%q fetches=%d", strings.Contains(got, "airport"), used, fetches)
	}
}
