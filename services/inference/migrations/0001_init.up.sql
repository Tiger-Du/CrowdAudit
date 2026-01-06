-- prompts: a question/task users are judging answers for
create table prompts (
  id bigserial primary key,
  title text not null,
  body text not null,
  created_at timestamptz not null default now()
);

-- responses: LLM outputs (could include provider/model metadata)
create table responses (
  id bigserial primary key,
  prompt_id bigint not null references prompts(id) on delete cascade,
  provider text not null,
  model text not null,
  content text not null,
  created_at timestamptz not null default now()
);

-- pairs: stable pairing of two responses for the same prompt
create table response_pairs (
  id bigserial primary key,
  prompt_id bigint not null references prompts(id) on delete cascade,
  response_a_id bigint not null references responses(id) on delete cascade,
  response_b_id bigint not null references responses(id) on delete cascade,
  created_at timestamptz not null default now(),
  constraint pairs_same_prompt check (response_a_id <> response_b_id)
);

-- votes: one user/session votes on a pair
-- voter_id can be a user id, or an anonymous session id.
create table votes (
  id bigserial primary key,
  pair_id bigint not null references response_pairs(id) on delete cascade,
  voter_id text not null,
  choice smallint not null, -- 1 = A, 2 = B, 3 = Tie
  created_at timestamptz not null default now(),
  -- idempotency: only one vote per pair per voter
  constraint votes_unique unique (pair_id, voter_id)
);

CREATE TABLE IF NOT EXISTS community_alignment_conversations (
  conversation_id                bigint PRIMARY KEY,   -- or drop PK if duplicates possible
  assigned_lang                  text NOT NULL,
  annotator_id                   bigint NOT NULL,
  first_turn_preferred_response  text,
  first_turn_prompt              text,
  first_turn_response_a          text,
  first_turn_response_b          text,
  first_turn_feedback            text,
  feedback_score INTEGER DEFAULT 0
);

create table outbox_events (
  id bigserial primary key,
  topic text not null,
  key text not null,
  event_type text not null,
  payload jsonb not null,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  attempts int not null default 0,
  last_error text
);

CREATE TABLE eligible_models (
    id TEXT PRIMARY KEY,       -- e.g., 'google/gemini-flash-1.5:free'
    display_name TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    category TEXT,             -- 'reasoning', 'fast', 'creative'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

create index idx_responses_prompt on responses(prompt_id);
create index idx_pairs_prompt on response_pairs(prompt_id);
create index idx_votes_pair on votes(pair_id);
create index idx_outbox_unpublished
  on outbox_events (id)
  where published_at is null;
create index idx_outbox_created_at
  on outbox_events (created_at);

CREATE INDEX IF NOT EXISTS community_alignment_conversations_annotator_id_idx ON community_alignment_conversations (annotator_id);
CREATE INDEX IF NOT EXISTS community_alignment_conversations_assigned_lang_idx ON community_alignment_conversations (assigned_lang);
