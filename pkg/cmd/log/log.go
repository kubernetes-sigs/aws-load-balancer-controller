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
