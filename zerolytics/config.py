#!/usr/bin/python3
# SPDX-License-Identifier: ice License 1.0

import os
import yaml
from urllib.parse import urlparse


class ConfigLoader:
    """Configuration management for Zerolytics Dashboard"""
    
    def __init__(self):
        self.dashboard_name = "Postgres Stats Dashboard"
        self.servers_config = {}
        self.min_posts_per_topic = 20
    
    def load_from_args(self, args):
        """Load configuration from command line arguments and environment"""
        name, min_posts, databases = self.dashboard_name, self.min_posts_per_topic, []
        
        if args.config:
            try:
                with open(args.config, 'r') as f:
                    config = yaml.safe_load(f)
                    name = config.get('name', name)
                    min_posts = config.get('min_posts_per_topic', min_posts)
                    databases = config.get('database', databases)
            except (FileNotFoundError, yaml.YAMLError) as e:
                print(f"Warning: Could not load config file '{args.config}'. Error: {e}")
        
        if os.getenv('DASHBOARD_NAME'):
            name = os.getenv('DASHBOARD_NAME')
        
        env_min_posts = os.getenv('MIN_POSTS_PER_TOPIC')
        if env_min_posts and env_min_posts.isdigit():
            min_posts = int(env_min_posts)
        
        if os.getenv('DATABASE_URLS'):
            databases = os.getenv('DATABASE_URLS').split(',')
        
        if args.name:
            name = args.name
        if args.min_posts_per_topic:
            min_posts = args.min_posts_per_topic
        if args.database:
            databases = args.database
        
        if not databases:
            print("Error: No database connections configured.")
            exit(1)
        
        self.dashboard_name = name
        self.min_posts_per_topic = min_posts
        self._setup_servers_config(databases)
        
        self._print_config()
    
    def load_from_env(self):
        """Load configuration from environment variables only"""
        class DummyArgs:
            pass
        
        args = DummyArgs()
        args.config = os.getenv('CONFIG_FILE_PATH')
        args.name = None
        args.min_posts_per_topic = None
        args.database = None
        
        self.load_from_args(args)
    
    def _setup_servers_config(self, databases):
        """Setup server configuration from database URLs"""
        self.servers_config.clear()
        
        for i, conn_str in enumerate(databases):
            alias = f"server-{i+1}"
            display_name = f"Connection {i+1}"
            
            try:
                parsed = urlparse(conn_str)
                display_name = f"{parsed.hostname}:{parsed.port}/{parsed.path[1:]}"
            except Exception:
                pass
            
            self.servers_config[alias] = {
                "conn_string": conn_str,
                "display_name": display_name
            }
    
    def _print_config(self):
        """Print current configuration"""
        print(f"Configuration loaded successfully.")
        print(f"Dashboard Name: {self.dashboard_name}")
        print("Configured servers:")
        for alias, details in self.servers_config.items():
            print(f"  - {alias}: {details['display_name']}")
        print(f"Minimum posts per topic for chart: {self.min_posts_per_topic}")
    
    def get_servers_for_display(self):
        """Get servers configuration for display with total option"""
        servers_to_display = self.servers_config.copy()
        if len(self.servers_config) > 1:
            servers_to_display['total'] = {'display_name': 'Aggregated from all servers'}
        return servers_to_display
