package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// A delivered reminder carries buttons, because the moment a reminder arrives is
// the moment the reader knows what they want to do with it — and it is the only
// moment they have the reminder in front of them. Sending them off to
// `/recurring pause <id>` instead means copying a 16 character ID out of a
// different command's output.

const (
	// actionPath is where the buttons post back to. Relative to
	// /plugins/<plugin id>/, which is how Mattermost resolves plugin actions.
	actionPath = "/api/v1/actions"

	// contextReminderID names the reminder a button belongs to.
	contextReminderID = "reminder_id"
	// contextSnoozeMinutes is how long a snooze button defers a reminder for.
	contextSnoozeMinutes = "snooze_minutes"
)

// snoozeOptions are the offered delays: fifteen minutes for "not right this
// second", an hour for "not now". Both defer only the occurrence that just
// fired, leaving the schedule itself alone.
var snoozeOptions = []struct {
	Label   string
	Minutes int
}{
	{"15 min", 15},
	{"1 hour", 60},
}

// reminderActions builds the buttons attached to a delivered reminder.
func (p *Plugin) reminderActions(r *reminder.Reminder) []*model.PostAction {
	actions := make([]*model.PostAction, 0, len(snoozeOptions)+1)

	for _, option := range snoozeOptions {
		actions = append(actions, &model.PostAction{
			// Mattermost puts this ID straight into the callback path
			// (/api/v4/posts/<id>/actions/<action id>), so it has to be
			// URL-safe. Building it from the label put a space in it and every
			// press 404'd.
			Id:   fmt.Sprintf("snooze_%dm", option.Minutes),
			Type: model.PostActionTypeButton,
			Name: "Snooze " + option.Label,
			Integration: &model.PostActionIntegration{
				URL: actionPath + "/snooze",
				Context: map[string]any{
					contextReminderID:    r.ID,
					contextSnoozeMinutes: option.Minutes,
				},
			},
		})
	}

	actions = append(actions, &model.PostAction{
		Id:    "pause",
		Type:  model.PostActionTypeButton,
		Name:  "Stop this reminder",
		Style: "danger",
		Integration: &model.PostActionIntegration{
			URL: actionPath + "/pause",
			Context: map[string]any{
				contextReminderID: r.ID,
			},
		},
	})

	return actions
}

// decodeAction reads an action request and resolves the reminder it names.
// Anything malformed is answered with a 400 and reported as not handled.
func (p *Plugin) decodeAction(w http.ResponseWriter, req *http.Request) (*model.PostActionIntegrationRequest, *reminder.Reminder, bool) {
	var request model.PostActionIntegrationRequest
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		http.Error(w, "invalid action request", http.StatusBadRequest)
		return nil, nil, false
	}

	reminderID, ok := request.Context[contextReminderID].(string)
	if !ok || reminderID == "" {
		http.Error(w, "action is missing a reminder", http.StatusBadRequest)
		return nil, nil, false
	}

	// The lookup is scoped to the header the server sets, not to anything in
	// the request body, so a forged context naming someone else's reminder
	// finds nothing.
	r, err := p.loadOwnReminder(req.Header.Get("Mattermost-User-ID"), reminderID)
	if err != nil {
		p.respondToAction(w, &request, actionFailureText(err))
		return nil, nil, false
	}

	return &request, r, true
}

// handleSnooze defers a reminder without changing its schedule.
func (p *Plugin) handleSnooze(w http.ResponseWriter, req *http.Request) {
	request, r, ok := p.decodeAction(w, req)
	if !ok {
		return
	}

	minutes, err := contextInt(request.Context, contextSnoozeMinutes)
	if err != nil {
		http.Error(w, "action has an unusable snooze length", http.StatusBadRequest)
		return
	}

	now := time.Now()
	until := now.Add(time.Duration(minutes) * time.Minute)

	// Snoozing moves only this occurrence. The schedule is untouched, so the
	// reminder returns to its usual rhythm after the deferred delivery.
	r.NextRunAt = until.UnixMilli()
	r.Paused = false
	r.FailureCount = 0
	r.UpdatedAt = now.UnixMilli()

	if err := p.kvstore.UpdateReminder(r); err != nil {
		p.client.Log.Error("Failed to snooze a reminder", "user_id", r.UserID, "reminder_id", r.ID, "err", err)
		p.respondToAction(w, request, "Something went wrong. Please try again.")
		return
	}

	p.respondToAction(w, request, "⏰ **"+escapeInline(r.Message)+"**\n_snoozed until "+p.formatRunAt(r)+"_")
}

// handlePause stops a reminder from the delivered message.
func (p *Plugin) handlePause(w http.ResponseWriter, req *http.Request) {
	request, r, ok := p.decodeAction(w, req)
	if !ok {
		return
	}

	now := time.Now()
	r.Paused = true
	r.UpdatedAt = now.UnixMilli()

	if err := p.kvstore.UpdateReminder(r); err != nil {
		p.client.Log.Error("Failed to pause a reminder", "user_id", r.UserID, "reminder_id", r.ID, "err", err)
		p.respondToAction(w, request, "Something went wrong. Please try again.")
		return
	}

	p.respondToAction(w, request,
		"⏰ **"+escapeInline(r.Message)+"**\n_stopped · resume it with_ `/recurring resume "+r.ID+"`")
}

// loadOwnReminder fetches a reminder belonging to the given user.
func (p *Plugin) loadOwnReminder(userID, reminderID string) (*reminder.Reminder, error) {
	if userID == "" {
		return nil, errors.New("unauthenticated")
	}

	r, err := p.kvstore.GetReminder(userID, reminderID)
	if err != nil {
		if errors.Is(err, reminder.ErrNotFound) {
			return nil, err
		}
		p.client.Log.Error("Failed to load a reminder for an action", "user_id", userID, "reminder_id", reminderID, "err", err)
		return nil, err
	}

	return r, nil
}

// actionFailureText explains a failed action to the person who pressed it.
func actionFailureText(err error) string {
	if errors.Is(err, reminder.ErrNotFound) {
		return "That reminder is gone — it looks like it was already deleted."
	}

	return "Something went wrong. Please try again."
}

// respondToAction rewrites the delivered message in place, so the buttons are
// replaced by the outcome of pressing them rather than staying live.
func (p *Plugin) respondToAction(w http.ResponseWriter, request *model.PostActionIntegrationRequest, message string) {
	response := &model.PostActionIntegrationResponse{
		Update: &model.Post{
			Id:      request.PostId,
			Message: message,
			Props: map[string]any{
				// Dropping the attachments removes the buttons.
				"attachments": nil,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		p.client.Log.Error("Failed to write an action response", "err", err)
	}
}

// contextInt reads a number out of an action context. Values arrive as JSON, so
// an integer sent by the plugin comes back as a float64.
func contextInt(context map[string]any, key string) (int, error) {
	switch v := context[key].(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, errors.Wrapf(err, "context %q is not a whole number", key)
		}
		return int(n), nil
	default:
		return 0, errors.Errorf("context %q is missing or not a number", key)
	}
}
