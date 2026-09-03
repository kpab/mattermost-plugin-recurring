package main

import (
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
	"github.com/kpab/mattermost-plugin-recurring/server/store/kvstore"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// kvstore is the client used to read/write KV records for this plugin.
	kvstore kvstore.KVStore

	// client is the Mattermost server API client.
	client *pluginapi.Client

	// router is the HTTP router for handling API requests.
	router *mux.Router

	// backgroundJob runs the periodic reminder delivery sweep.
	backgroundJob *cluster.Job

	// botUserID is the account reminders are delivered from.
	botUserID string

	// send delivers one reminder. It points at sendReminder in production and
	// is swapped out in tests so the delivery sweep can be exercised without a
	// live server.
	send func(r *reminder.Reminder) error

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)

	p.kvstore = kvstore.NewKVStore(p.client)

	p.router = p.initRouter()

	if err := p.registerCommand(); err != nil {
		return err
	}

	botUserID, err := p.client.Bot.EnsureBot(&model.Bot{
		// "recurring" alone reads as an adjective with its noun missing when it
		// turns up in a DM list.
		Username:    "recurring-reminders",
		DisplayName: "Recurring Reminders",
		Description: "Delivers your repeating reminders. Type /recurring to set one up, or /recurring list to see them.",
	})
	if err != nil {
		return errors.Wrap(err, "failed to ensure the reminder bot")
	}
	p.botUserID = botUserID

	p.send = p.sendReminder

	if err := p.startScheduler(); err != nil {
		return err
	}

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	if p.backgroundJob != nil {
		if err := p.backgroundJob.Close(); err != nil {
			p.API.LogError("Failed to stop reminder delivery", "err", err)
		}
	}

	return nil
}

// UserHasBeenDeactivated drops a deactivated user's reminders. Without this
// their reminders would be swept and delivered on every tick forever, failing
// each time.
func (p *Plugin) UserHasBeenDeactivated(_ *plugin.Context, user *model.User) {
	if err := p.kvstore.DeleteAllReminders(user.Id); err != nil {
		p.client.Log.Error("Failed to remove a deactivated user's reminders", "user_id", user.Id, "err", err)
	}
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
