/**
 * SPDX-License-Identifier: ice License 1.0
 */


function initializeStatsPage(serverAlias) {
    const contentArea = document.getElementById('content-area');
    const refreshButton = document.getElementById('refresh-btn');
    const periodSelect = document.getElementById('period-select');

    function renderContent(data) {
        contentArea.innerHTML = '';
        
        if (data.error) {
            contentArea.innerHTML = `<div class="error"><strong>Error:</strong> ${data.error}</div>`;
            return;
        }

        const days = parseInt(periodSelect.value);

        const statsGrid = window.chartManager.renderStatsGrid(data);
        contentArea.appendChild(statsGrid);

        const activityTitle = document.createElement('h2');
        activityTitle.innerText = `User Activity Statistics (Total)`;
        activityTitle.style.marginTop = '2rem';
        contentArea.appendChild(activityTitle);

        const activityGrid = window.chartManager.renderUserActivityGrid(data);
        
        if (serverAlias !== 'total') {
            const userCountCard = window.chartManager.renderSingleServerUserCount(data);
            activityGrid.appendChild(userCountCard);
        }
        
        contentArea.appendChild(activityGrid);

        const chartArea = document.createElement('div');
        chartArea.className = 'chart-area';
        contentArea.appendChild(chartArea);

        const dailyChartWrapper = document.createElement('div');
        dailyChartWrapper.className = 'centered-chart-wrapper';
        
        const dailyChartTitle = document.createElement('h2');
        dailyChartTitle.innerText = `Posts in Last ${days} Days`;
        dailyChartWrapper.appendChild(dailyChartTitle);

        const dailyData = data.posts_per_day;
        const isDailyDataEmpty = dailyData.every(item => item.post_count === 0);
        
        if (isDailyDataEmpty) {
            const emptyMessage = window.chartManager.renderEmptyChart(`No posts found in the last ${days} days.`);
            dailyChartWrapper.appendChild(emptyMessage);
        } else {
            const dailyChartContainer = document.createElement('div');
            dailyChartContainer.className = 'chart-container';
            dailyChartContainer.style.maxHeight = '400px';
            dailyChartContainer.style.marginBottom = '0';
            
            window.chartManager.createDailyChart(dailyChartContainer, dailyData);
            dailyChartWrapper.appendChild(dailyChartContainer);
        }
        
        chartArea.appendChild(dailyChartWrapper);

        const timeSeriesWrapper = document.createElement('div');
        timeSeriesWrapper.className = 'time-series-wrapper';

        const timeSeriesTitle = document.createElement('h2');
        timeSeriesTitle.innerText = `Activity Timeline (Last ${days} Days)`;
        timeSeriesWrapper.appendChild(timeSeriesTitle);

        const timeSeriesGrid = document.createElement('div');
        timeSeriesGrid.className = 'time-series-grid';

        const activities = [
            { key: 'reactions_per_day', title: 'Reactions', color: 'rgba(255, 99, 132, 0.6)' },
            { key: 'messages_per_day', title: 'Messages', color: 'rgba(54, 162, 235, 0.6)' },
            { key: 'reposts_per_day', title: 'Reposts', color: 'rgba(255, 205, 86, 0.6)' },
            { key: 'comments_per_day', title: 'Comments', color: 'rgba(75, 192, 192, 0.6)' },
            { key: 'articles_per_day', title: 'Articles', color: 'rgba(153, 102, 255, 0.6)' },
            { key: 'stories_per_day', title: 'Stories', color: 'rgba(255, 159, 64, 0.6)', note: 'Last 2 days only' },
            { key: 'videos_per_day', title: 'Videos', color: 'rgba(255, 99, 255, 0.6)' }
        ];

        activities.forEach(activity => {
            const activityData = data[activity.key] || [];
            
            const chartContainer = document.createElement('div');
            chartContainer.className = 'time-series-chart';
            
            const chartTitle = document.createElement('h3');
            chartTitle.innerText = activity.title;
            chartContainer.appendChild(chartTitle);
            
            if (activity.note) {
                const chartNote = document.createElement('div');
                chartNote.className = 'chart-note';
                chartNote.innerText = activity.note;
                chartContainer.appendChild(chartNote);
            }
            
            const chartCanvas = document.createElement('div');
            chartCanvas.className = 'chart-canvas';
            chartContainer.appendChild(chartCanvas);
            
            window.chartManager.createActivityTimeSeriesChart(chartCanvas, activityData, activity.title, activity.color);
            timeSeriesGrid.appendChild(chartContainer);
        });

        timeSeriesWrapper.appendChild(timeSeriesGrid);
        chartArea.appendChild(timeSeriesWrapper);

        const messagesWrapper = document.createElement('div');
        messagesWrapper.className = 'centered-chart-wrapper';
        
        const messagesTitle = document.createElement('h2');
        messagesTitle.innerText = 'Messages by Wrapped Kind';
        messagesWrapper.appendChild(messagesTitle);

        const messagesContainer = document.createElement('div');
        messagesContainer.className = 'chart-container';
        messagesContainer.style.maxHeight = '400px';
        
        if (data.messages_by_wrapped_kind && data.messages_by_wrapped_kind.length > 0) {
            window.chartManager.createMessagesBreakdownChart(messagesContainer, data.messages_by_wrapped_kind);
        } else {
            const emptyMessage = window.chartManager.renderEmptyChart('No message data available.');
            messagesContainer.appendChild(emptyMessage);
        }
        messagesWrapper.appendChild(messagesContainer);
        chartArea.appendChild(messagesWrapper);

        if (serverAlias === 'total') {
            const profileWrapper = document.createElement('div');
            profileWrapper.className = 'centered-chart-wrapper';
            
            const profileTitle = document.createElement('h2');
            profileTitle.innerText = 'User Distribution by Database';
            profileWrapper.appendChild(profileTitle);

            const profileContainer = document.createElement('div');
            profileContainer.className = 'table-container';
            
            if (data.profile_updates_by_database && data.profile_updates_by_database.length > 0) {
                window.chartManager.createProfileDistributionTable(profileContainer, data.profile_updates_by_database);
            } else {
                const emptyMessage = window.chartManager.renderEmptyChart('No user distribution data available.');
                profileContainer.appendChild(emptyMessage);
            }
            profileWrapper.appendChild(profileContainer);
            chartArea.appendChild(profileWrapper);
        }

        const topicChartTitle = document.createElement('h2');
        topicChartTitle.innerText = 'Posts per Topic';
        chartArea.appendChild(topicChartTitle);

        const topicChartContainer = document.createElement('div');
        topicChartContainer.className = 'chart-container';
        
        const topicData = data.posts_per_topic;
        window.chartManager.createTopicChart(topicChartContainer, topicData);
        chartArea.appendChild(topicChartContainer);
    }

    function setupProgressBar() {
        contentArea.innerHTML = `
            <div id="progress-container">
                <div class="progress-bar-container">
                    <div id="progress-bar" class="progress-bar"></div>
                </div>
                <div id="progress-text" class="progress-text">Initializing...</div>
            </div>
        `;
    }

    async function fetchAndUpdateData() {
        refreshButton.disabled = true;
        const days = parseInt(periodSelect.value);

        if (serverAlias === 'total') {
            setupProgressBar();
            const progressBar = document.getElementById('progress-bar');
            const progressText = document.getElementById('progress-text');

            window.zerolyticsAPI.setupStreamingStats(
                serverAlias,
                (data) => {
                    const percentage = (data.current / data.total) * 100;
                    progressBar.style.width = percentage + '%';
                    progressText.innerText = `Fetching server ${data.current} / ${data.total} (${data.server_name || ''})...`;
                },
                (data) => {
                    progressText.innerText = 'Rendering results...';
                    progressBar.style.width = '100%';
                    renderContent(data);
                    refreshButton.disabled = false;
                },
                (err) => {
                    contentArea.innerHTML = `<div class="error"><strong>Error:</strong> Failed to stream data. The connection was lost.</div>`;
                    refreshButton.disabled = false;
                },
                days
            );
        } else {
            contentArea.innerHTML = '<div class="loading">Fetching data...</div>';
            try {
                const data = await window.zerolyticsAPI.fetchStats(serverAlias, days);
                renderContent(data);
            } catch (error) {
                contentArea.innerHTML = `<div class="error"><strong>Error:</strong> Failed to connect to the server or API. Please check server logs.</div>`;
            } finally {
                refreshButton.disabled = false;
            }
        }
    }

    refreshButton.addEventListener('click', fetchAndUpdateData);
    periodSelect.addEventListener('change', fetchAndUpdateData);
    
    fetchAndUpdateData();
}
