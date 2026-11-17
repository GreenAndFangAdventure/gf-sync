package main

import "testing"

func TestNumeric(t *testing.T) {
	for in, want := range map[string][4]uint16{
		"0.1.0":       {0, 1, 0, 0},
		"0.1.0-beta1": {0, 1, 0, 0},
		"1.2.3.4":     {1, 2, 3, 4},
		"2":           {2, 0, 0, 0},
	} {
		got, err := numeric(in)
		if err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "a.b", "1.2.3.4.5", "70000.0"} {
		if _, err := numeric(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestMakeIcon(t *testing.T) {
	if _, err := makeIcon(); err != nil {
		t.Fatal(err)
	}
}
