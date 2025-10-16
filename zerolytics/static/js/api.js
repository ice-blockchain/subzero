/**
 * SPDX-License-Identifier: ice License 1.0
 */

class ZerolyticsAPI {
    constructor() {
        this.baseUrl = '';
    }

    async fetchStats(serverAlias, days = 7) {
        try {
            const response = await fetch(`/api/stats/${serverAlias}?days=${days}`);
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            return await response.json();
        } catch (error) {
            console.error("Failed to fetch data:", error);
            throw error;
        }
    }

    createEventSource(url) {
        return new EventSource(url);
    }

    setupStreamingStats(serverAlias, onProgress, onResult, onError, days = 7) {
        const eventSource = this.createEventSource(`/api/stats/total/stream?days=${days}`);

        eventSource.onmessage = function(event) {
            const data = JSON.parse(event.data);
            if (data.type === 'progress') {
                onProgress(data);
            } else if (data.type === 'result') {
                onResult(data.data);
                eventSource.close();
            }
        };

        eventSource.onerror = function(err) {
            console.error("EventSource failed:", err);
            onError(err);
            eventSource.close();
        };

        return eventSource;
    }
}

window.zerolyticsAPI = new ZerolyticsAPI();
