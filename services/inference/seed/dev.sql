-- 1) prompt
insert into prompts (id, title, body)
values (1, 'Explain CAP theorem', 'Explain the CAP theorem in distributed systems.')
on conflict (id) do nothing;

-- 2) responses
insert into responses (id, prompt_id, provider, model, content)
values
  (1, 1, 'openai', 'gpt-4.1', 'CAP says you can only fully guarantee two of Consistency, Availability, Partition tolerance...'),
  (2, 1, 'anthropic', 'claude-3.5', 'CAP theorem states that in the presence of a network partition...')
on conflict (id) do nothing;

-- 3) pair
insert into response_pairs (id, prompt_id, response_a_id, response_b_id)
values (1, 1, 1, 2)
on conflict (id) do nothing;

-- 4) votes
insert into votes (pair_id, voter_id, choice)
values
  (1, 'dev-user-001', 1),
  (1, 'dev-user-002', 2),
  (1, 'dev-user-003', 1),
  (1, 'dev-user-004', 3)
on conflict (pair_id, voter_id) do nothing;

insert into eligible_models (id, display_name)
values
  ('allenai/olmo-3.1-32b-think:free', 'AllenAI: Olmo 3.1 32B Think (free)'),
  ('nvidia/nemotron-3-nano-30b-a3b:free', 'NVIDIA: Nemotron 3 Nano 30B A3B (free)'),
  ('mistralai/devstral-2512:free', 'Mistral: Devstral 2 2512 (free)'),
  ('allenai/olmo-3-32b-think:free', 'AllenAI: Olmo 3 32B Think (free)'),
  ('nvidia/nemotron-nano-12b-v2-vl:free', 'NVIDIA: Nemotron Nano 12B 2 VL (free)'),
  ('nvidia/nemotron-nano-9b-v2:free', 'NVIDIA: Nemotron Nano 9B V2 (free)'),
  ('openai/gpt-oss-120b:free', 'OpenAI: gpt-oss-120b (free)'),
  ('openai/gpt-oss-20b:free', 'OpenAI: gpt-oss-20b (free)'),
  ('google/gemma-3n-e2b-it:free', 'Google: Gemma 3n 2B (free)'),
  ('google/gemma-3n-e4b-it:free', 'Google: Gemma 3n 4B (free)'),
  ('mistralai/mistral-small-3.1-24b-instruct:free', 'Mistral: Mistral Small 3.1 24B (free)'),
  ('google/gemma-3-4b-it:free', 'Google: Gemma 3 4B (free)'),
  ('google/gemma-3-12b-it:free', 'Google: Gemma 3 12B (free)'),
  ('google/gemma-3-27b-it:free', 'Google: Gemma 3 27B (free)'),
  ('google/gemini-2.0-flash-exp:free', 'Google: Gemini 2.0 Flash Experimental (free)'),
  ('meta-llama/llama-3.3-70b-instruct:free', 'Meta: Llama 3.3 70B Instruct (free)'),
  ('meta-llama/llama-3.2-3b-instruct:free', 'Meta: Llama 3.2 3B Instruct (free)'),
  ('meta-llama/llama-3.1-405b-instruct:free', 'Meta: Llama 3.1 405B Instruct (free)'),
  ('mistralai/mistral-7b-instruct:free', 'Mistral: Mistral 7B Instruct (free)')
on conflict (id, display_name) do nothing;
