// Copyright 2019 The redis-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

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
