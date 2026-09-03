import React, {useCallback, useEffect, useState} from 'react';

import {fetchReminders, type Reminder} from '../client';

// Styles are inline objects rather than a stylesheet: the sidebar renders
// inside the host app, and a plugin's global CSS can only ever fight with it.
// Colours come from Mattermost's own CSS variables so the panel follows the
// user's theme instead of pinning its own.
const styles = {
    container: {
        padding: '16px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
    },
    empty: {
        color: 'var(--center-channel-color-64)',
        lineHeight: 1.6,
    },
    code: {
        background: 'var(--center-channel-color-08)',
        borderRadius: '4px',
        padding: '2px 6px',
        fontFamily: 'monospace',
        fontSize: '12px',
    },
    card: {
        border: '1px solid var(--center-channel-color-16)',
        borderRadius: '8px',
        padding: '12px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    message: {
        fontWeight: 600,

        // A reminder can be a whole sentence, and the sidebar is narrow.
        overflowWrap: 'anywhere' as const,
    },
    meta: {
        fontSize: '12px',
        color: 'var(--center-channel-color-64)',
    },
    pausedBadge: {
        fontSize: '11px',
        textTransform: 'uppercase' as const,
        letterSpacing: '0.04em',
        color: 'var(--center-channel-color-56)',
    },
    error: {
        color: 'var(--error-text)',
    },
    retry: {
        alignSelf: 'flex-start' as const,
    },
};

type LoadState =
    | {status: 'loading'}
    | {status: 'ready'; reminders: Reminder[]}
    | {status: 'failed'};

export default function Sidebar() {
    const [state, setState] = useState<LoadState>({status: 'loading'});

    const load = useCallback(async () => {
        setState({status: 'loading'});
        try {
            setState({status: 'ready', reminders: await fetchReminders()});
        } catch {
            setState({status: 'failed'});
        }
    }, []);

    useEffect(() => {
        load();
    }, [load]);

    if (state.status === 'loading') {
        return <div style={styles.container}>{'Loading…'}</div>;
    }

    if (state.status === 'failed') {
        return (
            <div style={styles.container}>
                <div style={styles.error}>{'Could not load your reminders.'}</div>
                <button
                    className='btn btn-tertiary'
                    style={styles.retry}
                    onClick={load}
                >
                    {'Try again'}
                </button>
            </div>
        );
    }

    if (state.reminders.length === 0) {
        return (
            <div style={styles.container}>
                <div style={styles.empty}>
                    {'No reminders yet. Create one from any channel:'}
                    <div style={{marginTop: '8px'}}>
                        <span style={styles.code}>{'/recurring daily 9:00 stand-up'}</span>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div style={styles.container}>
            {state.reminders.map((reminder) => (
                <div
                    key={reminder.id}
                    style={styles.card}
                >
                    <div style={styles.message}>{reminder.message}</div>
                    <div style={styles.meta}>{reminder.schedule}</div>
                    {reminder.paused ? (
                        <div style={styles.pausedBadge}>{'Paused'}</div>
                    ) : (
                        <div style={styles.meta}>{reminder.next_run}</div>
                    )}
                </div>
            ))}
        </div>
    );
}
