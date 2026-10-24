package main

import "testing"

func TestWebLinkBase(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		account string
		want    string
	}{
		{name: "unset base URL yields empty link base", baseURL: "", account: "sales", want: ""},
		{name: "base URL gets account prefix", baseURL: "http://localhost:8080", account: "sales", want: "http://localhost:8080/a/sales"},
		{name: "trailing slash is trimmed", baseURL: "http://localhost:8080/", account: "service", want: "http://localhost:8080/a/service"},
		{name: "https base is kept", baseURL: "https://pm.example.com", account: "a-1", want: "https://pm.example.com/a/a-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webLinkBase(tt.baseURL, tt.account); got != tt.want {
				t.Errorf("webLinkBase(%q, %q) = %q, want %q", tt.baseURL, tt.account, got, tt.want)
			}
		})
	}
}
