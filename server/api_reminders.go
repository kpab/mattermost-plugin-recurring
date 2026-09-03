package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kpab/mattermost-plugin-recurring/server/reminder"
)

// reminderView is a reminder as the sidebar needs it.
//
// Every string arrives already rendered, label included, so that the wording,
// the date formatting and the translations live in one place rather than being
// reimplemented in TypeScript and drifting from what the slash command says.
// The sidebar's "Next:" label stayed English for exactly that reason.
type reminderView struct {
	ID       string `json:"id"`
	Message  string `json:"message"`
	Schedule string `json:"schedule"`
	// NextRun is the whole line, label included: "次回 今日 18:00 JST".
	NextRun string `json:"next_run"`
	Paused  bool   `json:"paused"`
}

// remindersResponse is the payload of GET /api/v1/reminders.
type remindersResponse struct {
	Reminders []reminderView `json:"reminders"`
}

// handleGetReminders lists the calling user's reminders.
func (p *Plugin) handleGetReminders(w http.ResponseWriter, req *http.Request) {
	userID := req.Header.Get("Mattermost-User-ID")

	reminders, err := p.kvstore.GetReminders(userID)
	if err != nil {
		p.client.Log.Error("Failed to list reminders for the sidebar", "user_id", userID, "err", err)
		http.Error(w, "failed to load reminders", http.StatusInternalServerError)
		return
	}

	lang := p.langFor(userID)

	response := remindersResponse{Reminders: make([]reminderView, 0, len(reminders))}
	for _, r := range reminders {
		response.Reminders = append(response.Reminders, p.viewOf(r, lang))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		p.client.Log.Error("Failed to write the reminder list", "user_id", userID, "err", err)
	}
}

// viewOf renders a reminder for the sidebar.
func (p *Plugin) viewOf(r *reminder.Reminder, lang reminder.Lang) reminderView {
	// A paused reminder has no next run to label — "next paused" is nonsense.
	next := p.formatRunAt(r, lang)
	if !r.Paused && r.NextRunAt != 0 {
		next = fmt.Sprintf(msg(lang, "next"), next)
	}

	return reminderView{
		ID:       r.ID,
		Message:  r.Message,
		Schedule: r.Schedule.Describe(lang),
		NextRun:  next,
		Paused:   r.Paused,
	}
}
