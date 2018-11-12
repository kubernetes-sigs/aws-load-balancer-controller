package utils

import (
	"errors"
	"fmt"
	"testing"

	"github.com/magiconair/properties/assert"
)

func TestSplitMapStringBool(t *testing.T) {
	for i, tc := range []struct {
		Str            string
		ExpectedOutput map[string]bool
		ExpectedErr    error
	}{
		{
			Str:            "",
			ExpectedOutput: make(map[string]bool),
			ExpectedErr:    nil,
		},
		{
			Str:            "key=true",
			ExpectedOutput: map[string]bool{"key": true},
			ExpectedErr:    nil,
		},
		{
			Str:            "key1=true,key2=false,key3=true",
			ExpectedOutput: map[string]bool{"key1": true, "key2": false, "key3": true},
			ExpectedErr:    nil,
		},
		{
			Str:            "key1=true, key2=false,key3=true ",
			ExpectedOutput: map[string]bool{"key1": true, "key2": false, "key3": true},
			ExpectedErr:    nil,
		},
		{
			Str:            "key1=true,key2=dummy",
			ExpectedOutput: nil,
			ExpectedErr:    errors.New("invalid mapStringBool: key2=dummy"),
		},
		{
			Str:            "key1=true,key2",
			ExpectedOutput: nil,
			ExpectedErr:    errors.New("invalid mapStringBool: key2"),
		},
	} {
		t.Run(fmt.Sprintf("case-%v", i), func(t *testing.T) {
			output, err := SplitMapStringBool(tc.Str)
			assert.Equal(t, output, tc.ExpectedOutput)
			assert.Equal(t, err, tc.ExpectedErr)
		})
	}
}
