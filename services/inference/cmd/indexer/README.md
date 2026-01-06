```
export PG_URL='postgres://crowdaudit:crowdaudit@localhost:5432/crowdaudit?sslmode=disable'
export KAFKA_BROKERS='localhost:9092'
export KAFKA_TOPIC='search-index'
export KAFKA_GROUP_ID='search-indexer'
export OS_URL='http://localhost:9200'
go run ./cmd/indexer
```