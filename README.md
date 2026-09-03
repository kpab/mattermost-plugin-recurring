# Recurring Reminders for Mattermost

[![Build Status](https://github.com/kpab/mattermost-plugin-recurring/actions/workflows/ci.yml/badge.svg)](https://github.com/kpab/mattermost-plugin-recurring/actions/workflows/ci.yml)

> Reminders that repeat — every weekday at 9:00, every Monday, the 1st of the month.

Mattermost's built-in message reminder fires once. This one keeps going.

- **Daily, weekdays, weekly, or monthly**, written in plain English or Japanese
- **Replies in your language** — English or Japanese, following your Mattermost setting
- **Delivered by direct message** in your own timezone, and correct across
  daylight-saving changes
- **Snooze or stop** a reminder from the message itself, without retyping anything
- **Pause and resume** a reminder without losing it
- **No configuration and no external service** — reminders live in your own
  server's KV store and are delivered by a bot that runs there

> **Status: early.** Released and in use, but not yet in the Mattermost Marketplace.

## Usage

```
/recurring daily 9:00 stand-up
/recurring weekdays 18:00 log off
/recurring every monday at 10:00 weekly report
/recurring monthly on the 1st 9:00 expenses
```

Times can be `10:00`, `9am`, `6:30pm`, or `at 9`.

Japanese input works too, and replies follow your Mattermost language setting:

```
/recurring 毎朝9時 ストレッチ
/recurring 毎週月曜 10:00 週次報告
```

Managing them:

```
/recurring list                  # show your reminders
/recurring pause <id>            # stop one without deleting it
/recurring resume <id>           # start it again
/recurring delete <id>           # remove one
/recurring help
```

Reminders arrive as a direct message from the plugin's bot, in your own
timezone. There is nothing to configure — the plugin has no settings.

## Installation

Take the `.tar.gz` from the
[latest release](https://github.com/kpab/mattermost-plugin-recurring/releases)
and upload it in **System Console → Plugins → Plugin Management**.
Plugin uploads must be enabled on the server (`PluginSettings.EnableUploads`).

Requires Mattermost 9.5 or later. There is nothing to configure once it is
enabled.

To build it yourself instead:

```sh
make dist
```

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

## Roadmap

Not built yet:

- Reminders posted to a channel, rather than only to yourself
- A right-hand sidebar to browse, create and edit reminders
- One-off reminders ("in 30 minutes") — for now, use Mattermost's built-in
  **Remind me about this** on a message
- Several weekdays in one reminder ("every Monday and Thursday")

## Why not Slack, Google Calendar, or Todoist?

Because a lot of Mattermost runs where those cannot: air-gapped networks,
regulated industries, organisations that self-host by policy. This plugin adds
no external service and no extra account — reminders are stored in your own
server's KV store and delivered by a bot that lives there. And the reminder
arrives where the work already is.

If you want task lists, use the
[To Do plugin](https://github.com/mattermost-community/mattermost-plugin-todo).
This one is for things that repeat on a clock.

## Why this exists

Mattermost has had one-off message reminders since v7.10, but
[recurring reminders are explicitly unsupported](https://docs.mattermost.com/end-user-guide/collaborate/message-reminders.html).
The community plugin that used to fill that gap
([mattermost-plugin-remind](https://github.com/scottleedavis/mattermost-plugin-remind))
is archived, and the request to add recurrence to the To Do plugin
([#61](https://github.com/mattermost-community/mattermost-plugin-todo/issues/61))
has been open since 2020.

## License

[Apache-2.0](LICENSE)
