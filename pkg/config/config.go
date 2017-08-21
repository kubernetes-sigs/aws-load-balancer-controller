package config

import "time"

// Config contains the ALB Ingress Controller configuration
type Config struct {
	ClusterName     string
	AWSDebug        bool
	DisableRoute53  bool
	ALBSyncInterval time.Duration
}
