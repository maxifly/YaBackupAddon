package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_convertDateString(t *testing.T) {
	type args struct {
		modified string
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr assert.ErrorAssertionFunc
	}{
		{"2023-10-31T03:32:52+00:00", args{"2023-10-31T03:32:52+00:00"}, 1698723172000000000, assert.NoError},
		{"2023-10-31T03:32:52+10:00", args{"2023-10-31T03:32:52+10:00"}, 1698687172000000000, assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertDateString(tt.args.modified)
			if !tt.wantErr(t, err, fmt.Sprintf("convertDateString(%v)", tt.args.modified)) {
				return
			}
			assert.Equalf(t, tt.want, got.UTC().UnixNano(), "convertDateString(%v)", tt.args.modified)
		})
	}
}
