package main

import (
	// The plugin binary is built with CGO_ENABLED=0 and cross-compiled, so it
	// cannot fall back to the host's zoneinfo files. Without the embedded
	// database every time.LoadLocation fails on a server whose image ships no
	// tzdata, and every reminder is silently interpreted as UTC — a 09:00
	// reminder firing at 18:00 for a user in Tokyo, with nothing to show why.
	_ "time/tzdata"

	"github.com/mattermost/mattermost/server/public/plugin"
)

func main() {
	plugin.ClientMain(&Plugin{})
}
