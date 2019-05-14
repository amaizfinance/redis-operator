package redis

import (
	"math"
	"reflect"
	"testing"
)

func Test_deepContains(t *testing.T) {
	// test types
	type basicStruct struct {
		Tstring  string
		Tbool    bool
		Tint     int
		Tuint    uint
		Tfloat32 float32

		unexported int
	}

	type compositeStruct struct {
		Tarray     [2]basicStruct
		Tslice     []basicStruct
		Tinterface interface{}
		Tmap       map[basicStruct]basicStruct
		Tpointer   *basicStruct
		Tstruct    basicStruct
	}

	// values to test against
	basic := basicStruct{
		Tstring:  "o",
		Tbool:    true,
		Tint:     1,
		Tuint:    uint(1),
		Tfloat32: math.Pi,

		unexported: 1,
	}

	composite := compositeStruct{
		Tarray:     [2]basicStruct{basic, basic},
		Tslice:     []basicStruct{basic, basic, basic},
		Tinterface: basic,
		Tpointer:   &basic,
		Tstruct:    basic,
		Tmap:       map[basicStruct]basicStruct{basic: basic, basicStruct{Tstring: "o"}: basic},
	}

	f := func() {}
	type args struct {
		x interface{}
		y interface{}
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		// positive tests
		// basic
		{"basic-empty", args{basicStruct{}, basicStruct{}}, true},
		{"basic-nil-zero", args{[]byte{}, []byte(nil)}, true},
		{"basic-empty", args{basic, basicStruct{}}, true},
		{"basic-unexported", args{basic, basicStruct{unexported: 1}}, true},
		{"basic-unexported-random", args{basic, basicStruct{unexported: 123}}, true},
		{"basic-Tstring-true", args{basic, basicStruct{Tstring: "o"}}, true},
		{"basic-Tbool-true", args{basic, basicStruct{Tbool: true}}, true},
		{"basic-Tint-true", args{basic, basicStruct{Tint: 1}}, true},
		{"basic-Tuint-true", args{basic, basicStruct{Tuint: uint(1)}}, true},
		{"basic-Tfloat32-true", args{basic, basicStruct{Tfloat32: math.Pi}}, true},
		// composite
		{"composite-empty", args{compositeStruct{}, compositeStruct{}}, true},
		{"composite-equal", args{composite, composite}, true},
		{"composite-Tarray-true", args{composite, compositeStruct{Tarray: [2]basicStruct{basicStruct{Tstring: "o"}, basicStruct{}}}}, true},
		{"composite-Tslice-true", args{composite, compositeStruct{Tslice: []basicStruct{basicStruct{Tstring: "o"}}}}, true},
		{"composite-Tinterface-true", args{composite, compositeStruct{Tslice: []basicStruct{basicStruct{Tstring: "o"}}}}, true},
		{"composite-Tstruct-true", args{composite, compositeStruct{Tstruct: basicStruct{Tstring: "o"}}}, true},
		{"composite-Tpointer-true", args{composite, compositeStruct{Tpointer: &basicStruct{Tstring: "o"}}}, true},
		{"composite-Tpointer-deep-true", args{compositeStruct{Tinterface: &[]basicStruct{basic}}, compositeStruct{Tinterface: &[]basicStruct{basic}}}, true},
		{"composite-Tmap-true", args{composite, compositeStruct{Tmap: map[basicStruct]basicStruct{basic: basicStruct{Tstring: "o"}}}}, true},

		// negative tests
		// basic
		{"nil", args{basic, nil}, false},
		{"nil", args{nil, basic}, false},
		{"apples-oranges", args{4, "5"}, false},
		{"apples-oranges", args{basicStruct{Tfloat32: math.Pi}, basicStruct{Tstring: "o"}}, false},
		{"basic-Tstring-false", args{basic, basicStruct{Tstring: "oo"}}, false},
		{"basic-Tint-false", args{basic, basicStruct{Tint: 2}}, false},
		{"basic-Tuint-false", args{basic, basicStruct{Tuint: uint(2)}}, false},
		{"basic-Tfloat32-false", args{basic, basicStruct{Tfloat32: math.E}}, false},
		// composite
		{"composite-Tarray-false", args{composite, compositeStruct{Tarray: [2]basicStruct{basicStruct{Tstring: "oo"}, basicStruct{}}}}, false},
		{"composite-Tslice-false", args{composite, compositeStruct{Tslice: []basicStruct{basicStruct{Tstring: "oo"}, basicStruct{}, basicStruct{}, basicStruct{}}}}, false},
		{"composite-Tinterface-false", args{composite, compositeStruct{Tslice: []basicStruct{basicStruct{Tstring: "oo"}}}}, false},
		{"composite-Tinterface-deepFunc-false", args{compositeStruct{Tinterface: f}, compositeStruct{Tinterface: f}}, false},
		{"composite-Tstruct-false", args{composite, compositeStruct{Tstruct: basicStruct{Tstring: "oo"}}}, false},
		{"composite-Tpointer-false", args{composite, compositeStruct{Tpointer: &basicStruct{Tstring: "oo"}}}, false},
		{"composite-Tpointer-deep-false", args{compositeStruct{Tinterface: &[]basicStruct{basic}}, compositeStruct{Tinterface: &[]basicStruct{basicStruct{Tstring: "oo"}}}}, false},
		{"composite-Tpointer-deepType-false", args{compositeStruct{Tinterface: &[]basicStruct{basic}}, compositeStruct{Tinterface: &[]int{}}}, false},
		{"composite-Tmap-falseValue", args{composite, compositeStruct{Tmap: map[basicStruct]basicStruct{basic: basicStruct{Tstring: "oo"}}}}, false},
		{"composite-Tmap-falseKey", args{composite, compositeStruct{Tmap: map[basicStruct]basicStruct{basicStruct{}: basicStruct{Tstring: "o"}}}}, false},
		{"composite-Tmap-falseLen", args{composite, compositeStruct{Tmap: map[basicStruct]basicStruct{basicStruct{Tstring: "o"}: basicStruct{}, basicStruct{Tstring: "oo"}: basicStruct{}, basicStruct{Tstring: "ooo"}: basicStruct{}}}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deepContains(tt.args.x, tt.args.y); got != tt.want {
				t.Errorf("deepContains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isEmptyValue(t *testing.T) {
	tests := []struct {
		name string
		v    reflect.Value
		want bool
	}{
		{"final-false", reflect.ValueOf(struct{}{}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmptyValue(tt.v); got != tt.want {
				t.Errorf("isEmptyValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
