{{- define "type" -}}
{{- $type := . -}}
{{- if markdownShouldRenderType $type -}}

### {{ $type.Name }}

{{ if $type.IsAlias }}_Underlying type:_ `{{ markdownRenderTypeLink $type.UnderlyingType  }}`{{ end }}

{{ $type.Doc }}

{{ if $type.References -}}
_Appears in:_
{{- range $type.SortedReferences }}
- {{ markdownRenderTypeLink . }}
{{- end }}
{{- end }}

{{ if $type.Members -}}
| Field | Description |
| --- | --- |
{{ if $type.GVK -}}
| `apiVersion` _string_ | `{{ $type.GVK.Group }}/{{ $type.GVK.Version }}`
| `kind` _string_ | `{{ $type.GVK.Kind }}`
{{ end -}}

{{ range $type.Members -}}
{{- /* 
  Force the VirtualMachineClass.Spec.ConfigSpec field to be emitted
  as a json.RawMessage, otherwise it will be emitted as a slice of
  integers.
*/ -}}
{{ if and (eq .Name "configSpec") (and (and .Type (eq .Type.Kind 6)) (and .Type.UnderlyingType (eq .Type.UnderlyingType.Kind 2)) ) -}}
| `{{ .Name  }}` _[json.RawMessage](https://pkg.go.dev/encoding/json#RawMessage)_ | {{ template "type_members" . }} |
{{ else -}}
| `{{ .Name  }}` _{{ markdownRenderType .Type }}_ | {{ template "type_members" . }} |
{{ end -}}
{{ end -}}

{{ end -}}

{{- end -}}
{{- end -}}