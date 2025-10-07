-- SPDX-License-Identifier: ice License 1.0

--- TODO: optimize index size and usage.

create index if not exists ranked_events_points_ix           on ranked_events(points) where points <= 0;
create index if not exists ranked_events_score_ix            on ranked_events(score desc);
create index if not exists ranked_events_verified_score_ix   on ranked_events(event_verified desc, score desc);
create index if not exists ranked_events_created_at_score_ix on ranked_events(event_created_at desc, score desc);
create index if not exists ranked_events_verified_created_at_score_ix on ranked_events(event_verified desc, event_created_at desc, score desc);
