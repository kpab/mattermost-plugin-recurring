package kvstore

import (
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// Client is the KVStore implementation backed by the plugin KV store. Every
// call to the pluginapi goes through this one type, so the key layout stays in
// this package and callers can be tested against the KVStore interface instead.
type Client struct {
	client *pluginapi.Client
}

// NewKVStore returns a KVStore backed by the given plugin API client.
func NewKVStore(client *pluginapi.Client) KVStore {
	return Client{
		client: client,
	}
}
