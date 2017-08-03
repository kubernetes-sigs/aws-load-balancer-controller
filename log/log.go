// Package log contains logging utilities used by the ALB Ingress controller.
package log

import (
	"fmt"
	"os"
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

// Debugf will print debug messages if debug logging is enabled
func Debugf(format, ingressName string, args ...interface{}) {
	if logLevel < INFO {
		ingressName = leftBracket + ingressName + rightBracket
		prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, debugLevel)
		glog.Infof(prefix+format, args...)
	}
}

// Infof will print info level messages
func Infof(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, infoLevel)
	glog.Infof(prefix+format, args...)
}

// Warnf will print warning level messages
func Warnf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, warnLevel)
	glog.Infof(prefix+format, args...)
}

// Errorf will print error level messages
func Errorf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, errorLevel)
	glog.Infof(prefix+format, args...)
}

// Fatalf will print error level messages
func Fatalf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, errorLevel)
	glog.Fatalf(prefix+format, args...)
}

// Exitf will print error level messages
func Exitf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, errorLevel)
	glog.Infof(prefix+format, args...)
	os.Exit(0)
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
		Infof("Log level read as \"%s\", defaulting to INFO. To change, set LOG_LEVEL environment variable to WARN, ERROR, or DEBUG.", "controller", level)
	}
}
