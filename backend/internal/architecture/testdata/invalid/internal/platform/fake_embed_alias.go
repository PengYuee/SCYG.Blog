package platform

import emb "example.com/embed"

//go:embed asset.txt
var fakeAlias emb.FS
