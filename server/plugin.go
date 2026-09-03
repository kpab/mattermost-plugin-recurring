package main

import (
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"

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

	// scheduler queues each reminder's next firing.
	scheduler *cluster.JobOnceScheduler

	// botUserID is the account reminders are delivered from.
	botUserID string

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
		Username:    "recurring",
		DisplayName: "Recurring Reminders",
		Description: "Delivers your recurring reminders.",
	})
	if err != nil {
		return errors.Wrap(err, "failed to ensure the reminder bot")
	}
	p.botUserID = botUserID

	if err := p.startScheduler(); err != nil {
		return err
	}

	return nil
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
