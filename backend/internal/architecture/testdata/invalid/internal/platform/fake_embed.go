package platform

import _ "embed"

//go:embed asset.txt
var fakeEmbedded = map[string]string{}
