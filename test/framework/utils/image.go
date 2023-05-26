package utils

func GetDeploymentImage(awsRegion string, image string) string {
	awsAccount := DefaultAWSAccount
	if awsRegion == "us-iso-east-1" {
		awsAccount = DCAAccount
	} else if awsRegion == "us-isob-east-1" {
		awsAccount = LCKAccount
	}
	dpImage := awsAccount + "/" + image
	return dpImage
}
