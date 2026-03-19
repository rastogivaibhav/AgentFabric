{{/*
AgentFabric Helm chart helpers
*/}}

{{/*
Render a fully-qualified image reference.
Usage: {{ include "agentfabric.image" .Values.collector.image }}

Produces:
  repository:tag               — when digest is empty / unset
  repository:tag@sha256:<...>  — when digest is set (SOC 2 supply-chain pin)
*/}}
{{- define "agentfabric.image" -}}
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
{{- define "agentfabric.labels" -}}
app.kubernetes.io/managed-by: Helm
app.kubernetes.io/part-of: agentfabric
{{- end -}}

{{/*
Selector labels for a given component.
Usage: {{ include "agentfabric.selectorLabels" (dict "component" "collector") }}
*/}}
{{- define "agentfabric.selectorLabels" -}}
app.kubernetes.io/name: agentfabric
app.kubernetes.io/component: {{ .component }}
{{- end -}}
