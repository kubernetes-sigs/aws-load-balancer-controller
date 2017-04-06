// Logging utilities used by the ALB Ingress controller.
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
		prefix := fmt.Sprintf("%s %s %s: ", identifier, ingressName, debugLevel)
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

// Uses awsutil.Prettify to print structs, but also removes '\n' for better logging.
func Prettify(i interface{}) string {
	return strings.Replace(awsutil.Prettify(i), "\n", "", -1)
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
