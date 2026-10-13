package main

import "testing"

func TestParseXMRToAtomic(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{name: "fractional XMR", input: "0.05", want: 50_000_000_000},
		{name: "whole XMR", input: "1", want: 1_000_000_000_000},
		{name: "one atomic unit", input: "0.000000000001", want: 1},
		{name: "maximum SQLite integer", input: "9223372.036854775807", want: maxSQLiteInteger},
		{name: "zero", input: "0", wantErr: true},
		{name: "negative amount", input: "-0.1", wantErr: true},
		{name: "more than twelve decimals", input: "0.0000000000001", wantErr: true},
		{name: "exponent notation", input: "5e-2", wantErr: true},
		{name: "overflow", input: "9223372.036854775808", wantErr: true},
		{name: "missing whole part", input: ".05", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseXMRToAtomic(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseXMRToAtomic(%q) expected an error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseXMRToAtomic(%q) returned error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseXMRToAtomic(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseXMRToAtomicRoundTrip(t *testing.T) {
	atomic, err := ParseXMRToAtomic("0.050000000001")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got, want := AtomicToXMRString(atomic), "0.050000000001"; got != want {
		t.Fatalf("round trip = %q, want %q", got, want)
	}
}
