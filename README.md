# Recurring Reminders for Mattermost

[![Build Status](https://github.com/kpab/mattermost-plugin-recurring/actions/workflows/ci.yml/badge.svg)](https://github.com/kpab/mattermost-plugin-recurring/actions/workflows/ci.yml)

Schedule **recurring** reminders in Mattermost — "every weekday at 9:00", "every Monday at 10:00",
"on the 1st of every month".

Mattermost ships with one-off message reminders, but
[recurring reminders are explicitly unsupported](https://docs.mattermost.com/end-user-guide/collaborate/message-reminders.html).
The community plugin that used to fill that gap
([mattermost-plugin-remind](https://github.com/scottleedavis/mattermost-plugin-remind)) is archived, and the
request to add recurrence to the To Do plugin
([#61](https://github.com/mattermost-community/mattermost-plugin-todo/issues/61)) has been open since 2020.
This plugin fills that gap.

> **Status: early development.** Not yet released. See [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md) for the roadmap.

## Features

- **Recurring reminders** — daily, weekdays, weekly on given days, monthly
- **One-off reminders** — "in 30 minutes", "tomorrow at 9"
- **Natural language input** in English and Japanese, plus a structured picker in the right-hand sidebar
- **List, edit, complete and snooze** your reminders from the RHS
- **Timezone-aware** — reminders fire in *your* timezone, not the server's

## Installation

Not yet published to the Mattermost Marketplace. For now, build from source:

```sh
make dist
```

Then upload `dist/com.github.kpab.recurring-<version>.tar.gz` via
**System Console → Plugins → Plugin Management**.
Plugin uploads must be enabled on the server (`PluginSettings.EnableUploads`).

## Development

Requires Go 1.25+ and the Node version in [`.nvmrc`](.nvmrc) (`nvm i` to install it).

```sh
make dist          # build the plugin bundle
make test          # run server and webapp tests
make check-style   # lint
make deploy        # build and install directly to a running server
make watch         # deploy, then redeploy the webapp on change
make logs          # tail the plugin's server logs
```

### Deploying to a server

`make deploy` needs credentials for the target server. Using a
[personal access token](https://docs.mattermost.com/developer/personal-access-tokens.html) of a system admin:

```sh
export MM_SERVICESETTINGS_SITEURL=https://your-server.example.com
export MM_ADMIN_TOKEN=<token>
make deploy
```

Username and password (`MM_ADMIN_USERNAME` / `MM_ADMIN_PASSWORD`) also work, but not when the server
enforces MFA — use a token there.

If the server runs locally with [local mode](https://docs.mattermost.com/administration/mmctl-cli-tool.html#local-mode)
enabled, `make deploy` uses the Unix socket instead and needs no credentials
(override the path with `MM_LOCALSOCKETPATH`).

Set `MM_DEBUG=1` on any `make` invocation to build unminified JavaScript.

### Versioning

The version is derived at compile time from git: a tagged commit yields that tag (leading `v` stripped),
otherwise the nearest tag is combined with the short hash, e.g. `1.3.1+d06e53e1`.
Use `make patch` / `make minor` / `make major` (and the `-rc` variants) to cut a release.

## Contributing

Bug reports and feature requests are welcome in
[issues](https://github.com/kpab/mattermost-plugin-recurring/issues).

## License

[Apache-2.0](LICENSE)
