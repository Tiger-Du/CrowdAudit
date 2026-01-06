Load data from CSV:

```
docker compose exec -T postgres psql \
  -U crowdaudit -d crowdaudit \
  -c "\copy community_alignment_conversations (conversation_id,assigned_lang,annotator_id,first_turn_preferred_response,first_turn_prompt,first_turn_response_a,first_turn_response_b,first_turn_feedback) FROM STDIN WITH (FORMAT csv, HEADER true, NULL '\N', QUOTE '\"', ESCAPE '\"');" \
  < community_alignment_conversations.csv
```