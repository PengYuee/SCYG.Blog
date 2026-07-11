package platform

import "embed"

var (
	//go:embed asset.txt
	embedded embed.FS
	singleton = map[string]string{}
)
