package throttle

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"golang.org/x/time/rate"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type throttleConfig struct {
	operationPtn *regexp.Regexp
	r            rate.Limit
	burst        int
}

var _ pflag.Value = &ServiceOperationsThrottleConfig{}

// serviceOperationsThrottleConfig is throttleConfig for each service's operations.
// It supports to be configured using flags with format like "${serviceID}:${operationRegex}={rate}:{burst}"
// e.g. "appmesh:DescribeMesh=1.3:5,appmesh:Create.*=1.7:3"
// Note: default throttle for each service will be cleared if any override is set for that service.
type ServiceOperationsThrottleConfig struct {
	// service:operationRegex:config
	value map[string][]throttleConfig
}

func (c *ServiceOperationsThrottleConfig) String() string {
	if c == nil {
		return ""
	}

	var configs []string
	var serviceIDs []string
	for serviceID := range c.value {
		serviceIDs = append(serviceIDs, serviceID)
	}
	sort.Strings(serviceIDs)
	for _, serviceID := range serviceIDs {
		for _, operationsThrottleConfig := range c.value[serviceID] {
			configs = append(configs, fmt.Sprintf("%s:%s=%v:%d",
				serviceID,
				operationsThrottleConfig.operationPtn.String(),
				operationsThrottleConfig.r,
				operationsThrottleConfig.burst,
			))
		}
	}
	return strings.Join(configs, ",")
}

func (c *ServiceOperationsThrottleConfig) Set(val string) error {
	valueOverride := make(map[string][]throttleConfig)
	configPairs := strings.Split(val, ",")
	for _, pair := range configPairs {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return errors.Errorf("%s must be formatted as serviceID:operationRegex=rate:burst", pair)
		}
		serviceIDOperationRegexPair := strings.Split(kv[0], ":")
		if len(serviceIDOperationRegexPair) != 2 {
			return errors.Errorf("%s must be formatted as serviceID:operationRegex", kv[0])
		}
		rateBurstPair := strings.Split(kv[1], ":")
		if len(rateBurstPair) != 2 {
			return errors.Errorf("%s must be formatted as rate:burst", kv[1])
		}
		serviceID := serviceIDOperationRegexPair[0]
		operationPtn, err := regexp.Compile(serviceIDOperationRegexPair[1])
		if err != nil {
			return errors.Errorf("%s must be valid regex expression for operation", serviceIDOperationRegexPair[1])
		}
		r, err := strconv.ParseFloat(rateBurstPair[0], 64)
		if err != nil {
			return errors.Errorf("%s must be valid float number as rate for operations per second", rateBurstPair[0])
		}
		burst, err := strconv.Atoi(rateBurstPair[1])
		if err != nil {
			return errors.Errorf("%s must be valid integer as burst for operations", rateBurstPair[1])
		}
		valueOverride[serviceID] = append(valueOverride[serviceID], throttleConfig{
			operationPtn: operationPtn,
			r:            rate.Limit(r),
			burst:        burst,
		})
	}

	if c.value == nil {
		c.value = make(map[string][]throttleConfig)
	}
	for k, v := range valueOverride {
		c.value[k] = v
	}
	return nil
}

func (c *ServiceOperationsThrottleConfig) Type() string {
	return "serviceOperationsThrottleConfig"
}
