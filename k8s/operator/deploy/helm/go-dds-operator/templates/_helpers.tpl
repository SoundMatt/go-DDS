{{/*
Chart name and fully qualified app name, following the standard Helm
starter-chart convention.
*/}}
{{- define "go-dds-operator.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "go-dds-operator.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "go-dds-operator.labels" -}}
app.kubernetes.io/name: {{ include "go-dds-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "go-dds-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "go-dds-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "go-dds-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "go-dds-operator.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
The webhook Service's in-cluster DNS name — the Common Name / SAN the
self-signed certificate must cover.
*/}}
{{- define "go-dds-operator.webhookServiceDNS" -}}
{{- printf "%s-webhook.%s.svc" (include "go-dds-operator.fullname" .) .Release.Namespace -}}
{{- end -}}
