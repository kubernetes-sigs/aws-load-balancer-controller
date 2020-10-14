package utils

import (
	"context"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"net"
)

// WaitUntilDNSNameAvailable will wait until the DNSName is available
func WaitUntilDNSNameAvailable(ctx context.Context, hostName string) error {
	return wait.PollImmediateUntil(PollIntervalMedium, func() (bool, error) {
		_, err := net.LookupHost(hostName)
		if err != nil {
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}, ctx.Done())
}
