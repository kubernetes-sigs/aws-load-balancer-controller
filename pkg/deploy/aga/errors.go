package aga

import "fmt"

// Error constants
const (
	// ModelBuildFailed is the error code when the model building process fails
	ModelBuildFailed = "ModelBuildFailed"

	// DeploymentFailed is the error code when stack deployment fails
	DeploymentFailed = "DeploymentFailed"

	// Status reason constants
	EndpointLoadFailed = "EndpointLoadFailed"
)

// AcceleratorNotDisabledError is returned when an accelerator is not ready for deletion
type AcceleratorNotDisabledError struct {
	Message string
}

func (e *AcceleratorNotDisabledError) Error() string {
	return fmt.Sprintf("%s", e.Message)
}
