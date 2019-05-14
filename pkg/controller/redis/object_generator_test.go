package redis

import (
	"testing"
)

func Test_mapsEqual(t *testing.T) {
	var aNil, bNil map[string]string
	type args struct {
		a map[string]string
		b map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"nil", args{aNil, bNil}, true},
		{"empty", args{map[string]string{}, map[string]string{}}, true},
		{"match", args{map[string]string{"ok": "lol"}, map[string]string{"ok": "lol"}}, true},
		{"no-match", args{map[string]string{"ok": "lol"}, map[string]string{}}, false},
		{"no-match", args{map[string]string{"ok": "lol"}, map[string]string{"wow": "cool"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapsEqual(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("mapsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isSubset(t *testing.T) {
	type args struct {
		a map[string]string
		b map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"subset", args{map[string]string{"ok": "lol", "wow": "cool"}, map[string]string{"wow": "cool"}}, true},
		{"no-value-match", args{map[string]string{"ok": "lol", "wow": "cool"}, map[string]string{"wow": "such"}}, false},
		{"no-subset", args{map[string]string{"ok": "lol"}, map[string]string{"wow": "cool"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSubset(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("isSubset() = %v, want %v", got, tt.want)
			}
		})
	}
}
