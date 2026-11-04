package utils

func GetDeploymentImage(registry string, image string) string {
	return registry + "/" + image
}
