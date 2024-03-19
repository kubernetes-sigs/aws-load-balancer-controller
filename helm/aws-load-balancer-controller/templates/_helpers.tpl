{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "aws-load-balancer-controller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "aws-load-balancer-controller.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "aws-load-balancer-controller.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Chart name prefix for resource names
Strip the "-controller" suffix from the default .Chart.Name if the nameOverride is not specified.
This enables using a shorter name for the resources, for example aws-load-balancer-webhook.
*/}}
{{- define "aws-load-balancer-controller.namePrefix" -}}
{{- $defaultNamePrefix := .Chart.Name | trimSuffix "-controller" -}}
{{- default $defaultNamePrefix .Values.nameOverride | trunc 42 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "aws-load-balancer-controller.labels" -}}
{{- if eq (default "helm" .Values.creator) "helm" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ include "aws-load-balancer-controller.chart" . }}
{{- end }}
{{ include "aws-load-balancer-controller.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- if .Values.additionalLabels }}
{{ toYaml .Values.additionalLabels }}
{{- end -}}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "aws-load-balancer-controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aws-load-balancer-controller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "aws-load-balancer-controller.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "aws-load-balancer-controller.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the webhook service
*/}}
{{- define "aws-load-balancer-controller.webhookService" -}}
{{- printf "%s-webhook-service" (include "aws-load-balancer-controller.namePrefix" .) -}}
{{- end -}}

{{/*
Create the name of the webhook cert secret
*/}}
{{- define "aws-load-balancer-controller.webhookCertSecret" -}}
{{- printf "%s-tls" (include "aws-load-balancer-controller.namePrefix" .) -}}
{{- end -}}

{{/*
Generate certificates for webhook
*/}}
{{- define "aws-load-balancer-controller.webhookCerts" -}}
{{- $serviceName := (include "aws-load-balancer-controller.webhookService" .) -}}
{{- $secretName := (include "aws-load-balancer-controller.webhookCertSecret" .) -}}
{{- $secret := lookup "v1" "Secret" .Release.Namespace $secretName -}}
{{- if (and .Values.webhookTLS.caCert .Values.webhookTLS.cert .Values.webhookTLS.key) -}}
caCert: {{ .Values.webhookTLS.caCert | b64enc }}
clientCert: {{ .Values.webhookTLS.cert | b64enc }}
clientKey: {{ .Values.webhookTLS.key | b64enc }}
{{- else if and .Values.keepTLSSecret $secret -}}
caCert: {{ index $secret.data "ca.crt" }}
clientCert: {{ index $secret.data "tls.crt" }}
clientKey: {{ index $secret.data "tls.key" }}
{{- else -}}
{{- $altNames := list (printf "%s.%s" $serviceName .Release.Namespace) (printf "%s.%s.svc" $serviceName .Release.Namespace) (printf "%s.%s.svc.%s" $serviceName .Release.Namespace .Values.cluster.dnsDomain) -}}
{{- $ca := genCA "aws-load-balancer-controller-ca" 3650 -}}
{{- $cert := genSignedCert (include "aws-load-balancer-controller.fullname" .) nil $altNames 3650 $ca -}}
caCert: {{ $ca.Cert | b64enc }}
clientCert: {{ $cert.Cert | b64enc }}
clientKey: {{ $cert.Key | b64enc }}
{{- end -}}
{{- end -}}

{{/*
Convert map to comma separated key=value string
*/}}
{{- define "aws-load-balancer-controller.convertMapToCsv" -}}
{{- range $key, $value := . -}} {{ $key }}={{ $value }}, {{- end -}}
{{- end -}}

{{/*
Create the name of the ingressClassParams
*/}}
{{- define "aws-load-balancer-controller.ingressClassParamsName" -}}
{{ default .Values.ingressClass .Values.ingressClassParams.name }}
{{- end -}}
