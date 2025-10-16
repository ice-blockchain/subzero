#!/usr/bin/python3
# SPDX-License-Identifier: ice License 1.0

from flask import Flask, jsonify, redirect, render_template, url_for, Response, request
from database import DatabaseManager


class ZerolyticsRoutes:
    """Route handlers for Zerolytics Dashboard"""
    
    def __init__(self, config_loader):
        self.config = config_loader
    
    def register_routes(self, app):
        """Register all routes with the Flask app"""
        
        @app.route("/")
        def home():
            servers_to_display = self.config.get_servers_for_display()
            return render_template(
                'home.html',
                servers=servers_to_display,
                dashboard_name=self.config.dashboard_name
            )
        
        @app.route("/select/<server_alias>")
        def select_server(server_alias):
            if server_alias in self.config.servers_config or server_alias == 'total':
                return redirect(url_for("show_stats", server_alias=server_alias))
            return "Server alias not found.", 404
        
        @app.route("/stats/<server_alias>")
        def show_stats(server_alias):
            is_valid = (
                server_alias in self.config.servers_config or 
                (server_alias == 'total' and len(self.config.servers_config) > 1)
            )
            if not is_valid:
                return "Server alias not found.", 404
            
            return render_template(
                'stats.html',
                server_alias=server_alias,
                min_posts=self.config.min_posts_per_topic,
                dashboard_name=self.config.dashboard_name
            )
        
        @app.route("/api/stats/total/stream")
        def stream_stats_data():
            if len(self.config.servers_config) <= 1:
                return jsonify({"error": "Total view requires more than one server."}), 400
            
            days = request.args.get('days', 7, type=int)
            response = Response(
                DatabaseManager.stream_aggregate_stats(
                    self.config.servers_config,
                    self.config.min_posts_per_topic,
                    days=days
                ),
                mimetype='text/event-stream'
            )
            response.headers['X-Accel-Buffering'] = 'no'
            response.headers['Cache-Control'] = 'no-cache'
            response.headers['Access-Control-Allow-Origin'] = '*'
            response.headers['Access-Control-Allow-Methods'] = 'GET'
            response.headers['Access-Control-Allow-Headers'] = 'Content-Type'
            return response
        
        @app.route("/api/stats/<server_alias>")
        def get_stats_data(server_alias):
            if server_alias in self.config.servers_config:
                conn_string = self.config.servers_config[server_alias]["conn_string"]
                days = request.args.get('days', 7, type=int)
                stats = DatabaseManager.get_db_stats(conn_string, self.config.min_posts_per_topic, days=days)
                return jsonify(stats)
            return jsonify({"error": "Server alias not found."}), 404
        
        @app.route("/test")
        def test_eventsource():
            return app.send_static_file('../test_eventsource.html')
