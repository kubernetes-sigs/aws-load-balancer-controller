{{- define "aws-load-balancer-controller.vpcIdArg" -}}
{{- if .Values.vpcId }}
- --aws-vpc-id={{ .Values.vpcId }}
{{- else if and .Values.aws .Values.aws.vpcID }}
- --aws-vpc-id={{ .Values.aws.vpcID }}
{{- end }}
{{- end -}}

{{- define "aws-load-balancer-controller.regionArg" -}}
{{- if .Values.region }}
- --aws-region={{ .Values.region }}
{{- else if and .Values.aws .Values.aws.region }}
- --aws-region={{ .Values.aws.region }}
{{- end }}
{{- end -}}

{{- define "aws-load-balancer-controller.iamPodAnnotation" -}}
{{- if .Values.clusterID }}
iam.amazonaws.com/role: {{ printf "gs-%s-ALBController-Role" .Values.clusterID | quote }}
{{- end }}
{{- end -}}

{{/*
Set Giant Swarm serviceAccountAnnotations.
*/}}
{{- define "giantswarm.serviceAccountAnnotations" -}}
{{- if and (eq .Values.provider "aws") (or (eq .Values.region "cn-north-1") (eq .Values.region "cn-northwest-1")) (eq .Values.aws.irsa "true") (not (hasKey .Values.serviceAccount.annotations "eks.amazonaws.com/role-arn")) }}
{{- $_ := set .Values.serviceAccount.annotations "eks.amazonaws.com/role-arn" (tpl "arn:aws-cn:iam::{{ .Values.aws.accountID }}:role/gs-{{ .Values.clusterID }}-ALBController-Role" .) }}
{{- else if and (eq .Values.provider "aws") (eq .Values.aws.irsa "true") (not (hasKey .Values.serviceAccount.annotations "eks.amazonaws.com/role-arn")) }}
{{- $_ := set .Values.serviceAccount.annotations "eks.amazonaws.com/role-arn" (tpl "arn:aws:iam::{{ .Values.aws.accountID }}:role/gs-{{ .Values.clusterID }}-ALBController-Role" .) }}
{{- else if and (eq .Values.provider "capa") (not (hasKey .Values.serviceAccount.annotations "eks.amazonaws.com/role-arn")) }}
{{- $_ := set .Values.serviceAccount.annotations "eks.amazonaws.com/role-arn" (tpl "{{ .Values.clusterID }}-ALBController-Role" .) }}
{{- end }}
{{- end -}}
