// Logging utilities used by the ALB Ingress controller.
package log

import (
	"fmt"

	"github.com/golang/glog"
)

const (
	leftBracket  = "["
	rightBracket = "]"
	identifier   = "[ALB-INGRESS]"
	infoLevel    = "[INFO]"
	warnLevel    = "[WARN]"
	errorLevel   = "[ERROR]"
)

const (
	ERROR = iota
	WARN
	INFO
	DEBUG
)

var logLevel = INFO // Default log level

func Debugf(format, ingressName string, args ...interface{}) {
	if logLevel < INFO {
		ingressName = leftBracket + ingressName + rightBracket
		prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, infoLevel)
		glog.Infof(prefix+format, args...)
	}
}

func Infof(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, infoLevel)
	glog.Infof(prefix+format, args...)
}

func Warnf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, warnLevel)
	glog.Infof(prefix+format, args...)
}

func Errorf(format, ingressName string, args ...interface{}) {
	ingressName = leftBracket + ingressName + rightBracket
	prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, errorLevel)
	glog.Infof(prefix+format, args...)
}

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
