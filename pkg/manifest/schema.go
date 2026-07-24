package manifest

import _ "embed"

// SchemaJSON is the JSON Schema (draft-07) for a CargoShip manifest, embedded
// from schema.json. It is the machine-checkable form of the manifest spec in
// docs/reference/format/manifest.md, and is drift-checked against the Go structs
// in types.go by the schema tests (#274). Tooling authors can use it to validate
// manifests in any language without running CargoShip.
//
//go:embed schema.json
var SchemaJSON []byte
