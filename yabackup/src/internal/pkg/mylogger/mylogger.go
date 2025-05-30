package mylogger

import "log"

type Logger struct {
	ErrorLog     *log.Logger
	InfoLog      *log.Logger
	DebugLog     *log.Logger
	debugEnabled bool
}

func New(errorLog *log.Logger,
	infoLog *log.Logger,
	debugLog *log.Logger) *Logger {
	return &Logger{
		ErrorLog:     errorLog,
		InfoLog:      infoLog,
		DebugLog:     debugLog,
		debugEnabled: true,
	}
}

func (app *Logger) DisableDebug() {
	app.debugEnabled = false
}

func (app *Logger) IsDebugEnabled() bool {
	return app.debugEnabled
}
