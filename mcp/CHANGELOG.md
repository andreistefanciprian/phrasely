# Changelog

## [0.3.0](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.2.1...mcp-v0.3.0) (2026-06-22)


### Features

* **mcp:** add curate tool and make add_phrase save-only ([d2031fe](https://github.com/andreistefanciprian/phrasely/commit/d2031fe4819d6f16f81149e53fbc4d64c233e45f))

## [0.2.1](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.2.0...mcp-v0.2.1) (2026-06-20)


### Bug Fixes

* **mcp:** add tool annotations and server instructions for ChatGPT submission ([82ee90a](https://github.com/andreistefanciprian/phrasely/commit/82ee90a6a91ad6d234c26d3ded79a8c0b3552cfc))

## [0.2.0](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.1.0...mcp-v0.2.0) (2026-06-20)


### Features

* **backend,mcp:** add sample_phrases tool and GET /api/v1/phrases/random endpoint ([c7cbb17](https://github.com/andreistefanciprian/phrasely/commit/c7cbb17310a8662c32a8765a4c199a5bc5320112)), closes [#134](https://github.com/andreistefanciprian/phrasely/issues/134)


### Bug Fixes

* **mcp,backend:** address code review findings on sample_phrases ([3d05873](https://github.com/andreistefanciprian/phrasely/commit/3d05873e71b420223abdfdaf84103e23dc91af90))

## [0.1.0](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.9...mcp-v0.1.0) (2026-06-20)


### Features

* **backend+mcp:** add phrases summary endpoint and update list_phrases tool ([693fa1a](https://github.com/andreistefanciprian/phrasely/commit/693fa1a06b0aedf754e1f521b05098428942ca04)), closes [#133](https://github.com/andreistefanciprian/phrasely/issues/133)
* **backend+mcp:** restore headword filter on summary endpoint and fix test shape assertion ([75a24ca](https://github.com/andreistefanciprian/phrasely/commit/75a24ca4e0101177e5d64812a48e4367e7e6a1bd))

## [0.0.9](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.8...mcp-v0.0.9) (2026-06-17)


### Bug Fixes

* **mcp:** expand add_phrase description with curation instructions ([#100](https://github.com/andreistefanciprian/phrasely/issues/100)) ([8db6d0f](https://github.com/andreistefanciprian/phrasely/commit/8db6d0fc1c4a12ef58a2b6b5a3c716df2dce4768))

## [0.0.8](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.7...mcp-v0.0.8) (2026-06-17)


### Features

* **mcp:** implement RFC 7009 token revocation endpoint ([c585c2b](https://github.com/andreistefanciprian/phrasely/commit/c585c2b7a4f2757735ccc7472d747e2ef8948689))

## [0.0.7](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.6...mcp-v0.0.7) (2026-06-16)


### Features

* **backend:** add configurable log level via LOG_LEVEL env var ([#77](https://github.com/andreistefanciprian/phrasely/issues/77)) ([e3d05ff](https://github.com/andreistefanciprian/phrasely/commit/e3d05ff638ac44acf42fb2b7a2e7f1a854d9e888))

## [0.0.6](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.5...mcp-v0.0.6) (2026-06-16)


### Features

* **mcp:** add configurable log level via LOG_LEVEL env var ([#75](https://github.com/andreistefanciprian/phrasely/issues/75)) ([30280cc](https://github.com/andreistefanciprian/phrasely/commit/30280cc381626f2810f79ffa57b84b93b7da0469))

## [0.0.5](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.4...mcp-v0.0.5) (2026-06-16)


### Features

* **mcp:** thread per-request OAuth JWT into MCP tools ([#73](https://github.com/andreistefanciprian/phrasely/issues/73)) ([6cc03ab](https://github.com/andreistefanciprian/phrasely/commit/6cc03ab83833b0b9a84231bdeb00b381416c0f02))

## [0.0.4](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.3...mcp-v0.0.4) (2026-06-16)


### Features

* **mcp:** add /token proxy and fix Content-Type forwarding ([#69](https://github.com/andreistefanciprian/phrasely/issues/69)) ([611a009](https://github.com/andreistefanciprian/phrasely/commit/611a00960bd591128d050607480c792cf45d2ca9))

## [0.0.3](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.2...mcp-v0.0.3) (2026-06-16)


### Features

* **mcp:** add OAuth 2.1 discovery and dynamic client registration proxy ([e62f196](https://github.com/andreistefanciprian/phrasely/commit/e62f196304af50c54b6a785c39cbd022c086ca46))


### Bug Fixes

* **mcp:** return 413 instead of silently truncating oversized proxy bodies ([65b28a9](https://github.com/andreistefanciprian/phrasely/commit/65b28a9cd95daf69d83f8754c0d463e7e9f5d3a7))

## [0.0.2](https://github.com/andreistefanciprian/phrasely/compare/mcp-v0.0.1...mcp-v0.0.2) (2026-06-15)


### Features

* **mcp:** add add_phrase tool ([5fd1796](https://github.com/andreistefanciprian/phrasely/commit/5fd17961ee5678a752f541073eb18e39a3a3177b))
* **mcp:** add add_phrase tool ([c83c86e](https://github.com/andreistefanciprian/phrasely/commit/c83c86e993e5f01bd8f9eebe5c7294e9a9e20ade))

## 0.0.1 (2026-06-15)


### Features

* **mcp:** add apiClient and list_phrases tool ([d89c376](https://github.com/andreistefanciprian/phrasely/commit/d89c3761a4366758cb286756b9eb66f66d1da66a))
* **mcp:** add apiClient and list_phrases tool ([748c6ee](https://github.com/andreistefanciprian/phrasely/commit/748c6ee0f7555a8ab3da4a8dbcce19a8f3c180bb))
* **mcp:** scaffold mcp server module ([e3abe9a](https://github.com/andreistefanciprian/phrasely/commit/e3abe9a1543ae955f8b9233b0ecb7d66be68a5f6))
* **mcp:** scaffold mcp server module ([f420ac6](https://github.com/andreistefanciprian/phrasely/commit/f420ac60d5e48b5334fe17b3d4af6ff187ecc42f))
* **mcp:** wire up MCP Streamable HTTP protocol ([6569434](https://github.com/andreistefanciprian/phrasely/commit/65694341e74c2a292fe5aef4efa0684ececec222))
* **mcp:** wire up MCP Streamable HTTP protocol ([228f465](https://github.com/andreistefanciprian/phrasely/commit/228f46593917adae492e214198e4024350ee5d36))
