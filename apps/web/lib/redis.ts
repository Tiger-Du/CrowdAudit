import Redis from "ioredis";

declare global {
  // eslint-disable-next-line no-var
  var redis: Redis | null | undefined;
}

const redis =
  globalThis.redis ??
  (process.env.REDIS_CACHE_URL
    ? new Redis(process.env.REDIS_CACHE_URL)
    : null);

if (process.env.NODE_ENV !== "production") {
  globalThis.redis = redis;
}

export default redis;
