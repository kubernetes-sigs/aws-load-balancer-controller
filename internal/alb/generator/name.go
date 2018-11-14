package generator

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/sg"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/lb"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tg"
)

var _ tg.NameGenerator = (*NameGenerator)(nil)
var _ lb.NameGenerator = (*NameGenerator)(nil)
var _ sg.NameGenerator = (*NameGenerator)(nil)

type NameGenerator struct {
	ALBNamePrefix string
}

func (gen *NameGenerator) NameLB(namespace string, ingressName string) string {
	hasher := md5.New()
	_, _ = hasher.Write([]byte(namespace + ingressName))
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
	_, _ = hasher.Write([]byte(LBName))
	_, _ = hasher.Write([]byte(serviceName))
	_, _ = hasher.Write([]byte(servicePort))
	_, _ = hasher.Write([]byte(protocol))
	_, _ = hasher.Write([]byte(targetType))

	return fmt.Sprintf("%.12s-%.19s", gen.ALBNamePrefix, hex.EncodeToString(hasher.Sum(nil)))
}

func (gen *NameGenerator) NameLBSG(namespace string, ingressName string) string {
	return gen.NameLB(namespace, ingressName)
}

func (gen *NameGenerator) NameInstanceSG(namespace string, ingressName string) string {
	return "instance-" + gen.NameLB(namespace, ingressName)
}
