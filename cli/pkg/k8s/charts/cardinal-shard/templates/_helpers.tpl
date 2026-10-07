{{/* instanceName returns the Deployment/Service base name for pool index i. */}}
{{- define "cardinal-shard.instanceName" -}}
{{- $shard := index . 0 -}}{{- $i := index . 1 -}}
{{- if eq $i 0 }}{{ $shard }}{{ else }}{{ printf "%s-%d" $shard (add1 $i) }}{{ end -}}
{{- end -}}

{{/* labels shared by every object of one instance. */}}
{{- define "cardinal-shard.labels" -}}
app.kubernetes.io/managed-by: argocd
cardinal.argus.gg/shard-id: {{ index . 0 }}
cardinal.argus.gg/instance: {{ index . 1 }}
{{- end -}}

{{/* pathSegment makes a DNS-label-safe URL segment, same rule as cli/pkg/dnslabel. */}}
{{- define "cardinal-shard.pathSegment" -}}
{{- $s := regexReplaceAll "[^a-z0-9]+" (lower (trim .)) "-" | trimAll "-" -}}
{{- if $s }}{{ $s }}{{ else }}unknown{{ end -}}
{{- end -}}
