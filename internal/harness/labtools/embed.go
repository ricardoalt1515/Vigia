package labtools

import "embed"

//go:embed fixtures/cases/*.json fixtures/rules/*.json
var fixtureFS embed.FS
