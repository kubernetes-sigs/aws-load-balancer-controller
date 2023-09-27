# Create Endpoint on exisitng Endpointgroup

In order to create an endpoint for the ingress-group, the user
needs to specify two annotations:

`alb.ingress.kubernetes.io/ga-epg-arn: arn:aws:globalaccelerator::12345678912:accelerator/d60128f1-4134-4e03-bed9-edd00f77b3e6/listener/a309af4a/endpoint-group/ed7bf648f700`
`alb.ingress.kubernetes.io/ga-ep-create: "true"`

This second annotation exists because of the fact that endpoints don't support tags
and with the current stateless logic it is not possible to identify the correct
endpoint for deletion. This means that:
*Deletion is only supported by setting `ga-ep-create: "false"`*
