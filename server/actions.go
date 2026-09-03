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
	// actionPathPrefix is the plugin-relative part of the callback path.
	actionPathPrefix = "/api/v1/actions"

	// contextReminderID names the reminder a button belongs to.
	contextReminderID = "reminder_id"
	// contextSnoozeMinutes is how long a snooze button defers a reminder for.
	contextSnoozeMinutes = "snooze_minutes"
	// contextEphemeral marks a button that lives on an ephemeral post.
	contextEphemeral = "ephemeral"
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

// actionURL builds the address a button posts back to.
//
// It has to carry the full plugin path from the site root. The field's own
// documentation calls a plugin URL "a relative path", but the server feeds the
// value straight to an HTTP client, so a bare "/api/v1/..." fails with
// "unsupported protocol scheme".
func actionURL(action string) string {
	return "/plugins/" + manifest.Id + actionPathPrefix + "/" + action
}

// listActions builds the buttons shown next to a reminder in /recurring list.
//
// The list needs its own buttons, not just the delivered message's, because the
// mobile app runs no plugin UI at all: a sidebar is invisible there, and a
// 16 character ID printed for copying is unusable on a phone. Buttons are the
// one control surface that works everywhere.
func (p *Plugin) listActions(r *reminder.Reminder) []*model.PostAction {
	toggle := &model.PostAction{
		Id:   "pause",
		Type: model.PostActionTypeButton,
		Name: "Pause",
		Integration: &model.PostActionIntegration{
			URL: actionURL("pause"),
			Context: map[string]any{
				contextReminderID: r.ID,
				contextEphemeral:  true,
			},
		},
	}

	if r.Paused {
		toggle.Id = "resume"
		toggle.Name = "Resume"
		toggle.Integration.URL = actionURL("resume")
	}

	return []*model.PostAction{
		toggle,
		{
			Id:    "delete",
			Type:  model.PostActionTypeButton,
			Name:  "Delete",
			Style: "danger",
			Integration: &model.PostActionIntegration{
				URL: actionURL("delete"),
				Context: map[string]any{
					contextReminderID: r.ID,
					contextEphemeral:  true,
				},
			},
		},
	}
}

// reminderActions builds the buttons attached to a delivered reminder.
func (p *Plugin) reminderActions(r *reminder.Reminder) []*model.PostAction {
	actions := make([]*model.PostAction, 0, len(snoozeOptions)+1)

	for _, option := range snoozeOptions {
		actions = append(actions, &model.PostAction{
			// Mattermost puts this ID straight into its own callback route
			// (/api/v4/posts/<id>/actions/<action id>), which matches on
			// alphanumerics only. A space 404s, and so does an underscore.
			Id:   fmt.Sprintf("snooze%dm", option.Minutes),
			Type: model.PostActionTypeButton,
			Name: "Snooze " + option.Label,
			Integration: &model.PostActionIntegration{
				URL: actionURL("snooze"),
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
			URL: actionURL("pause"),
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

// handlePause stops a reminder.
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
		"⏰ **"+escapeInline(r.Message)+"**\n_paused · resume it with_ `/recurring resume "+r.ID+"`")
}

// handleResume starts a paused reminder again.
func (p *Plugin) handleResume(w http.ResponseWriter, req *http.Request) {
	request, r, ok := p.decodeAction(w, req)
	if !ok {
		return
	}

	now := time.Now()
	r.Paused = false
	r.UpdatedAt = now.UnixMilli()

	// Its old next run is in the past by now, so find the following one.
	r.Advance(now)

	if err := p.kvstore.UpdateReminder(r); err != nil {
		p.client.Log.Error("Failed to resume a reminder", "user_id", r.UserID, "reminder_id", r.ID, "err", err)
		p.respondToAction(w, request, "Something went wrong. Please try again.")
		return
	}

	p.respondToAction(w, request,
		"⏰ **"+escapeInline(r.Message)+"**\n_resumed · next "+p.formatRunAt(r)+"_")
}

// handleDelete removes a reminder.
func (p *Plugin) handleDelete(w http.ResponseWriter, req *http.Request) {
	request, r, ok := p.decodeAction(w, req)
	if !ok {
		return
	}

	if err := p.kvstore.DeleteReminder(r.UserID, r.ID); err != nil {
		p.client.Log.Error("Failed to delete a reminder", "user_id", r.UserID, "reminder_id", r.ID, "err", err)
		p.respondToAction(w, request, "Something went wrong. Please try again.")
		return
	}

	p.respondToAction(w, request, "~~"+escapeInline(r.Message)+"~~ _deleted_")
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

// respondToAction rewrites the post the button sits on, so that pressing it
// leaves the outcome behind rather than a live button.
//
// Which mechanism does that depends on the post. An ephemeral post — the reply
// to /recurring list — exists only in the reader's client, so asking the server
// to update it by ID fails with "Post not found"; those have to go back through
// UpdateEphemeralPost. A delivered reminder is a real post and takes the
// ordinary Update.
func (p *Plugin) respondToAction(w http.ResponseWriter, request *model.PostActionIntegrationRequest, message string) {
	response := &model.PostActionIntegrationResponse{}

	if isEphemeral(request) {
		post := &model.Post{
			Id:        request.PostId,
			ChannelId: request.ChannelId,
			UserId:    request.UserId,
			Message:   message,
		}
		// Dropping the attachments removes the buttons.
		post.AddProp("attachments", nil)

		p.client.Post.UpdateEphemeralPost(request.UserId, post)
	} else {
		response.Update = &model.Post{
			Id:      request.PostId,
			Message: message,
			Props: map[string]any{
				"attachments": nil,
			},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		p.client.Log.Error("Failed to write an action response", "err", err)
	}
}

// isEphemeral reports whether the button sits on an ephemeral post.
func isEphemeral(request *model.PostActionIntegrationRequest) bool {
	ephemeral, ok := request.Context[contextEphemeral].(bool)

	return ok && ephemeral
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
