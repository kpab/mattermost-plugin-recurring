import manifest from 'manifest';

export type Reminder = {
    id: string;
    message: string;

    /** Rendered server-side, so the sidebar always says what the slash command says. */
    schedule: string;
    next_run: string;
    paused: boolean;
};

type RemindersResponse = {
    reminders: Reminder[];
};

const pluginBase = `/plugins/${manifest.id}/api/v1`;

/** fetchReminders returns the current user's reminders. */
export async function fetchReminders(): Promise<Reminder[]> {
    const response = await fetch(`${pluginBase}/reminders`, {

        // The session cookie is what authenticates the request; the server
        // reads the user from the header it sets for authenticated callers.
        credentials: 'same-origin',
        headers: {'X-Requested-With': 'XMLHttpRequest'},
    });

    if (!response.ok) {
        throw new Error(`failed to load reminders: ${response.status}`);
    }

    const body = await response.json() as RemindersResponse;

    return body.reminders ?? [];
}
