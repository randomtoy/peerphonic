{{- define "peerphonic.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "peerphonic.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "peerphonic.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "peerphonic.labels" -}}
helm.sh/chart: {{ include "peerphonic.chart" . }}
app.kubernetes.io/name: {{ include "peerphonic.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "peerphonic.selectorLabels" -}}
app.kubernetes.io/name: {{ include "peerphonic.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: backend
{{- end }}

{{- define "peerphonic.webSelectorLabels" -}}
app.kubernetes.io/name: {{ include "peerphonic.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: web
{{- end }}

{{- define "peerphonic.secretName" -}}
{{- default (printf "%s-auth" (include "peerphonic.fullname" .)) .Values.auth.existingSecret }}
{{- end }}

{{- define "peerphonic.dataClaimName" -}}
{{- default (printf "%s-data" (include "peerphonic.fullname" .)) .Values.persistence.existingClaim }}
{{- end }}

