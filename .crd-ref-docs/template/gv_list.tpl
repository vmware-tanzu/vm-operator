{{- define "gvList" -}}
{{- $groupVersions := . -}}
{{- $gv := index $groupVersions 0 -}}

# {{ $gv.Version }}

{{ $gv.Doc }}

---

{{ if $gv.Kinds -}}

## Kinds

{{ range $gv.SortedKinds }}
{{- with $type := $gv.TypeForKind . }}
{{ template "type" . }}
{{- end -}}
{{- end -}}
{{- end }}

## Types

{{- range $gv.SortedTypes }}
{{- if not .GVK }}
{{ template "type" . }}
{{- end }}
{{- end }}

{{- end -}}
