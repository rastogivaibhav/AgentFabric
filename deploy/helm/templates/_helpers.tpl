{{/*
Govagn Helm chart helpers
*/}}

{{/*
Render a fully-qualified image reference.
Usage: {{ include "govagn.image" .Values.collector.image }}

Produces:
  repository:tag               — when digest is empty / unset
  repository:tag@sha256:<...>  — when digest is set (SOC 2 supply-chain pin)
*/}}
{{- define "govagn.image" -}}
{{- $repo := .repository -}}
{{- $tag  := .tag | default "latest" -}}
{{- $dig  := .digest | default "" -}}
{{- if $dig -}}
  {{- printf "%s:%s@%s" $repo $tag $dig -}}
{{- else -}}
  {{- printf "%s:%s" $repo $tag -}}
{{- end -}}
{{- end -}}

{{/*
Common labels applied to every resource.
*/}}
{{- define "govagn.labels" -}}
app.kubernetes.io/managed-by: Helm
app.kubernetes.io/part-of: govagn
{{- end -}}

{{/*
Selector labels for a given component.
Usage: {{ include "govagn.selectorLabels" (dict "component" "collector") }}
*/}}
{{- define "govagn.selectorLabels" -}}
app.kubernetes.io/name: govagn
app.kubernetes.io/component: {{ .component }}
{{- end -}}
