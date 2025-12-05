package globalaccelerator

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/test/framework/utils"
)

// verifyAGATrafficFlows verifies traffic reaches backend through GlobalAccelerator endpoints
func verifyAGATrafficFlows(ctx context.Context, f *framework.Framework, stack *ResourceStack, port ...int) error {
	gaARN := stack.GetGlobalAcceleratorARN()
	if err := waitForEndpointsHealthy(ctx, f, gaARN); err != nil {
		return err
	}

	dnsName := stack.GetGlobalAcceleratorDNSName()
	if err := utils.WaitUntilDNSNameAvailable(ctx, dnsName); err != nil {
		return err
	}

	listenerPort := 80
	if len(port) > 0 {
		listenerPort = port[0]
	}

	client := &http.Client{Timeout: 10 * utils.PollIntervalMedium}
	timeoutCtx, cancel := context.WithTimeout(ctx, utils.PollTimeoutShort)
	defer cancel()

	return wait.PollImmediateUntil(utils.PollIntervalMedium, func() (bool, error) {
		resp, err := client.Get(fmt.Sprintf("http://%v:%d/", dnsName, listenerPort))
		if err != nil {
			f.Logger.Info("waiting for traffic to flow", "error", err.Error())
			return false, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			f.Logger.Info("waiting for traffic to flow", "statusCode", resp.StatusCode)
			return false, nil
		}
		return true, nil
	}, timeoutCtx.Done())
}
