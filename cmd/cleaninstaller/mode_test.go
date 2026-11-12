package main

import "testing"

func TestParseMode(t *testing.T) {
	cases := []struct {
		args    []string
		want    Mode
		wantErr bool
	}{
		{nil, ModeInstall, false},
		{[]string{"--launch"}, ModeLaunch, false},
		{[]string{"--bogus"}, 0, true},
		{[]string{"--launch", "extra"}, 0, true},
	}
	for _, c := range cases {
		got, err := parseMode(c.args)
		if (err != nil) != c.wantErr {
			t.Errorf("parseMode(%v) err=%v wantErr=%v", c.args, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseMode(%v)=%v want %v", c.args, got, c.want)
		}
	}
}
