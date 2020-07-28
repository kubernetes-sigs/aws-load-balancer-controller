package throttle

// NewDefaultServiceOperationsThrottleConfig returns a ServiceOperationsThrottleConfig with default settings.
func NewDefaultServiceOperationsThrottleConfig() *ServiceOperationsThrottleConfig {
	return &ServiceOperationsThrottleConfig{
		value: map[string][]throttleConfig{
		},
	}
}
