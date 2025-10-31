package routeutils

type LiteralTargetGroupConfig struct {
	// GW API limits names to 253 characters, while a TG ARN might be 256, so just using the name.
	Name string
}
