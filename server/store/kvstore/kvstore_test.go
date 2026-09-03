package kvstore

import (
	"bytes"
	"sync"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// fakeAPI implements the handful of KV methods the store actually uses, backed
// by a map. The rest of the plugin API comes from the embedded plugintest.API,
// which panics if anything else is called — which is what we want, since it
// would mean the store grew a dependency this test does not know about.
type fakeAPI struct {
	plugintest.API

	mu    sync.Mutex
	store map[string][]byte
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{store: map[string][]byte{}}
}

func (a *fakeAPI) KVGet(key string) ([]byte, *model.AppError) {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.store[key], nil
}

func (a *fakeAPI) KVSetWithOptions(key string, value []byte, options model.PluginKVSetOptions) (bool, *model.AppError) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if options.Atomic && !bytes.Equal(a.store[key], options.OldValue) {
		return false, nil
	}

	if value == nil {
		delete(a.store, key)
		return true, nil
	}

	a.store[key] = value

	return true, nil
}

func (a *fakeAPI) KVDelete(key string) *model.AppError {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.store, key)

	return nil
}

func (a *fakeAPI) has(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	_, ok := a.store[key]

	return ok
}

// newTestStore returns a store wired to an in-memory KV, plus the fake itself
// so a test can assert on the raw keys.
func newTestStore(t *testing.T) (KVStore, *fakeAPI) {
	t.Helper()

	api := newFakeAPI()

	return NewKVStore(pluginapi.NewClient(api, nil)), api
}
