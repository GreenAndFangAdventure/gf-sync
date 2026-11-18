// Package brand embeds the guild emblem of 綠皮與獠牙冒險事務所.
//
// The Go code in this file is MIT licensed like the rest of GF Sync, but the
// emblem image itself is NOT: it is the guild's mark, all rights reserved.
// See README.md in this folder.
package brand

import _ "embed"

// EmblemPNG is guild-emblem-source.png (485×397, transparent background):
// a wooden ring, a dark green shield banner and a yellow fanged face.
//
//go:embed guild-emblem-source.png
var EmblemPNG []byte
