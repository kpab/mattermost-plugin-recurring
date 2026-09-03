// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';
import React from 'react';
import type {Store} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import type {PluginRegistry} from 'types/mattermost-webapp';

import Sidebar from './components/sidebar';

/** ClockIcon is the channel header button that opens the sidebar. */
function ClockIcon() {
    return (
        <i
            className='icon icon-clock-outline'
            aria-hidden='true'
        />
    );
}

export default class Plugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        const {toggleRHSPlugin} = registry.registerRightHandSidebarComponent(
            Sidebar,
            'Recurring Reminders',
        );

        registry.registerChannelHeaderButtonAction(
            <ClockIcon/>,
            () => store.dispatch(toggleRHSPlugin),
            'Recurring Reminders',
            'Recurring Reminders',
        );
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
