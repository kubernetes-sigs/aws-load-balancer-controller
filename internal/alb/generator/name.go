package generator

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
	"regexp"
)

var _ tg.NameGenerator = (*NameGenerator)(nil)
var _ lb.NameGenerator = (*NameGenerator)(nil)

type NameGenerator struct {
	ALBNamePrefix string
}

func (gen *NameGenerator) NameLB(namespace string, ingressName string) string {
	hasher := md5.New()
	hasher.Write([]byte(namespace + ingressName))
	hash := hex.EncodeToString(hasher.Sum(nil))[:4]

	r, _ := regexp.Compile("[[:^alnum:]]")
	name := fmt.Sprintf("%s-%s-%s",
		r.ReplaceAllString(gen.ALBNamePrefix, "-"),
		r.ReplaceAllString(namespace, ""),
		r.ReplaceAllString(ingressName, ""),
	)
	if len(name) > 26 {
		name = name[:26]
	}
	name = name + "-" + hash
	return name
}

func (gen *NameGenerator) NameTG(namespace string, ingressName string, serviceName, servicePort string,
	targetType string, protocol string) string {
	LBName := gen.NameLB(namespace, ingressName)

	hasher := md5.New()
	hasher.Write([]byte(LBName))
	hasher.Write([]byte(serviceName))
	hasher.Write([]byte(servicePort))
	hasher.Write([]byte(protocol))
	hasher.Write([]byte(targetType))

	return fmt.Sprintf("%.12s-%.19s", gen.ALBNamePrefix, hex.EncodeToString(hasher.Sum(nil)))
}
