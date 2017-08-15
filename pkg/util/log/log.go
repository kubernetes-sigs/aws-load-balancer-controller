// Package log contains logging utilities used by the ALB Ingress controller.
package log

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/golang/glog"
)

const (
	leftBracket  = "["
	rightBracket = "]"
	identifier   = "[ALB-INGRESS]"
	debugLevel   = "[DEBUG]"
	infoLevel    = "[INFO]"
	warnLevel    = "[WARN]"
	errorLevel   = "[ERROR]"
)

type Logger struct {
	name string
}

const (
	// ERROR is for error log levels
	ERROR = iota
	// WARN is for warning log levels
	WARN
	// INFO is for info log levels
	INFO
	// DEBUG is for debug log levels
	DEBUG
)

var logLevel = INFO // Default log level

// New creates a new Logger.
// The name appears in the log lines.
func New(name string) *Logger {
	return &Logger{name: name}
}

// Debugf will print debug messages if debug logging is enabled
func (l *Logger) Debugf(format string, args ...interface{}) {
	debugf(format, l.name, args...)
}

// Infof will print info level messages
func (l *Logger) Infof(format string, args ...interface{}) {
	infof(format, l.name, args...)
}

// Warnf will print warning level messages
func (l *Logger) Warnf(format string, args ...interface{}) {
	warnf(format, l.name, args...)
}

// Errorf will print error level messages
func (l *Logger) Errorf(format string, args ...interface{}) {
	errorf(format, l.name, args...)
}

// Fatalf will print error level messages
func (l *Logger) Fatalf(format string, args ...interface{}) {
	fatalf(format, l.name, args...)
}

// Exitf will print error level messages and exit
func (l *Logger) Exitf(format string, args ...interface{}) {
	exitf(format, l.name, args...)
}

// debugf will print debug messages if debug logging is enabled
func debugf(format, ingressName string, args ...interface{}) {
	if logLevel < INFO {
		ingressName = leftBracket + ingressName + rightBracket
		prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, debugLevel)
		glog.InfoDepth(1, fmt.Sprintf(prefix+format, args...))
	}
}

// infof will print info level messages
func infof(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, infoLevel)
	glog.InfoDepth(1, fmt.Sprintf(prefix+format, args...))
}

// warnf will print warning level messages
func warnf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, warnLevel)
	glog.WarningDepth(1, fmt.Sprintf(prefix+format, args...))
}

// errorf will print error level messages
func errorf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, errorLevel)
	glog.ErrorDepth(1, fmt.Sprintf(prefix+format, args...))
}

// fatalf will print error level messages
func fatalf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, errorLevel)
	glog.FatalDepth(1, fmt.Sprintf(prefix+format, args...))
}

// Exitf will print error level messages and exit
func exitf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, errorLevel)
	glog.ExitDepth(1, fmt.Sprintf(prefix+format, args...))
}

// Prettify uses awsutil.Prettify to print structs, but also removes '\n' for better logging.
func Prettify(i interface{}) string {
	return strings.Replace(awsutil.Prettify(i), "\n", "", -1)
}

// SetLogLevel configures the logging level based off of the level string
func SetLogLevel(level string) {
	switch level {
	case "INFO":
		// default, do nothing
	case "WARN":
		logLevel = WARN
	case "ERROR":
		logLevel = ERROR
	case "DEBUG":
		logLevel = DEBUG
	default:
		// Invalid, do nothing
		infof("Log level read as \"%s\", defaulting to INFO. To change, set LOG_LEVEL environment variable to WARN, ERROR, or DEBUG.", "controller", level)
	}
}
