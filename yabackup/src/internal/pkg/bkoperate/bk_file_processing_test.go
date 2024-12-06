package bkoperate

import (
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"ybg/internal/pkg/mylogger"
)

func Test_extractArchInfo(t *testing.T) {
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	logger := &mylogger.Logger{ErrorLog: errorLog,
		InfoLog:  errorLog,
		DebugLog: errorLog}

	info, err := extractArchInfo(logger, filepath.Join("../../../testresources", "correct_file.tar"))

	assert.Nil(t, err, "Error must be nil")
	assert.Equal(t, "5508d5ad", info.Slug, "Slug not equal")
	assert.Equal(t, "fileName1", info.Name, "Name not equal")
}

func Test_extractBadArchInfo(t *testing.T) {
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)
	infoLog := log.New(os.Stderr, "INFO\t", log.Ldate|log.Ltime|log.Lshortfile)

	logger := &mylogger.Logger{ErrorLog: errorLog,
		InfoLog:  infoLog,
		DebugLog: errorLog}

	type args struct {
		fileName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "not existent file", args: args{fileName: "NEfile"}, want: "cannot read tar file"},
		{name: "bad file", args: args{fileName: "bad_file.tar"}, want: "backup info not found"},
		{name: "without json file", args: args{fileName: "without_json_file.tar"}, want: "backup info not found"},
		{name: "bad json format file", args: args{fileName: "bad_json_format_file.tar"}, want: "cannot parse backup info"},
		{name: "bad json file", args: args{fileName: "bad_json_file.tar"}, want: "Necessary field not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractArchInfo(logger, filepath.Join("testresources", tt.args.fileName))
			assert.NotNilf(t, err, "Error expected. Test %s", tt.name)
			assert.True(t, strings.Contains(err.Error(), tt.want), "Error mast contain text %s. Real error message: %s. Test %s", tt.want, err.Error(), tt.name)
		})
	}
}
