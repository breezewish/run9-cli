package api

import "testing"

func TestNewClientDisablesRestyWarnings(t *testing.T) {
	client := NewClient(" http://example.com/ ")

	if got, want := client.http.BaseURL, "http://example.com"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
	if !client.http.DisableWarn {
		t.Fatal("DisableWarn = false, want true")
	}
}
