# Zerolytics

A Flask web application to visualize real-time statistics for Nostr events stored across one or more PostgreSQL databases. This dashboard provides insights into post counts, topic distribution, and daily activity, specifically for kinds `30023` and `30175`.

## Features

*   **Multi-Database Support:** Connects to one or more PostgreSQL servers.
*   **Aggregated View:** Provides a "Total" view that combines statistics from all configured servers (if more than one is present).
*   **Configurable Dashboard Name:** Set a custom name that appears as a prefix in the page titles.
*   **Key Statistics:** Displays total posts, posts with meaningful topics, and posts without topics.
*   **Daily Activity Chart:** A bar chart showing the number of posts created per day for the last 7 days. Displays a "No posts" message if all days are empty.
*   **Topic Distribution Chart:** A horizontal bar chart showing the most popular topics (excluding 'unclassified').
*   **Configurable Topic Filtering:** Set a minimum post count for topics to appear on the chart, with a dynamic note reflecting this filter.
*   **Flexible Configuration:** Options are loaded from command-line arguments, environment variables, or an optional YAML configuration file, with clear precedence.

## Prerequisites

### System Dependencies (Ubuntu/Debian)

```bash
sudo apt update
sudo apt install python3-psycopg python3-flask python3-yaml uwsgi uwsgi-plugin-python3 uwsgi-extra uwsgi-plugin-router-access
```

## Configuration

The application's configuration follows a hierarchical precedence (highest to lowest):

1.  **Command-line arguments**
2.  **Environment variables**
3.  **YAML configuration file**
4.  **Hardcoded defaults** (e.g., default port `9999`, default `min_posts_per_topic` `20`, default `name` "Postgres Stats Dashboard")

### Example `config.yaml`

Create a file named `config.yaml` in your project root:

```yaml
# config.yaml
name: Zerolytics
min_posts_per_topic: 15
database:
  - postgresql://user:pass@host1:5432/my_relay_db_a
  - postgresql://user:pass@host2:5432/my_relay_db_b
```

### Configuration Options

*   **`--config <path>` (CLI)** / **`CONFIG_FILE_PATH` (ENV)**: Path to an optional YAML configuration file.
*   **`--name <string>` (CLI)** / **`DASHBOARD_NAME` (ENV)** / `name` (YAML): A custom name for the dashboard, used as a prefix in HTML page titles (e.g., "My Relay Stats | Stats for server-1").
*   **`--port <int>` (CLI, default: `9999`)**: The port number for the Flask development server to listen on. *Note: This is ignored when running via Gunicorn/WSGI in production, where Gunicorn handles the binding port.*
*   **`--min-posts-per-topic <int>` (CLI)** / **`MIN_POSTS_PER_TOPIC` (ENV)** / `min_posts_per_topic` (YAML, default: `20`): Minimum number of posts a topic must have (cumulatively across all aggregated servers) to be displayed in the topic chart.
*   **`--database <conn_string>` (CLI, can be specified multiple times)** / **`DATABASE_URLS` (ENV, comma-separated)** / `database` (YAML, list of strings): PostgreSQL connection string(s) (e.g., `postgresql://user:pass@host:port/dbname`). **At least one database connection must be provided.**

---

## License

This project is licensed under the ice License 1.0.
