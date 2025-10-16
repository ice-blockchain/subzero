#!/usr/bin/python3
# SPDX-License-Identifier: ice License 1.0

import json
import psycopg


class DatabaseManager:
    """Database operations and statistics calculation"""
    
    @staticmethod
    def get_daily_queries():
        return {
            "reactions_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                reaction_counts AS (
                    SELECT 
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS event_count
                    FROM events
                    WHERE kind = 7 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS event_date,
                    COALESCE(rc.event_count, 0) AS event_count
                FROM date_series ds LEFT JOIN reaction_counts rc ON ds.day = rc.day
                ORDER BY ds.day ASC;
            """,
            
            "messages_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                message_counts AS (
                    SELECT 
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS event_count
                    FROM events
                    WHERE kind = 1059 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS event_date,
                    COALESCE(mc.event_count, 0) AS event_count
                FROM date_series ds LEFT JOIN message_counts mc ON ds.day = mc.day
                ORDER BY ds.day ASC;
            """,
            
            "reposts_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                repost_counts AS (
                    SELECT 
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS event_count
                    FROM events
                    WHERE kind IN (6, 16) AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS event_date,
                    COALESCE(rc.event_count, 0) AS event_count
                FROM date_series ds LEFT JOIN repost_counts rc ON ds.day = rc.day
                ORDER BY ds.day ASC;
            """,
            
            "comments_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                comment_counts AS (
                    SELECT 
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS event_count
                    FROM events
                    WHERE kind IN (1, 30175) AND NOT has_ephemeral_attestation AND is_reply = true AND NOT deleted AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS event_date,
                    COALESCE(cc.event_count, 0) AS event_count
                FROM date_series ds LEFT JOIN comment_counts cc ON ds.day = cc.day
                ORDER BY ds.day ASC;
            """,
            
            "articles_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                article_counts AS (
                    SELECT 
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS event_count
                    FROM events
                    WHERE kind = 30023 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS event_date,
                    COALESCE(ac.event_count, 0) AS event_count
                FROM date_series ds LEFT JOIN article_counts ac ON ds.day = ac.day
                ORDER BY ds.day ASC;
            """,
            
            "stories_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - interval '1 days'), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                story_counts AS (
                    SELECT 
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS event_count
                    FROM events e
                    WHERE expiration IS NOT NULL AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - interval '2 days') * 1000000000
                        AND (
                            kind IN (1, 6, 30175) OR 
                            (kind = 16 AND EXISTS (
                                SELECT 1 FROM event_tags et 
                                WHERE et.event_id = e.id AND et.event_tag_key = 'k' AND et.event_tag_value1 = '30175'
                            ))
                        )
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS event_date,
                    COALESCE(sc.event_count, 0) AS event_count
                FROM date_series ds LEFT JOIN story_counts sc ON ds.day = sc.day
                ORDER BY ds.day ASC;
            """,
            
            "profile_updates_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                profile_counts AS (
                    SELECT 
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS event_count
                    FROM events
                    WHERE kind = 0 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS event_date,
                    COALESCE(pc.event_count, 0) AS event_count
                FROM date_series ds LEFT JOIN profile_counts pc ON ds.day = pc.day
                ORDER BY ds.day ASC;
            """,
            
            "posts_per_day": """
                WITH date_series AS (
                    SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                ),
                post_counts AS (
                    SELECT
                        DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                        COUNT(*) AS post_count
                    FROM events
                    WHERE kind IN (30023, 30175)
                        AND NOT has_ephemeral_attestation
                        AND is_reply = false
                        AND expiration IS NULL
                        AND NOT deleted
                        AND NOT hidden
                        AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                    GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                )
                SELECT 
                    ds.day AS post_date,
                    COALESCE(pc.post_count, 0) AS post_count
                FROM date_series ds LEFT JOIN post_counts pc ON ds.day = pc.day
                ORDER BY ds.day ASC;
            """,
                "videos_per_day": """
                    WITH date_series AS (
                        SELECT generate_series(date_trunc('day', NOW() - (%s || ' days')::interval), date_trunc('day', NOW()), '1 day')::date AS day
                    ),
                    video_counts AS (
                        SELECT
                            DATE(to_timestamp(lookup_created_at::double precision / 1000000000)) AS day,
                            COUNT(DISTINCT e.id) AS video_count
                        FROM events e
                        WHERE e.kind IN (1, 30175)
                            AND e.has_videos = true
                            AND NOT e.has_ephemeral_attestation
                            AND NOT e.deleted
                            AND NOT e.hidden
                            AND lookup_created_at >= EXTRACT(epoch FROM NOW() - (%s || ' days')::interval) * 1000000000
                        GROUP BY DATE(to_timestamp(lookup_created_at::double precision / 1000000000))
                    )
                    SELECT 
                        ds.day AS video_date,
                        COALESCE(vc.video_count, 0) AS video_count
                    FROM date_series ds LEFT JOIN video_counts vc ON ds.day = vc.day
                    ORDER BY ds.day ASC;
                """
        }
    
    QUERIES = {
        "total_posts": "SELECT COUNT(id) FROM events WHERE kind IN (30023, 30175) AND is_reply=false AND expiration IS NULL AND NOT deleted AND NOT hidden;",
        "posts_with_topics": "SELECT COUNT(id) FROM events WHERE kind IN (30023, 30175) AND is_reply=false AND expiration IS NULL AND NOT deleted AND NOT hidden AND array_length(array_remove(t_tags, 'unclassified'), 1) > 0;",
        "posts_without_topic": "SELECT COUNT(id) FROM events WHERE kind IN (30023, 30175) AND is_reply=false AND expiration IS NULL AND NOT deleted AND NOT hidden AND coalesce(array_length(array_remove(t_tags, 'unclassified'), 1), 0) = 0;",
        "posts_per_topic": "SELECT unnested_topic AS topic, COUNT(id) AS post_count FROM events, unnest(t_tags) AS unnested_topic WHERE kind IN (30023, 30175) AND is_reply=false AND expiration IS NULL AND NOT deleted AND NOT hidden AND unnested_topic <> 'unclassified' GROUP BY unnested_topic HAVING COUNT(id) >= %s ORDER BY post_count DESC, topic ASC;",
        
        "reactions": "SELECT COUNT(*) FROM events WHERE kind = 7 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden;",
        "messages": "SELECT COUNT(*) FROM events WHERE kind = 1059 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden;",
        "messages_by_wrapped_kind": """
            SELECT 
                et.event_tag_value1 AS wrapped_kind,
                COUNT(*) AS message_count
            FROM events e
            JOIN event_tags et ON e.id = et.event_id
            WHERE e.kind = 1059 
                AND et.event_tag_key = 'k'
                AND NOT e.has_ephemeral_attestation 
                AND NOT e.deleted 
                AND NOT e.hidden
                AND et.event_tag_value1 IN ('1', '4', '7', '9735', '1984', '1985', '9734')  -- Valid wrapped kinds for encrypted messages
            GROUP BY et.event_tag_value1
            ORDER BY message_count DESC;
        """,
        "reposts": "SELECT COUNT(*) FROM events WHERE kind IN (6, 16) AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden;",
        "comments": "SELECT COUNT(*) FROM events WHERE kind IN (1, 30175) AND NOT has_ephemeral_attestation AND is_reply = true AND NOT deleted AND NOT hidden;",
        "articles": "SELECT COUNT(*) FROM events WHERE kind = 30023 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden;",
        
        "stories": """
            SELECT COUNT(*) FROM events e 
            WHERE expiration IS NOT NULL AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden
                AND lookup_created_at >= EXTRACT(epoch FROM NOW() - interval '2 days') * 1000000000
                AND (
                    kind IN (1, 6, 30175) OR 
                    (kind = 16 AND EXISTS (
                        SELECT 1 FROM event_tags et 
                        WHERE et.event_id = e.id AND et.event_tag_key = 'k' AND et.event_tag_value1 = '30175'
                    ))
                );
        """,
        
        "videos": """
            SELECT COUNT(DISTINCT e.id) 
            FROM events e
            WHERE e.kind IN (1, 30175) 
              AND e.has_videos = true
              AND NOT e.has_ephemeral_attestation 
              AND NOT e.deleted 
              AND NOT e.hidden;
        """,
        
        "following_actions": """
            SELECT COUNT(et.event_tag_value1) as total_p_tags
            FROM events e
            JOIN event_tags et ON e.id = et.event_id
            WHERE e.kind = 3
              AND et.event_tag_key = 'p'
              AND NOT e.has_ephemeral_attestation
              AND NOT e.deleted
              AND NOT e.hidden;
        """,
    }
    
    @classmethod
    def get_db_stats(cls, conn_string, min_posts, days=7):
        """Get statistics from a single database"""
        results = {
            "total_posts": 0,
            "posts_with_topics": 0,
            "posts_without_topic": 0,
            "posts_per_topic": [],
            "posts_per_day": [],
            
            "reactions": 0,
            "messages": 0,
            "messages_by_wrapped_kind": [],
            "reposts": 0,
            "comments": 0,
            "articles": 0,
            "stories": 0,
            "videos": 0,
            "following_actions": 0,
            "profile_updates_by_database": [],
            
            "reactions_per_day": [],
            "messages_per_day": [],
            "reposts_per_day": [],
            "comments_per_day": [],
            "articles_per_day": [],
            "stories_per_day": [],
            "videos_per_day": [],
            "profile_updates_per_day": [],
            
            "error": None
        }
        
        try:
            daily_queries = cls.get_daily_queries()
            days_minus_1 = days - 1
            
            with psycopg.connect(conn_string) as conn:
                with conn.cursor() as cur:
                    cur.execute(cls.QUERIES["total_posts"])
                    results["total_posts"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["posts_with_topics"])
                    results["posts_with_topics"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["posts_without_topic"])
                    results["posts_without_topic"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["posts_per_topic"], (min_posts,))
                    results["posts_per_topic"] = [
                        {"topic": r[0], "post_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(daily_queries["posts_per_day"], (days_minus_1, days))
                    results["posts_per_day"] = [
                        {"post_date": str(r[0]), "post_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(cls.QUERIES["reactions"])
                    results["reactions"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["messages"])
                    results["messages"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["messages_by_wrapped_kind"])
                    results["messages_by_wrapped_kind"] = [
                        {"wrapped_kind": r[0], "message_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(cls.QUERIES["reposts"])
                    results["reposts"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["comments"])
                    results["comments"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["articles"])
                    results["articles"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["stories"])
                    results["stories"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["videos"])
                    results["videos"] = cur.fetchone()[0]
                    
                    cur.execute(cls.QUERIES["following_actions"])
                    following_result = cur.fetchone()
                    results["following_actions"] = following_result[0]
                    
                    cur.execute("SELECT COUNT(*) FROM events WHERE kind = 0 AND NOT has_ephemeral_attestation AND NOT deleted AND NOT hidden;")
                    profile_count = cur.fetchone()[0]
                    results["profile_updates_by_database"] = [
                        {"database_name": "Current Database", "profile_count": profile_count}
                    ]
                    
                    cur.execute(daily_queries["reactions_per_day"], (days_minus_1, days))
                    results["reactions_per_day"] = [
                        {"event_date": str(r[0]), "event_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(daily_queries["messages_per_day"], (days_minus_1, days))
                    results["messages_per_day"] = [
                        {"event_date": str(r[0]), "event_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(daily_queries["reposts_per_day"], (days_minus_1, days))
                    results["reposts_per_day"] = [
                        {"event_date": str(r[0]), "event_count": r[1]} for r in cur.fetchall()
                    ]
                    cur.execute(daily_queries["comments_per_day"], (days_minus_1, days))
                    results["comments_per_day"] = [
                        {"event_date": str(r[0]), "event_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(daily_queries["articles_per_day"], (days_minus_1, days))
                    results["articles_per_day"] = [
                        {"event_date": str(r[0]), "event_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(daily_queries["stories_per_day"])
                    results["stories_per_day"] = [
                        {"event_date": str(r[0]), "event_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(daily_queries["videos_per_day"], (days_minus_1, days))
                    results["videos_per_day"] = [
                        {"video_date": str(r[0]), "video_count": r[1]} for r in cur.fetchall()
                    ]
                    
                    cur.execute(daily_queries["profile_updates_per_day"], (days_minus_1, days))
                    results["profile_updates_per_day"] = [
                        {"event_date": str(r[0]), "event_count": r[1]} for r in cur.fetchall()
                    ]
                    
        except psycopg.Error as e:
            print(f"Database connection or query error: {e}")
            results["error"] = f"Could not connect or query database. ({type(e).__name__})"
        except Exception as e:
            print(f"An unexpected error occurred in get_db_stats: {e}")
            results["error"] = "An unexpected server-side error occurred."
        
        return results
    
    @staticmethod
    def _aggregate_daily_data(aggregated_dict, server_data, date_key="event_date", count_key="event_count"):
        """Helper function to aggregate daily time series data"""
        for item in server_data:
            date = item[date_key]
            aggregated_dict[date] = aggregated_dict.get(date, 0) + item[count_key]
    
    @classmethod
    def stream_aggregate_stats(cls, servers_config, min_posts_per_topic, days=7):
        """Stream aggregated statistics from multiple databases"""
        total_servers = len(servers_config)
        yield f"data: {json.dumps({'type': 'progress', 'current': 0, 'total': total_servers})}\n\n"

        total_results = {
            "total_posts": 0,
            "posts_with_topics": 0,
            "posts_without_topic": 0,
            
            "reactions": 0,
            "messages": 0,
            "reposts": 0,
            "comments": 0,
            "articles": 0,
            "stories": 0,
            "videos": 0,
            "following_actions": 0,
            
            "error": None
        }
        aggregated_topics = {}
        aggregated_days = {}
        aggregated_messages_by_kind = {}
        aggregated_profile_databases = {}
        
        aggregated_reactions_per_day = {}
        aggregated_messages_per_day = {}
        aggregated_reposts_per_day = {}
        aggregated_comments_per_day = {}
        aggregated_articles_per_day = {}
        aggregated_stories_per_day = {}
        aggregated_videos_per_day = {}
        aggregated_profiles_per_day = {}
        
        successful_fetches = 0

        for i, (alias, config) in enumerate(servers_config.items()):
            yield f"data: {json.dumps({'type': 'progress', 'current': i + 1, 'total': total_servers, 'server_name': alias})}\n\n"
            
            server_stats = cls.get_db_stats(config["conn_string"], min_posts=1, days=days)
            
            if server_stats.get("error"):
                print(f"Warning: Could not fetch stats from {alias}. Error: {server_stats['error']}")
                continue
            
            successful_fetches += 1
            
            total_results["total_posts"] += server_stats["total_posts"]
            total_results["posts_with_topics"] += server_stats["posts_with_topics"]
            total_results["posts_without_topic"] += server_stats["posts_without_topic"]
            
            total_results["reactions"] += server_stats["reactions"]
            total_results["messages"] += server_stats["messages"]
            total_results["reposts"] += server_stats["reposts"]
            total_results["comments"] += server_stats["comments"]
            total_results["articles"] += server_stats["articles"]
            total_results["stories"] += server_stats["stories"]
            total_results["videos"] += server_stats["videos"]
            total_results["following_actions"] += server_stats["following_actions"]
            
            for item in server_stats["posts_per_topic"]:
                topic = item["topic"]
                aggregated_topics[topic] = aggregated_topics.get(topic, 0) + item["post_count"]
            
            cls._aggregate_daily_data(aggregated_days, server_stats["posts_per_day"], "post_date", "post_count")
            
            for item in server_stats["messages_by_wrapped_kind"]:
                kind = item["wrapped_kind"]
                aggregated_messages_by_kind[kind] = aggregated_messages_by_kind.get(kind, 0) + item["message_count"]
            
            for item in server_stats["profile_updates_by_database"]:
                profile_count = item["profile_count"]
                aggregated_profile_databases[alias] = aggregated_profile_databases.get(alias, 0) + profile_count
            
            cls._aggregate_daily_data(aggregated_reactions_per_day, server_stats["reactions_per_day"])
            cls._aggregate_daily_data(aggregated_messages_per_day, server_stats["messages_per_day"])
            cls._aggregate_daily_data(aggregated_reposts_per_day, server_stats["reposts_per_day"])
            cls._aggregate_daily_data(aggregated_comments_per_day, server_stats["comments_per_day"])
            cls._aggregate_daily_data(aggregated_articles_per_day, server_stats["articles_per_day"])
            cls._aggregate_daily_data(aggregated_stories_per_day, server_stats["stories_per_day"])
            cls._aggregate_daily_data(aggregated_videos_per_day, server_stats["videos_per_day"], "video_date", "video_count")
            cls._aggregate_daily_data(aggregated_profiles_per_day, server_stats["profile_updates_per_day"])

        if successful_fetches == 0:
            total_results["error"] = "Failed to fetch data from any of the configured servers."
            yield f"data: {json.dumps({'type': 'result', 'data': total_results})}\n\n"
            return

        filtered_topics = [
            {"topic": t, "post_count": c} 
            for t, c in aggregated_topics.items() 
            if c >= min_posts_per_topic
        ]
        total_results["posts_per_topic"] = sorted(filtered_topics, key=lambda x: x['post_count'], reverse=True)
        
        total_results["posts_per_day"] = [
            {"post_date": d, "post_count": c} 
            for d, c in sorted(aggregated_days.items())
        ]
        
        total_results["messages_by_wrapped_kind"] = [
            {"wrapped_kind": k, "message_count": c}
            for k, c in sorted(aggregated_messages_by_kind.items(), key=lambda x: x[1], reverse=True)
        ]
        
        total_results["profile_updates_by_database"] = [
            {"database_name": db, "profile_count": c}
            for db, c in sorted(aggregated_profile_databases.items(), key=lambda x: x[1], reverse=True)
        ]
        
        total_results["reactions_per_day"] = [
            {"event_date": d, "event_count": c}
            for d, c in sorted(aggregated_reactions_per_day.items())
        ]
        
        total_results["messages_per_day"] = [
            {"event_date": d, "event_count": c}
            for d, c in sorted(aggregated_messages_per_day.items())
        ]
        
        total_results["reposts_per_day"] = [
            {"event_date": d, "event_count": c}
            for d, c in sorted(aggregated_reposts_per_day.items())
        ]
        
        total_results["comments_per_day"] = [
            {"event_date": d, "event_count": c}
            for d, c in sorted(aggregated_comments_per_day.items())
        ]
        
        total_results["articles_per_day"] = [
            {"event_date": d, "event_count": c}
            for d, c in sorted(aggregated_articles_per_day.items())
        ]
        
        total_results["stories_per_day"] = [
            {"event_date": d, "event_count": c}
            for d, c in sorted(aggregated_stories_per_day.items())
        ]
        
        total_results["videos_per_day"] = [
            {"video_date": d, "video_count": c}
            for d, c in sorted(aggregated_videos_per_day.items())
        ]
        
        total_results["profile_updates_per_day"] = [
            {"event_date": d, "event_count": c}
            for d, c in sorted(aggregated_profiles_per_day.items())
        ]

        yield f"data: {json.dumps({'type': 'result', 'data': total_results})}\n\n"
