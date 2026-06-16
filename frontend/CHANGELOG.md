# Changelog

## [0.0.3](https://github.com/andreistefanciprian/phrasely/compare/frontend-v0.0.2...frontend-v0.0.3) (2026-06-16)


### Features

* **frontend:** add OAuth 2.1 consent screen at /authorize ([#64](https://github.com/andreistefanciprian/phrasely/issues/64)) ([456c3c3](https://github.com/andreistefanciprian/phrasely/commit/456c3c3a5abe430662db504734b6e1ac91ec8141))

## [0.0.2](https://github.com/andreistefanciprian/phrasely/compare/frontend-v0.0.1...frontend-v0.0.2) (2026-06-12)


### Bug Fixes

* **frontend:** match navbar logo size on story page ([262a526](https://github.com/andreistefanciprian/phrasely/commit/262a52660dafc20d2c1a7f143e38fc5345f8d6b1))
* **frontend:** match navbar logo size on story page ([44da5b7](https://github.com/andreistefanciprian/phrasely/commit/44da5b7b0f33a0b6a206920cf8ae130dd8677f37))

## 0.0.1 (2026-06-12)


### Features

* add bubble preview image to landing page; shorten story teaser ([ff3905b](https://github.com/andreistefanciprian/phrasely/commit/ff3905b56249c47185102cf8564dc3852824e748))
* add caption above bubble preview; reduce gap between sections ([f020bff](https://github.com/andreistefanciprian/phrasely/commit/f020bffd22090f13f6774dc4530023f3e6b4b6af))
* add custom 404 page with branded styling ([c325341](https://github.com/andreistefanciprian/phrasely/commit/c32534141587774358b27dbad12b31ddb17a4265))
* add meta tags, llms.txt, robots.txt for SEO and LLM crawlers ([bc008f0](https://github.com/andreistefanciprian/phrasely/commit/bc008f0b4df7bb65434427f9ec843520237ca749))
* mobile bottom tab bar, story page, SEO meta tags, llms.txt ([d0f7ddd](https://github.com/andreistefanciprian/phrasely/commit/d0f7dddda6abed19f76ee7d2e721bd3c519e6483))
* mobile bottom tab bar, story page, SEO meta tags, llms.txt ([540f349](https://github.com/andreistefanciprian/phrasely/commit/540f3491d50aab7ea301a17dcb8a33a6ee12d88f))
* nginx reverse proxy + non-privileged frontend container ([e846697](https://github.com/andreistefanciprian/phrasely/commit/e84669756e91405f9e4b6a24ffcbd2096ce4697f))
* vanilla JS frontend + API improvements ([34bd29e](https://github.com/andreistefanciprian/phrasely/commit/34bd29e9c043c35f9ace88e9d6f50b57564fcc56))


### Bug Fixes

* add aria-label to both nav landmarks for screen reader accessibility ([6a80d74](https://github.com/andreistefanciprian/phrasely/commit/6a80d7425345ad1045c06182b3168abbbf929d28))
* add DNS resolver to nginx so upstream hostname is re-resolved dynamically ([0f735c0](https://github.com/andreistefanciprian/phrasely/commit/0f735c0a41047cc5f5afbc869e942d70037cdc94))
* build headword links via DOM nodes to prevent XSS from javascript: URLs ([88018e8](https://github.com/andreistefanciprian/phrasely/commit/88018e8f4c6f7f9b903cd167a0f449003c4da387))
* copy only web assets to nginx web root; add aria-labels to navbar controls ([5e2255a](https://github.com/andreistefanciprian/phrasely/commit/5e2255a8abf5b7d6441d233a163434f0b8252ce4))
* escape all user-controlled fields in phrases.html and auth-verify.html ([6aa6c6c](https://github.com/andreistefanciprian/phrasely/commit/6aa6c6c55d361c0905b909a0900f7eda886b36fc))
* escape headwords and note in search results; fix invalid HTML in empty state ([50d4f38](https://github.com/andreistefanciprian/phrasely/commit/50d4f38c19f8c6d0a133be2a9cb730a67ba0ddcd))
* escape HTML in format() before injecting into innerHTML to prevent XSS ([e95dfd4](https://github.com/andreistefanciprian/phrasely/commit/e95dfd4e756524a6c880cdd441894b520fc9f031))
* filter IPv4-only nameserver from /etc/resolv.conf for nginx resolver ([90194a5](https://github.com/andreistefanciprian/phrasely/commit/90194a51e05351b44d954267d4997f4716c9c1a2))
* handle IPv6 DNS resolver — wrap in brackets for nginx, remove ipv6=off ([3f78ddd](https://github.com/andreistefanciprian/phrasely/commit/3f78ddd7ae92c2c2d5247ce44e0365f1a37cbbaa))
* make 16-set-resolver.envsh executable so nginx entrypoint sources it ([cf9bb2d](https://github.com/andreistefanciprian/phrasely/commit/cf9bb2d5e001ac578aa5e58ad50ad94c4e0e752b))
* polish 404 page copy to match the app's vocabulary theme ([d7f39b9](https://github.com/andreistefanciprian/phrasely/commit/d7f39b9b2536e8d20e44f0d00f29f54a04fb8f1a))
* polish story section copy and fix spelling ([4c7cfe7](https://github.com/andreistefanciprian/phrasely/commit/4c7cfe776b0d561a7371d996817f7520a3cdb367))
* prevent tab bar from squashing on mobile scroll ([5d6c05b](https://github.com/andreistefanciprian/phrasely/commit/5d6c05b7935fa1e735de97db083efab38e3b0dd5))
* read DNS resolver from /etc/resolv.conf at runtime for nginx upstream ([67303f2](https://github.com/andreistefanciprian/phrasely/commit/67303f2227b333243e950772e1f9df24a908192f))
* remove inline display:flex from sign-out button so CSS media query can hide it on mobile ([1b0d8f3](https://github.com/andreistefanciprian/phrasely/commit/1b0d8f3106e6c8b1226c439ebb1da6aa02078ae0))
* revert 404 copy to original without hyphen ([687f685](https://github.com/andreistefanciprian/phrasely/commit/687f6852b9ef131c36e5ab4671400c86b425a7c7))
* try/catch/finally on all fetches; clarify curate prompt; update docs ([714a36b](https://github.com/andreistefanciprian/phrasely/commit/714a36b687e324f9bf1fab7ce1feb600aac73a05))
* use COPY --chmod=755 to set executable bit as non-root user cannot chmod ([f6d4185](https://github.com/andreistefanciprian/phrasely/commit/f6d41854e37088195afba165dcf17022c8298de6))
