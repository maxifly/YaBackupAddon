package mylogger

import "log"

type Logger struct {
	ErrorLog *log.Logger
	InfoLog  *log.Logger
	DebugLog *log.Logger
}
