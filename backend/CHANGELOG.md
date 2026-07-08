# Changelog

## [0.7.1](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.7.0...backend-v0.7.1) (2026-07-08)


### Bug Fixes

* **backend:** use ?? as the curate placeholder instead of (?) ([0169cf8](https://github.com/andreistefanciprian/phrasely/commit/0169cf8b21a18b94a5f42af0ea27768a34d12248))

## [0.7.0](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.6.1...backend-v0.7.0) (2026-06-26)


### Features

* **backend:** deep-link phrase digest email to the specific phrase ([b9645b2](https://github.com/andreistefanciprian/phrasely/commit/b9645b2b163e54d17bed98463aed0964c9a6ee02))

## [0.6.1](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.6.0...backend-v0.6.1) (2026-06-22)


### Bug Fixes

* **phrasedigest:** remove fixed UTC hour gate ([c9a51c9](https://github.com/andreistefanciprian/phrasely/commit/c9a51c9362dd70c4d3b0e8f834718bd52c254126))

## [0.6.0](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.5.0...backend-v0.6.0) (2026-06-21)


### Features

* **backend,frontend:** add Phrase Digest email feature ([90d719f](https://github.com/andreistefanciprian/phrasely/commit/90d719f713f62c4654016fe62515012610dc773d))


### Bug Fixes

* **docs:** align phrase-digest worker schedule description with actual cron ([7707ccd](https://github.com/andreistefanciprian/phrasely/commit/7707ccd596fdebf11f9ce642fbe355f8da0cd7a8))

## [0.5.0](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.4.0...backend-v0.5.0) (2026-06-20)


### Features

* **backend,frontend:** add related phrases and rename shuffle page ([427302a](https://github.com/andreistefanciprian/phrasely/commit/427302a8fddb7c0b8e41e2321481c9bd4bb1983f))

## [0.4.0](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.3.0...backend-v0.4.0) (2026-06-20)


### Features

* **backend,mcp:** add sample_phrases tool and GET /api/v1/phrases/random endpoint ([c7cbb17](https://github.com/andreistefanciprian/phrasely/commit/c7cbb17310a8662c32a8765a4c199a5bc5320112)), closes [#134](https://github.com/andreistefanciprian/phrasely/issues/134)


### Bug Fixes

* **mcp,backend:** address code review findings on sample_phrases ([3d05873](https://github.com/andreistefanciprian/phrasely/commit/3d05873e71b420223abdfdaf84103e23dc91af90))

## [0.3.0](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.2.2...backend-v0.3.0) (2026-06-20)


### Features

* **backend+mcp:** add phrases summary endpoint and update list_phrases tool ([693fa1a](https://github.com/andreistefanciprian/phrasely/commit/693fa1a06b0aedf754e1f521b05098428942ca04)), closes [#133](https://github.com/andreistefanciprian/phrasely/issues/133)
* **backend+mcp:** restore headword filter on summary endpoint and fix test shape assertion ([75a24ca](https://github.com/andreistefanciprian/phrasely/commit/75a24ca4e0101177e5d64812a48e4367e7e6a1bd))

## [0.2.2](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.2.1...backend-v0.2.2) (2026-06-19)


### Bug Fixes

* **backend:** make OAuth access token TTL configurable via OAUTH_ACCESS_TOKEN_TTL ([633615b](https://github.com/andreistefanciprian/phrasely/commit/633615bfc03852f7bda0e3b6384f7663c011dae9))
* **backend:** update oauth handler tests for OAUTH_ACCESS_TOKEN_TTL param ([3114ff0](https://github.com/andreistefanciprian/phrasely/commit/3114ff03323cf96841cfc48ce0b3fd33118576e7))

## [0.2.1](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.2.0...backend-v0.2.1) (2026-06-18)


### Bug Fixes

* **backend:** make embed-backfill async with per-phrase progress logging ([8255dfb](https://github.com/andreistefanciprian/phrasely/commit/8255dfb3a59032df2ba1d4aeaa89cc352631f9d3))

## [0.2.0](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.1.0...backend-v0.2.0) (2026-06-18)


### Features

* **backend:** add semantic phrase search endpoint ([7799ea4](https://github.com/andreistefanciprian/phrasely/commit/7799ea4dec9d1dd7a57f3dcff224ef5b5f73ba7f))


### Bug Fixes

* **backend:** trim and cap search query in semantic search handler ([5b069e5](https://github.com/andreistefanciprian/phrasely/commit/5b069e5bab24ee3eeb9bc95bed1a9113f83ef943))
* **frontend:** restore empty-state message when user has no phrases ([5b069e5](https://github.com/andreistefanciprian/phrasely/commit/5b069e5bab24ee3eeb9bc95bed1a9113f83ef943))

## [0.1.0](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.10...backend-v0.1.0) (2026-06-17)


### Features

* **backend:** add pgvector foundation for semantic phrase search ([#102](https://github.com/andreistefanciprian/phrasely/issues/102)) ([27ae101](https://github.com/andreistefanciprian/phrasely/commit/27ae101f360819d1e156a5ab75cc9ba5cf5cfe4f))

## [0.0.10](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.9...backend-v0.0.10) (2026-06-17)


### Features

* **mcp:** implement RFC 7009 token revocation endpoint ([c585c2b](https://github.com/andreistefanciprian/phrasely/commit/c585c2b7a4f2757735ccc7472d747e2ef8948689))


### Bug Fixes

* **frontend:** improve logging, comments and test coverage for token revocation ([0174a04](https://github.com/andreistefanciprian/phrasely/commit/0174a042f2a9add82cfb0081461de5ba5d920626))
* **frontend:** revoke OAuth tokens when user denies consent ([44153c1](https://github.com/andreistefanciprian/phrasely/commit/44153c1f113c69bf089e7a6f32ea147e21da50e8))

## [0.0.9](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.8...backend-v0.0.9) (2026-06-16)


### Features

* **backend:** add configurable log level via LOG_LEVEL env var ([#77](https://github.com/andreistefanciprian/phrasely/issues/77)) ([e3d05ff](https://github.com/andreistefanciprian/phrasely/commit/e3d05ff638ac44acf42fb2b7a2e7f1a854d9e888))

## [0.0.8](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.7...backend-v0.0.8) (2026-06-16)


### Features

* **backend:** add refresh_token grant with rotation ([#71](https://github.com/andreistefanciprian/phrasely/issues/71)) ([24f9f93](https://github.com/andreistefanciprian/phrasely/commit/24f9f93fdfc81465d55445d33bf57a8767b59917))
* **mcp:** thread per-request OAuth JWT into MCP tools ([#73](https://github.com/andreistefanciprian/phrasely/issues/73)) ([6cc03ab](https://github.com/andreistefanciprian/phrasely/commit/6cc03ab83833b0b9a84231bdeb00b381416c0f02))

## [0.0.7](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.6...backend-v0.0.7) (2026-06-16)


### Features

* **backend:** add OAuth 2.1 token endpoint with PKCE verification ([#67](https://github.com/andreistefanciprian/phrasely/issues/67)) ([4f9cf54](https://github.com/andreistefanciprian/phrasely/commit/4f9cf540a91177867aeb9f512f1508cb2d2a0d12))

## [0.0.6](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.5...backend-v0.0.6) (2026-06-16)


### Features

* **frontend:** add OAuth 2.1 consent screen at /authorize ([#64](https://github.com/andreistefanciprian/phrasely/issues/64)) ([456c3c3](https://github.com/andreistefanciprian/phrasely/commit/456c3c3a5abe430662db504734b6e1ac91ec8141))

## [0.0.5](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.4...backend-v0.0.5) (2026-06-16)


### Features

* **backend:** add internal OAuth authorize endpoint ([a42cdd1](https://github.com/andreistefanciprian/phrasely/commit/a42cdd1695af0f5b6b28f7d8f8358e89a103ee49))
* **mcp:** add OAuth 2.1 discovery and dynamic client registration proxy ([e62f196](https://github.com/andreistefanciprian/phrasely/commit/e62f196304af50c54b6a785c39cbd022c086ca46))

## [0.0.4](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.3...backend-v0.0.4) (2026-06-16)


### Features

* **backend:** add Dynamic Client Registration internal endpoint ([e8022c3](https://github.com/andreistefanciprian/phrasely/commit/e8022c39b01983bd4e5e5a047747aa33763ce2df))


### Bug Fixes

* **backend:** narrow internal route exemption to /internal/oauth/ ([09a1119](https://github.com/andreistefanciprian/phrasely/commit/09a1119ab8a33b93fe1f3515acd9a3840186a2d2))

## [0.0.3](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.2...backend-v0.0.3) (2026-06-15)


### Bug Fixes

* **backend:** improve curate prompt for idiom dictionary links ([0185a92](https://github.com/andreistefanciprian/phrasely/commit/0185a92b66189281d5ce0fa40520049a964c811b))
* **backend:** improve curate prompt for idiom dictionary links ([36168d5](https://github.com/andreistefanciprian/phrasely/commit/36168d56d2d80e45403ba3840a2565ab0f2045d2))

## [0.0.2](https://github.com/andreistefanciprian/phrasely/compare/backend-v0.0.1...backend-v0.0.2) (2026-06-12)


### Features

* **backend:** make magic link and JWT expiry configurable ([e805887](https://github.com/andreistefanciprian/phrasely/commit/e80588786ce9d85c79f09938aa80ebdb31b4d7ec))

## 0.0.1 (2026-06-12)


### Bug Fixes

* **backend:** reject blank headword strings ([499fcfd](https://github.com/andreistefanciprian/phrasely/commit/499fcfd6391729012329c2cbbb24fad97fdbf1db))
* **backend:** reject blank headword strings ([cb7b826](https://github.com/andreistefanciprian/phrasely/commit/cb7b8269f2c8c708bf92a77e6ff5791bdac18272))
