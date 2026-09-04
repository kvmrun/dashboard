package cert

import "embed"

//go:embed CA.crt client.crt client.key
var EmbedStore embed.FS
