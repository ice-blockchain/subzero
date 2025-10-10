#!/usr/bin/python3
# SPDX-License-Identifier: ice License 1.0

import os
import argparse
import json
from urllib.parse import urlparse
from time import sleep

import psycopg
import yaml
from flask import Flask, jsonify, redirect, render_template_string, url_for, Response

HOME_PAGE_TEMPLATE = """
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ dashboard_name }} | Server Selection</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background-color: #f4f4f9; color: #333; margin: 0; padding: 2rem; }
        .container { max-width: 800px; margin: 0 auto; background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        h1 { color: #1a1a1a; border-bottom: 2px solid #eee; padding-bottom: 0.5rem; }
        .server-list { list-style: none; padding: 0; }
        .server-list li { margin: 1rem 0; }
        .server-list a { text-decoration: none; color: #007bff; font-weight: bold; font-size: 1.2rem; padding: 0.75rem; display: block; border-radius: 5px; background-color: #f8f9fa; border: 1px solid #dee2e6; transition: background-color 0.2s, border-color 0.2s; }
        .server-list a:hover { background-color: #e9ecef; border-color: #ced4da; }
        .server-list .hostname { color: #6c757d; font-size: 0.9rem; font-weight: normal; margin-left: 1rem; }
        .total-link { border-color: #007bff; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Select a Database Server</h1>
        <ul class="server-list">
            {% if 'total' in servers %}
                <li>
                    <a href="{{ url_for('select_server', server_alias='total') }}" class="total-link">
                        Total
                        <span class="hostname">({{ servers.total.display_name }})</span>
                    </a>
                </li>
            {% endif %}
            {% for alias, details in servers.items() if alias != 'total' %}
                <li>
                    <a href="{{ url_for('select_server', server_alias=alias) }}">
                        {{ alias }}
                        <span class="hostname">({{ details.display_name }})</span>
                    </a>
                </li>
            {% endfor %}
        </ul>
    </div>
</body>
</html>
"""

STATS_PAGE_TEMPLATE = """
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ dashboard_name }} | Stats for {{ server_alias }}</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background-color: #f4f4f9; color: #333; margin: 0; padding: 2rem; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .header { display: flex; justify-content: space-between; align-items: center; border-bottom: 2px solid #eee; padding-bottom: 0.5rem; margin-bottom: 1.5rem; }
        h1, h2 { color: #1a1a1a; margin: 0; }
        h2 { margin-bottom: 1rem; text-align: center; }
        #refresh-btn { padding: 0.6rem 1.2rem; font-size: 1rem; cursor: pointer; background-color: #007bff; color: white; border: none; border-radius: 5px; transition: background-color 0.2s; }
        #refresh-btn:hover { background-color: #0056b3; }
        #refresh-btn:disabled { background-color: #6c757d; cursor: not-allowed; }
        .stats-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1.5rem; margin-bottom: 2rem; }
        .stat-card { background-color: #f8f9fa; padding: 1.5rem; border-radius: 8px; text-align: center; border: 1px solid #dee2e6; }
        .stat-card h3 { margin: 0 0 0.5rem 0; color: #495057; font-size: 1.1rem; }
        .stat-card .value { font-size: 2.5rem; font-weight: bold; color: #007bff; }
        .chart-area { margin-top: 2rem; }
        .chart-container { position: relative; width: 100%; max-height: 600px; overflow-y: auto; border: 1px solid #dee2e6; padding: 1rem; border-radius: 8px; margin-bottom: 2rem; }
        .centered-chart-wrapper { max-width: 900px; margin: 0 auto 2rem auto; }
        .progress-bar-container { width: 100%; background-color: #e9ecef; border-radius: 5px; overflow: hidden; margin: 2rem 0; }
        .progress-bar { width: 0%; height: 20px; background-color: #007bff; text-align: center; line-height: 20px; color: white; transition: width 0.4s ease; }
        .progress-text { text-align: center; color: #6c757d; font-size: 0.9rem; margin-top: -1.5rem; margin-bottom: 2rem; }
        .error, .loading { text-align: center; font-size: 1.2rem; padding: 2rem; color: #dc3545; background-color: #f8d7da; border-radius: 5px; }
        .loading { color: #004085; background-color: #cce5ff; }
        .empty-chart-message { text-align: center; font-size: 1rem; padding: 3rem 1rem; color: #6c757d; background-color: #f8f9fa; border: 1px solid #dee2e6; border-radius: 8px; }
        a.back-link { color: #6c757d; text-decoration: none; }
        a.back-link:hover { text-decoration: underline; }
        .footer-note { text-align: center; margin-top: 1rem; font-style: italic; color: #6c757d; font-size: 0.9rem; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Stats for: {{ server_alias }}</h1>
            <button id="refresh-btn">Refresh</button>
        </div>
        <a href="{{ url_for('home') }}" class="back-link">&larr; Back to Server List</a>
        <div id="content-area">
            <div class="loading">Loading data...</div>
        </div>
        <footer class="footer-note">
            * Topics with fewer than {{ min_posts }} posts are not shown.
        </footer>
    </div>
    <script>
        const serverAlias = "{{ server_alias }}";
        const contentArea = document.getElementById('content-area');
        const refreshButton = document.getElementById('refresh-btn');
        let topicsChart = null;
        let postsPerDayChart = null;

        function renderContent(data) {
            contentArea.innerHTML = '';
            if (data.error) {
                contentArea.innerHTML = `<div class="error"><strong>Error:</strong> ${data.error}</div>`;
                return;
            }
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
            contentArea.appendChild(statsGrid);
            const chartArea = document.createElement('div');
            chartArea.className = 'chart-area';
            contentArea.appendChild(chartArea);
            const dailyChartWrapper = document.createElement('div');
            dailyChartWrapper.className = 'centered-chart-wrapper';
            const dailyChartTitle = document.createElement('h2');
            dailyChartTitle.innerText = 'Posts in Last 7 Days';
            dailyChartWrapper.appendChild(dailyChartTitle);
            const dailyData = data.posts_per_day;
            const isDailyDataEmpty = dailyData.every(item => item.post_count === 0);
            if (isDailyDataEmpty) {
                const emptyMessage = document.createElement('div');
                emptyMessage.className = 'empty-chart-message';
                emptyMessage.innerText = 'No posts found in the last 7 days.';
                dailyChartWrapper.appendChild(emptyMessage);
            } else {
                const dailyChartContainer = document.createElement('div');
                dailyChartContainer.className = 'chart-container';
                dailyChartContainer.style.maxHeight = '400px';
                dailyChartContainer.style.marginBottom = '0';
                const dailyCanvas = document.createElement('canvas');
                dailyCanvas.id = 'postsPerDayChart';
                dailyChartContainer.appendChild(dailyCanvas);
                dailyChartWrapper.appendChild(dailyChartContainer);
                if (postsPerDayChart) { postsPerDayChart.destroy(); }
                const dailyCtx = dailyCanvas.getContext('2d');
                postsPerDayChart = new Chart(dailyCtx, {
                    type: 'bar',
                    data: {
                        labels: dailyData.map(item => item.post_date),
                        datasets: [{ label: 'Posts', data: dailyData.map(item => item.post_count), backgroundColor: 'rgba(40, 167, 69, 0.6)', borderColor: 'rgba(40, 167, 69, 1)', borderWidth: 1 }]
                    },
                    options: { scales: { y: { beginAtZero: true } }, plugins: { legend: { display: false } } }
                });
            }
            chartArea.appendChild(dailyChartWrapper);
            const topicChartTitle = document.createElement('h2');
            topicChartTitle.innerText = 'Posts per Topic';
            chartArea.appendChild(topicChartTitle);
            const topicChartContainer = document.createElement('div');
            topicChartContainer.className = 'chart-container';
            const topicCanvas = document.createElement('canvas');
            topicCanvas.id = 'topicsChart';
            topicChartContainer.appendChild(topicCanvas);
            chartArea.appendChild(topicChartContainer);
            if (topicsChart) { topicsChart.destroy(); }
            const topicCtx = topicCanvas.getContext('2d');
            const topicData = data.posts_per_topic;
            topicsChart = new Chart(topicCtx, {
                type: 'bar',
                data: {
                    labels: topicData.map(item => item.topic),
                    datasets: [{ label: 'Number of Posts', data: topicData.map(item => item.post_count), backgroundColor: 'rgba(0, 123, 255, 0.6)', borderColor: 'rgba(0, 123, 255, 1)', borderWidth: 1 }]
                },
                options: { indexAxis: 'y', scales: { x: { beginAtZero: true }, y: { ticks: { font: { size: 13 } } } }, plugins: { legend: { display: true } }, maintainAspectRatio: false }
            });
            topicChartContainer.style.height = `${topicData.length * 30 + 100}px`;
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

            if (serverAlias === 'total') {
                setupProgressBar();
                const progressBar = document.getElementById('progress-bar');
                const progressText = document.getElementById('progress-text');
                const eventSource = new EventSource(`/api/stats/total/stream`);

                eventSource.onmessage = function(event) {
                    const data = JSON.parse(event.data);
                    if (data.type === 'progress') {
                        const percentage = (data.current / data.total) * 100;
                        progressBar.style.width = percentage + '%';
                        progressText.innerText = `Fetching server ${data.current} / ${data.total} (${data.server_name || ''})...`;
                    } else if (data.type === 'result') {
                        progressText.innerText = 'Rendering results...';
                        progressBar.style.width = '100%';
                        renderContent(data.data);
                        eventSource.close();
                        refreshButton.disabled = false;
                    }
                };

                eventSource.onerror = function(err) {
                    console.error("EventSource failed:", err);
                    contentArea.innerHTML = `<div class="error"><strong>Error:</strong> Failed to stream data. The connection was lost.</div>`;
                    eventSource.close();
                    refreshButton.disabled = false;
                };

            } else {
                contentArea.innerHTML = '<div class="loading">Fetching data...</div>';
                try {
                    const response = await fetch(`/api/stats/${serverAlias}`);
                    if (!response.ok) { throw new Error(`HTTP error! status: ${response.status}`); }
                    const data = await response.json();
                    renderContent(data);
                } catch (error) {
                    console.error("Failed to fetch data:", error);
                    contentArea.innerHTML = `<div class="error"><strong>Error:</strong> Failed to connect to the server or API. Please check server logs.</div>`;
                } finally {
                    refreshButton.disabled = false;
                }
            }
        }
        refreshButton.addEventListener('click', fetchAndUpdateData);
        document.addEventListener('DOMContentLoaded', fetchAndUpdateData);
    </script>
</body>
</html>
"""

_active_dashboard_name = "Postgres Stats Dashboard"
_active_servers_config = {}
_active_min_posts_per_topic = 20

QUERIES = {
    "total_posts": "SELECT COUNT(id) FROM events WHERE kind IN (30023, 30175) and is_reply=false and expiration is null and deleted=false and hidden=false;",
    "posts_with_topics": "SELECT COUNT(id) FROM events WHERE kind IN (30023, 30175) AND is_reply=false and expiration is null and deleted=false and array_length(array_remove(t_tags, 'unclassified'), 1) > 0 and hidden=false;",
    "posts_without_topic": "SELECT COUNT(id) FROM events WHERE kind IN (30023, 30175) AND is_reply=false and expiration is null and deleted=false and coalesce(array_length(array_remove(t_tags, 'unclassified'), 1), 0) = 0 and hidden=false;",
    "posts_per_topic": "SELECT unnested_topic AS topic, COUNT(id) AS post_count FROM events, unnest(t_tags) AS unnested_topic WHERE kind IN (30023, 30175) and is_reply=false and expiration is null and deleted=false AND unnested_topic <> 'unclassified' and hidden=false GROUP BY unnested_topic HAVING COUNT(id) >= %s ORDER BY post_count DESC, topic ASC;",
    "posts_per_day": """
        WITH date_series AS (
            SELECT generate_series(date_trunc('day', NOW() - interval '6 days'), date_trunc('day', NOW()), '1 day')::date AS day
        ),
        post_counts AS (
            SELECT
                date_trunc('day', to_timestamp(lookup_created_at / 1000000000.0))::date AS day,
                COUNT(id) AS post_count
            FROM events
            WHERE kind IN (30023, 30175)
              AND lookup_created_at >= (extract(epoch from NOW() - interval '7 days') * 1000000000)::bigint
              AND is_reply=false
              AND expiration is null
              AND deleted=false
              AND hidden=false
            GROUP BY 1
        )
        SELECT to_char(ds.day, 'YYYY-MM-DD') AS post_date, COALESCE(pc.post_count, 0) AS post_count
        FROM date_series ds LEFT JOIN post_counts pc ON ds.day = pc.day
        ORDER BY ds.day ASC;
    """
}

def get_db_stats(conn_string, min_posts):
    results = {"total_posts": 0, "posts_with_topics": 0, "posts_without_topic": 0, "posts_per_topic": [], "posts_per_day": [], "error": None}
    try:
        with psycopg.connect(conn_string) as conn:
            with conn.cursor() as cur:
                cur.execute(QUERIES["total_posts"]); results["total_posts"] = cur.fetchone()[0]
                cur.execute(QUERIES["posts_with_topics"]); results["posts_with_topics"] = cur.fetchone()[0]
                cur.execute(QUERIES["posts_without_topic"]); results["posts_without_topic"] = cur.fetchone()[0]
                cur.execute(QUERIES["posts_per_topic"], (min_posts,)); results["posts_per_topic"] = [{"topic": r[0], "post_count": r[1]} for r in cur.fetchall()]
                cur.execute(QUERIES["posts_per_day"]); results["posts_per_day"] = [{"post_date": r[0], "post_count": r[1]} for r in cur.fetchall()]
    except psycopg.Error as e:
        print(f"Database connection or query error: {e}"); results["error"] = f"Could not connect or query database. ({type(e).__name__})"
    except Exception as e:
        print(f"An unexpected error occurred in get_db_stats: {e}"); results["error"] = "An unexpected server-side error occurred."
    return results

def stream_aggregate_stats():
    total_servers = len(_active_servers_config)
    yield f"data: {json.dumps({'type': 'progress', 'current': 0, 'total': total_servers})}\n\n"

    total_results = {"total_posts": 0, "posts_with_topics": 0, "posts_without_topic": 0, "error": None}
    aggregated_topics, aggregated_days = {}, {}
    successful_fetches = 0

    for i, (alias, config) in enumerate(_active_servers_config.items()):
        yield f"data: {json.dumps({'type': 'progress', 'current': i + 1, 'total': total_servers, 'server_name': alias})}\n\n"
        server_stats = get_db_stats(config["conn_string"], min_posts=1)
        if server_stats.get("error"):
            print(f"Warning: Could not fetch stats from {alias}. Error: {server_stats['error']}")
            continue
        successful_fetches += 1
        total_results["total_posts"] += server_stats["total_posts"]
        total_results["posts_with_topics"] += server_stats["posts_with_topics"]
        total_results["posts_without_topic"] += server_stats["posts_without_topic"]
        for item in server_stats["posts_per_topic"]: aggregated_topics[item["topic"]] = aggregated_topics.get(item["topic"], 0) + item["post_count"]
        for item in server_stats["posts_per_day"]: aggregated_days[item["post_date"]] = aggregated_days.get(item["post_date"], 0) + item["post_count"]

    if successful_fetches == 0:
        total_results["error"] = "Failed to fetch data from any of the configured servers."
        yield f"data: {json.dumps({'type': 'result', 'data': total_results})}\n\n"
        return

    filtered_topics = [item for item in [{"topic": t, "post_count": c} for t, c in aggregated_topics.items()] if item['post_count'] >= _active_min_posts_per_topic]
    total_results["posts_per_topic"] = sorted(filtered_topics, key=lambda x: x['post_count'], reverse=True)
    total_results["posts_per_day"] = [{"post_date": d, "post_count": c} for d, c in sorted(aggregated_days.items())]

    yield f"data: {json.dumps({'type': 'result', 'data': total_results})}\n\n"

def configure_app_globals(args):
    global _active_dashboard_name, _active_servers_config, _active_min_posts_per_topic
    name, min_posts, databases = "Postgres Stats Dashboard", 20, []
    if args.config:
        try:
            with open(args.config, 'r') as f:
                config = yaml.safe_load(f)
                name, min_posts, databases = config.get('name', name), config.get('min_posts_per_topic', min_posts), config.get('database', databases)
        except (FileNotFoundError, yaml.YAMLError) as e: print(f"Warning: Could not load config file '{args.config}'. Error: {e}")
    if os.getenv('DASHBOARD_NAME'): name = os.getenv('DASHBOARD_NAME')
    env_min_posts = os.getenv('MIN_POSTS_PER_TOPIC')
    if env_min_posts and env_min_posts.isdigit(): min_posts = int(env_min_posts)
    if os.getenv('DATABASE_URLS'): databases = os.getenv('DATABASE_URLS').split(',')
    if args.name: name = args.name
    if args.min_posts_per_topic: min_posts = args.min_posts_per_topic
    if args.database: databases = args.database
    if not databases: print("Error: No database connections configured."); exit(1)
    _active_dashboard_name, _active_min_posts_per_topic = name, min_posts
    _active_servers_config.clear()
    for i, conn_str in enumerate(databases):
        alias, display_name = f"server-{i+1}", f"Connection {i+1}"
        try: parsed = urlparse(conn_str); display_name = f"{parsed.hostname}:{parsed.port}/{parsed.path[1:]}"
        except Exception: pass
        _active_servers_config[alias] = {"conn_string": conn_str, "display_name": display_name}
    print(f"Configuration loaded successfully.\nDashboard Name: {_active_dashboard_name}")
    print("Configured servers:")
    for alias, details in _active_servers_config.items(): print(f"  - {alias}: {details['display_name']}")
    print(f"Minimum posts per topic for chart: {_active_min_posts_per_topic}")

def create_app(cli_args=None):
    if cli_args:
        configure_app_globals(cli_args)
    else:
        class DummyArgs: pass
        args = DummyArgs()
        args.config, args.name, args.min_posts_per_topic, args.database = os.getenv('CONFIG_FILE_PATH'), None, None, None
        configure_app_globals(args)
    app_instance = Flask(__name__)
    @app_instance.route("/")
    def home():
        servers_to_display = _active_servers_config.copy()
        if len(_active_servers_config) > 1: servers_to_display['total'] = {'display_name': 'Aggregated from all servers'}
        return render_template_string(HOME_PAGE_TEMPLATE, servers=servers_to_display, dashboard_name=_active_dashboard_name)
    @app_instance.route("/select/<server_alias>")
    def select_server(server_alias):
        if server_alias in _active_servers_config or server_alias == 'total': return redirect(url_for("show_stats", server_alias=server_alias))
        return "Server alias not found.", 404
    @app_instance.route("/stats/<server_alias>")
    def show_stats(server_alias):
        is_valid = server_alias in _active_servers_config or (server_alias == 'total' and len(_active_servers_config) > 1)
        if not is_valid: return "Server alias not found.", 404
        return render_template_string(STATS_PAGE_TEMPLATE, server_alias=server_alias, min_posts=_active_min_posts_per_topic, dashboard_name=_active_dashboard_name)
    @app_instance.route("/api/stats/total/stream")
    def stream_stats_data():
        if len(_active_servers_config) <= 1:
            return jsonify({"error": "Total view requires more than one server."}), 400
        response = Response(stream_aggregate_stats(), mimetype='text/event-stream')
        response.headers['X-Accel-Buffering'] = 'no'
        return response
    @app_instance.route("/api/stats/<server_alias>")
    def get_stats_data(server_alias):
        if server_alias in _active_servers_config:
            conn_string = _active_servers_config[server_alias]["conn_string"]
            stats = get_db_stats(conn_string, _active_min_posts_per_topic)
            return jsonify(stats)
        return jsonify({"error": "Server alias not found."}), 404
    return app_instance

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Run a web dashboard for Nostr event statistics.")
    parser.add_argument("--config", help="Path to an optional YAML configuration file.")
    parser.add_argument("--database", action="append", help="PostgreSQL connection string. Overrides config/env.")
    parser.add_argument("--port", type=int, default=9999, help="Port to run the web server on.")
    parser.add_argument("--min-posts-per-topic", type=int, help="Minimum posts for a topic to be shown. Overrides config/env.")
    parser.add_argument("--name", default="Stats", help="Dashboard name. Overrides config/env.")
    args = parser.parse_args()
    application = create_app(cli_args=args)
    print(f"\nDashboard running locally at http://127.0.0.1:{args.port}/ (for dev only!)")
    application.run(host="0.0.0.0", port=args.port)
else:
    application = create_app()
