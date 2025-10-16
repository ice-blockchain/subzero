#!/usr/bin/python3
# SPDX-License-Identifier: ice License 1.0

import argparse
from flask import Flask
from config import ConfigLoader
from routes import ZerolyticsRoutes


def create_app(cli_args=None):
    """Create and configure the Flask application"""
    config_loader = ConfigLoader()
    if cli_args:
        config_loader.load_from_args(cli_args)
    else:
        config_loader.load_from_env()
    
    app = Flask(__name__)
    
    routes = ZerolyticsRoutes(config_loader)
    routes.register_routes(app)
    
    return app


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
