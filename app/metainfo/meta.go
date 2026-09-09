// Package metainfo holds build metadata stamped into the binary by the linker
// at release time (see .goreleaser.yaml). Outside a release build the values
// keep their development defaults. It holds linker-stamped vars only and is
// excluded from the coverage gate.
package metainfo

import "time"

func init() {
	if BuildTime == "" {
		BuildTime = time.Now().Format(time.RFC3339)
	}
}

var Version = "dev-build"
var BuildTime = ""
var ShaVer = "undefined"
