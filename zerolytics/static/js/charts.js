/**
 * SPDX-License-Identifier: ice License 1.0
 */


class ChartManager {
    constructor() {
        this.topicsChart = null;
        this.postsPerDayChart = null;
    }

    destroyCharts() {
        if (this.topicsChart) {
            this.topicsChart.destroy();
            this.topicsChart = null;
        }
        if (this.postsPerDayChart) {
            this.postsPerDayChart.destroy();
            this.postsPerDayChart = null;
        }
    }

    createDailyChart(container, dailyData) {
        const wrapper = document.createElement('div');
        wrapper.className = 'chart-wrapper';
        
        const canvas = document.createElement('canvas');
        canvas.id = 'postsPerDayChart';
        wrapper.appendChild(canvas);
        container.appendChild(wrapper);

        const ctx = canvas.getContext('2d');
        this.postsPerDayChart = new Chart(ctx, {
            type: 'bar',
            data: {
                labels: dailyData.map(item => item.post_date),
                datasets: [{
                    label: 'Posts',
                    data: dailyData.map(item => item.post_count),
                    backgroundColor: 'rgba(40, 167, 69, 0.6)',
                    borderColor: 'rgba(40, 167, 69, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: {
                    intersect: false
                },
                scales: {
                    y: { 
                        beginAtZero: true,
                        grid: {
                            display: true
                        }
                    },
                    x: {
                        grid: {
                            display: false
                        }
                    }
                },
                plugins: {
                    legend: { display: false }
                },
                layout: {
                    padding: {
                        top: 20,
                        bottom: 20,
                        left: 20,
                        right: 20
                    }
                }
            }
        });
    }

    createTopicChart(container, topicData) {
        const wrapper = document.createElement('div');
        wrapper.className = 'chart-wrapper';
        wrapper.style.height = `${Math.max(400, topicData.length * 40 + 100)}px`;
        
        const canvas = document.createElement('canvas');
        canvas.id = 'topicsChart';
        wrapper.appendChild(canvas);
        container.appendChild(wrapper);

        const ctx = canvas.getContext('2d');
        this.topicsChart = new Chart(ctx, {
            type: 'bar',
            data: {
                labels: topicData.map(item => item.topic),
                datasets: [{
                    label: 'Number of Posts',
                    data: topicData.map(item => item.post_count),
                    backgroundColor: 'rgba(0, 123, 255, 0.6)',
                    borderColor: 'rgba(0, 123, 255, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                indexAxis: 'y',
                scales: {
                    x: { beginAtZero: true },
                    y: { ticks: { font: { size: 13 } } }
                },
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: true }
                },
                layout: {
                    padding: {
                        top: 20,
                        bottom: 20,
                        left: 20,
                        right: 20
                    }
                }
            }
        });

    }

    renderStatsGrid(data) {
        const statsGrid = document.createElement('div');
        statsGrid.className = 'stats-grid';
        statsGrid.innerHTML = `
            <div class="stat-card">
                <h3>Total Posts</h3>
                <div class="value">${data.total_posts.toLocaleString()}</div>
            </div>
            <div class="stat-card">
                <h3>Posts with Topics</h3>
                <div class="value">${data.posts_with_topics.toLocaleString()}</div>
            </div>
            <div class="stat-card">
                <h3>Posts without Topic</h3>
                <div class="value">${data.posts_without_topic.toLocaleString()}</div>
            </div>
        `;
        return statsGrid;
    }

    renderUserActivityGrid(data) {
        const activityGrid = document.createElement('div');
        activityGrid.className = 'activity-grid';
        activityGrid.innerHTML = `
            <div class="stat-card reactions">
                <h3>👍 Reactions</h3>
                <div class="value">${data.reactions.toLocaleString()}</div>
            </div>
            <div class="stat-card messages">
                <h3>💬 Messages</h3>
                <div class="value">${data.messages.toLocaleString()}</div>
            </div>
            <div class="stat-card reposts">
                <h3>🔄 Reposts</h3>
                <div class="value">${data.reposts.toLocaleString()}</div>
            </div>
            <div class="stat-card comments">
                <h3>💭 Comments</h3>
                <div class="value">${data.comments.toLocaleString()}</div>
            </div>
            <div class="stat-card articles">
                <h3>📄 Articles</h3>
                <div class="value">${data.articles.toLocaleString()}</div>
            </div>
            <div class="stat-card stories">
                <h3>⏰ Stories</h3>
                <div class="value">${data.stories.toLocaleString()}</div>
                <div class="note">Last 2 days only</div>
            </div>
            <div class="stat-card videos">
                <h3>🎥 Videos</h3>
                <div class="value">${data.videos.toLocaleString()}</div>
            </div>
            <div class="stat-card following">
                <h3>👥 Following Actions</h3>
                <div class="value">${data.following_actions.toLocaleString()}</div>
            </div>
        `;
        return activityGrid;
    }

    createActivityTimeSeriesChart(container, data, title, color = 'rgba(54, 162, 235, 0.6)') {
        const canvas = document.createElement('canvas');
        container.appendChild(canvas);

        const dateKey = data.length > 0 && data[0].video_date ? 'video_date' : 'event_date';
        const countKey = data.length > 0 && data[0].video_count !== undefined ? 'video_count' : 'event_count';

        const ctx = canvas.getContext('2d');
        return new Chart(ctx, {
            type: 'line',
            data: {
                labels: data.map(item => item[dateKey]),
                datasets: [{
                    label: title,
                    data: data.map(item => item[countKey]),
                    backgroundColor: color,
                    borderColor: color.replace('0.6', '1'),
                    borderWidth: 2,
                    fill: true,
                    tension: 0.1
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    y: { beginAtZero: true }
                },
                plugins: {
                    legend: { display: true }
                }
            }
        });
    }

    createMessagesBreakdownChart(container, data) {
        if (!data || data.length === 0) {
            const emptyMessage = this.renderEmptyChart('No message breakdown data available.');
            container.appendChild(emptyMessage);
            return null;
        }

        const wrapper = document.createElement('div');
        wrapper.className = 'chart-wrapper';
        
        const canvas = document.createElement('canvas');
        wrapper.appendChild(canvas);
        container.appendChild(wrapper);

        const ctx = canvas.getContext('2d');
        return new Chart(ctx, {
            type: 'doughnut',
            data: {
                labels: data.map(item => `Kind ${item.wrapped_kind}`),
                datasets: [{
                    data: data.map(item => item.message_count),
                    backgroundColor: [
                        'rgba(255, 99, 132, 0.6)',
                        'rgba(54, 162, 235, 0.6)',
                        'rgba(255, 205, 86, 0.6)',
                        'rgba(75, 192, 192, 0.6)',
                        'rgba(153, 102, 255, 0.6)'
                    ]
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { 
                        position: 'bottom',
                        labels: {
                            boxWidth: 12,
                            padding: 15
                        }
                    }
                },
                layout: {
                    padding: {
                        top: 20,
                        bottom: 20,
                        left: 20,
                        right: 20
                    }
                }
            }
        });
    }

    createProfileDistributionTable(container, data) {
        if (!data || data.length === 0) {
            const emptyMessage = this.renderEmptyChart('No user distribution data available.');
            container.appendChild(emptyMessage);
            return;
        }

        const table = document.createElement('table');
        table.className = 'distribution-table';
        
        const thead = document.createElement('thead');
        thead.innerHTML = `
            <tr>
                <th>Database</th>
                <th>Users Count</th>
                <th>Percentage</th>
            </tr>
        `;
        table.appendChild(thead);
        
        const total = data.reduce((sum, item) => sum + item.profile_count, 0);
        
        const tbody = document.createElement('tbody');
        data.forEach(item => {
            const percentage = total > 0 ? ((item.profile_count / total) * 100).toFixed(1) : 0;
            const row = document.createElement('tr');
            row.innerHTML = `
                <td class="db-name">${item.database_name}</td>
                <td class="user-count">${item.profile_count.toLocaleString()}</td>
                <td class="percentage">${percentage}%</td>
            `;
            tbody.appendChild(row);
        });
        
        const totalRow = document.createElement('tr');
        totalRow.className = 'total-row';
        totalRow.innerHTML = `
            <td class="db-name"><strong>Total</strong></td>
            <td class="user-count"><strong>${total.toLocaleString()}</strong></td>
            <td class="percentage"><strong>100.0%</strong></td>
        `;
        tbody.appendChild(totalRow);
        
        table.appendChild(tbody);
        container.appendChild(table);
    }

    renderSingleServerUserCount(data) {
        const userCount = data.profile_updates_by_database ? 
            data.profile_updates_by_database.reduce((sum, item) => sum + item.profile_count, 0) : 0;
        
        const userCountCard = document.createElement('div');
        userCountCard.className = 'stat-card users-count';
        userCountCard.innerHTML = `
            <h3>👥 Total Users</h3>
            <div class="value">${userCount.toLocaleString()}</div>
        `;
        return userCountCard;
    }

    renderEmptyChart(message) {
        const emptyMessage = document.createElement('div');
        emptyMessage.className = 'empty-chart-message';
        emptyMessage.innerText = message;
        return emptyMessage;
    }
}

window.chartManager = new ChartManager();
