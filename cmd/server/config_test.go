package main

import "testing"

func TestParseConfigDefaultsAndPort(t *testing.T) {
	t.Setenv("PORT", "")
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.addr != "127.0.0.1:19091" {
		t.Fatalf("addr=%s", cfg.addr)
	}
	t.Setenv("PORT", "19123")
	cfg, err = parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.addr != "127.0.0.1:19123" {
		t.Fatalf("addr=%s", cfg.addr)
	}
}

func TestParseConfigRejectsInvalidPort(t *testing.T) {
	t.Setenv("PORT", "8080x")
	if _, err := parseConfig(nil); err == nil {
		t.Fatal("expected invalid PORT error")
	}
}
