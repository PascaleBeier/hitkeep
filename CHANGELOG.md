# Changelog

## Unreleased

### Database migration

* Split default-tenant analytics into `data/tenants/<tenant-id>/hitkeep.db` and run tenant files through one attached DuckDB data plane. This migration is mandatory in 2.13.0. Downgrading a split installation to 2.12 or older is unsupported because those releases do not understand the tenant file or split markers; restore a complete pre-upgrade backup instead.
* Restore every tenant database from S3 backup snapshots by deriving tenant object prefixes from the restored control snapshot, including temporary STS credentials and non-TLS S3-compatible endpoints.

## [2.13.9](https://github.com/PascaleBeier/hitkeep/compare/v2.13.8...v2.13.9) (2026-08-17)


### Bug Fixes

* **ci:** isolate docs sync and harden scroll test ([bfda899](https://github.com/PascaleBeier/hitkeep/commit/bfda8995335b8492a5cab26f9fd50b982d4e4d70))
* **sites:** align domain validation across clients ([#306](https://github.com/PascaleBeier/hitkeep/issues/306)) ([ffb92b8](https://github.com/PascaleBeier/hitkeep/commit/ffb92b891d46872c81dc8a510144749e218b518c))

## [2.13.8](https://github.com/PascaleBeier/hitkeep/compare/v2.13.7...v2.13.8) (2026-08-16)


### Bug Fixes

* **backend:** Correct logging via linters ([1a0bca6](https://github.com/PascaleBeier/hitkeep/commit/1a0bca61e9be8db41c9b040431250f6e42227c00))
* **backend:** Correct logging via linters ([754d0bf](https://github.com/PascaleBeier/hitkeep/commit/754d0bf2d23a06918c98c014020ba73924b7c04f))
* **ci:** add -trimpath ([94cf015](https://github.com/PascaleBeier/hitkeep/commit/94cf015151939a8c6e0dd89c16b0c8a32acc2fd2))
* **ci:** enforce go build flags ([71b07f1](https://github.com/PascaleBeier/hitkeep/commit/71b07f1563f86836f8caea42b5860f4ff7b52a40))
* **deps:** Bump frontend dependencies ([25d6a4b](https://github.com/PascaleBeier/hitkeep/commit/25d6a4bd91eec76c002ca66751afa7fef4014f1b))
* **deps:** bump go and deps to 1.26.6 ([963f4e7](https://github.com/PascaleBeier/hitkeep/commit/963f4e7a0c7ac0823becef8c6a5bcc08b24c5f95))
* **frontend:** implement chained scrolling behavior in metric cards and update tests, fixes [#284](https://github.com/PascaleBeier/hitkeep/issues/284) ([cdd2a4a](https://github.com/PascaleBeier/hitkeep/commit/cdd2a4a3c88517551b46449b4ee6b5d0cba8e38f))
* **frontend:** use .node-version instead of .nvmrc ([9241051](https://github.com/PascaleBeier/hitkeep/commit/924105181de8ff171c61f0fc41fb99cd2d3690b7))

## [2.13.7](https://github.com/PascaleBeier/hitkeep/compare/v2.13.6...v2.13.7) (2026-08-11)


### Bug Fixes

* **dx:** Be a bit more permissive on frontend engines ([1f5c3cb](https://github.com/PascaleBeier/hitkeep/commit/1f5c3cb90825b5987776e6ef36e96b4addf5bc4d))
* **ux:** improve email verification flow ([1d04c01](https://github.com/PascaleBeier/hitkeep/commit/1d04c0152d198bf1d34f3def17061e575d508ca6))

## [2.13.6](https://github.com/PascaleBeier/hitkeep/compare/v2.13.5...v2.13.6) (2026-08-07)


### Bug Fixes

* **docker:** persist tenant data path and recover interrupted tenant split ([479d932](https://github.com/PascaleBeier/hitkeep/commit/479d93213cd86f78cb932533904c2ab4f98fbc60)), closes [#288](https://github.com/PascaleBeier/hitkeep/issues/288)
* **recover:** add rebuild-default-tenant recovery command ([709024f](https://github.com/PascaleBeier/hitkeep/commit/709024f1f8bb775a2e3ab700f99f89d931602986)), closes [#288](https://github.com/PascaleBeier/hitkeep/issues/288)
* **recover:** import retention archives into a tenant database ([6fa7d7e](https://github.com/PascaleBeier/hitkeep/commit/6fa7d7ee6583bd6cb304c3705ca651601fa92c45)), closes [#288](https://github.com/PascaleBeier/hitkeep/issues/288)

## [2.13.5](https://github.com/PascaleBeier/hitkeep/compare/v2.13.4...v2.13.5) (2026-08-07)


### Bug Fixes

* **mailer:** prevent duplicate message-id headers ([a648925](https://github.com/PascaleBeier/hitkeep/commit/a6489259d86ec1837780e2a813eb96f5c4bc3c06))

## [2.13.4](https://github.com/PascaleBeier/hitkeep/compare/v2.13.3...v2.13.4) (2026-08-07)


### Bug Fixes

* **auth:** harden SSO entitlement and UX ([1d446f3](https://github.com/PascaleBeier/hitkeep/commit/1d446f32223ae2a7d8703916e0e75623e00d681f))
* **deps:** Bump Angular to 22.1.3 and OptimusUI to 1.0.1 ([62d7bc4](https://github.com/PascaleBeier/hitkeep/commit/62d7bc424d0c3054d6a00d74033797a120e77dda))
* **deps:** Bump go dependencies ([ec79004](https://github.com/PascaleBeier/hitkeep/commit/ec790047f1bc711db79a1e2f4db3b5da1adb90f4))
* **reports:** Improve mailer observability ([046744e](https://github.com/PascaleBeier/hitkeep/commit/046744e43d5b152c39bae2daa0cb23a72662dcb1))

## [2.13.3](https://github.com/PascaleBeier/hitkeep/compare/v2.13.2...v2.13.3) (2026-08-02)


### Bug Fixes

* **deps:** Bump go deps ([3295352](https://github.com/PascaleBeier/hitkeep/commit/3295352bd97bea7cecd71f7d0f978cc2c40f7a0f))
* Duplicated AI dashboard integration ([6220359](https://github.com/PascaleBeier/hitkeep/commit/6220359cc7e882fca78cb5fed8531fdeaa3fa92c))
* make hk developer tooling self-contained ([9154e56](https://github.com/PascaleBeier/hitkeep/commit/9154e5668aa4674f4fe7e807a55aa816a8ca04b5))

## [2.13.2](https://github.com/PascaleBeier/hitkeep/compare/v2.13.1...v2.13.2) (2026-07-30)


### Bug Fixes

* bump angular to v22.1 ([8982629](https://github.com/PascaleBeier/hitkeep/commit/8982629f199fd655172a8a526a76b092c5503d67))
* **search-console:** Drop extra indices ([e48f6de](https://github.com/PascaleBeier/hitkeep/commit/e48f6de58a4eea3ba73e923c94054f97be7ac019))
* **search-console:** Optimize Search Console memory usage, resolves [#277](https://github.com/PascaleBeier/hitkeep/issues/277) ([b133acb](https://github.com/PascaleBeier/hitkeep/commit/b133acb7877854ec0b7b04eaf255233fdfb77e16))

## [2.13.1](https://github.com/PascaleBeier/hitkeep/compare/v2.13.0...v2.13.1) (2026-07-30)


### Bug Fixes

* **dashboard:** keep brand navigation in SPA ([a9f57d4](https://github.com/PascaleBeier/hitkeep/commit/a9f57d46dafa9f23a5c315d75acf24d7d2c5145b))
* **deps:** Bump go deps ([6a0eae1](https://github.com/PascaleBeier/hitkeep/commit/6a0eae14d3f4f47dabd3c0b8907cf2388d14d259))
* **frontend:** unify favicon fallbacks ([56e09ca](https://github.com/PascaleBeier/hitkeep/commit/56e09ca7371dbec7188c487fa776c3fd5288f7bc))
* **search-console:** Improve search console observability and merge into app design tokens, references [#277](https://github.com/PascaleBeier/hitkeep/issues/277) ([1abc7ae](https://github.com/PascaleBeier/hitkeep/commit/1abc7ae9eedb0a012e66adc9444848d70edbb123))
* **search-console:** persist error messages ([fa5c4aa](https://github.com/PascaleBeier/hitkeep/commit/fa5c4aa685d3939e2e82dd9093aa0d8ca1a94c4c))
* Simplify GSC audit log ([2aa73c1](https://github.com/PascaleBeier/hitkeep/commit/2aa73c1c08452181b8cfa1e004471701b99a1855))

## [2.13.0](https://github.com/PascaleBeier/hitkeep/compare/v2.12.0...v2.13.0) (2026-07-29)


### Features

* **ai:** embedded multi-source AI agent master list and classification pipeline ([c57b5f1](https://github.com/PascaleBeier/hitkeep/commit/c57b5f1a66d4f4386fed9d48223c7cb8f16e144c))
* **api:** unified AI activity report, setup state, and AI analytics surface ([651b7d8](https://github.com/PascaleBeier/hitkeep/commit/651b7d84783849af1398201a36fc72bb1a9f2780))
* **auth:** add social sign-in ([e67252a](https://github.com/PascaleBeier/hitkeep/commit/e67252a332b7709e5bb8a2fb03b1571efb7ae88d))
* automate release metadata and configuration docs ([1612f78](https://github.com/PascaleBeier/hitkeep/commit/1612f7883e04ee3414d797b2aaf37fd8c71fb65d))
* **dashboard:** install-method switcher for the site tracking tab ([e4af0bc](https://github.com/PascaleBeier/hitkeep/commit/e4af0bcdc8eea95902461c1934acf6c95b88c00d))
* **dashboard:** unified AI Agents page, dashboard AI cards, and setup callouts ([75bea8a](https://github.com/PascaleBeier/hitkeep/commit/75bea8a0f1114f0959d5ff6a94d368a8d8f16c59))
* **database:** split default tenant data plane ([#272](https://github.com/PascaleBeier/hitkeep/issues/272)) ([1a4f49c](https://github.com/PascaleBeier/hitkeep/commit/1a4f49c254f88587ffe00168e7ba49bbcb2fb73b)), closes [#259](https://github.com/PascaleBeier/hitkeep/issues/259) [#236](https://github.com/PascaleBeier/hitkeep/issues/236)
* **dev:** add unified developer platform ([#252](https://github.com/PascaleBeier/hitkeep/issues/252)) ([96cba3e](https://github.com/PascaleBeier/hitkeep/commit/96cba3e084fdf2ae1d5641be2139e3a063f9181c))
* **dev:** centralize MCP and local auth setup ([7cbc4a0](https://github.com/PascaleBeier/hitkeep/commit/7cbc4a02c39f69d745f1a94ec820767c227ed351))
* **goals:** filter custom events in traffic records ([50071d9](https://github.com/PascaleBeier/hitkeep/commit/50071d91d5cd23fe7d10e9d738fe95e7746e8bc8)), closes [#275](https://github.com/PascaleBeier/hitkeep/issues/275)
* **reports:** add configurable scheduled reporting ([4ee9546](https://github.com/PascaleBeier/hitkeep/commit/4ee9546aca267f0466064c375a90d5a1665f6733))
* **reports:** consolidate scheduled reporting ([cdd3ade](https://github.com/PascaleBeier/hitkeep/commit/cdd3adeebae70b056e72f0052ea5f3229189af7e))
* **security:** migrate Spamhaus feeds to JSON ([#256](https://github.com/PascaleBeier/hitkeep/issues/256)) ([f385131](https://github.com/PascaleBeier/hitkeep/commit/f38513115745e67b78c418ce2a489f2d0697897f))
* **tracker:** publish @hitkeep/tracker npm package ([#269](https://github.com/PascaleBeier/hitkeep/issues/269)) ([7227d5d](https://github.com/PascaleBeier/hitkeep/commit/7227d5d9abaea614e5855a098c486728f6941f66))
* Unify design tokens ([f1cf602](https://github.com/PascaleBeier/hitkeep/commit/f1cf602c21f9225348a04246b6ea0c5f910eb7ac))
* unify traffic controls and operational tooling ([a594349](https://github.com/PascaleBeier/hitkeep/commit/a594349b51a0989026b4ed2a74ae513f05f6c461))


### Bug Fixes

* add valid Docker build argument defaults ([35d90f3](https://github.com/PascaleBeier/hitkeep/commit/35d90f3966e69b2112329b13324bbe9047d6bd68))
* **auth:** explain session endings on the login screen ([a23ab78](https://github.com/PascaleBeier/hitkeep/commit/a23ab788e73b1dcce583fb514f1f9e4099c5e196))
* Bump deps ([8d20b91](https://github.com/PascaleBeier/hitkeep/commit/8d20b9154e264fe341b07f2b56e42045dc12c6a8))
* **ci:** bootstrap canonical npm before caching ([091eef5](https://github.com/PascaleBeier/hitkeep/commit/091eef5b1408fe76ef8f0c490c9c2767e8685dbf))
* **database:** add automatic recovery and checkpoints ([0129d2c](https://github.com/PascaleBeier/hitkeep/commit/0129d2c7ed406c64d2868a626b9c7a7f69ca260c))
* **database:** Add migration guard ([d801cf0](https://github.com/PascaleBeier/hitkeep/commit/d801cf0602acde2019a12918876b6998d03f5820))
* **database:** include migration guard implementation ([79af19b](https://github.com/PascaleBeier/hitkeep/commit/79af19b8a1959bd3abda309f9470f7b46c1f523f))
* **database:** Mitigate corrupted indexes ([de8e979](https://github.com/PascaleBeier/hitkeep/commit/de8e979bd732832bcb19518aa08278c8dfa69c09))
* **database:** serialize social confirmation consumption ([8e5b1a6](https://github.com/PascaleBeier/hitkeep/commit/8e5b1a677670e1f0603fa0fd388c9bf6f237a1b7))
* **deps:** Bump frontend deps ([bee6bac](https://github.com/PascaleBeier/hitkeep/commit/bee6bac7c684c8303090088581dbea36eb2c0af3))
* **deps:** Bump go dependencies ([e450e91](https://github.com/PascaleBeier/hitkeep/commit/e450e917cea555b5f5e80557a9f47a4a7b71d971))
* **deps:** Bump go deps ([a6995f0](https://github.com/PascaleBeier/hitkeep/commit/a6995f023c3736d0e5a2433873f9ae2c87bdf61c))
* **deps:** resolve frontend audit findings ([4c14315](https://github.com/PascaleBeier/hitkeep/commit/4c143156e45b7a35aef595ab20b48abe7626b3b2))
* **dev:** preserve forwarded progress notifications ([794ee83](https://github.com/PascaleBeier/hitkeep/commit/794ee835d0dc53a7dc49be145944172b6d5b2357))
* **devtool:** serve object-form MCP tool schemas ([549803d](https://github.com/PascaleBeier/hitkeep/commit/549803d29fa668a5bedc6a15d882fcbe9f913d6f))
* **frontend:** Migrate to OptimusUI ([6b09350](https://github.com/PascaleBeier/hitkeep/commit/6b09350ca8ad43b282781727801088521814907f))
* **frontend:** Migrate to OptimusUI ([ab7d114](https://github.com/PascaleBeier/hitkeep/commit/ab7d114d2bcff1b0c6f9c6da9fcb7938a5007b05))
* **frontend:** provide recoverable application failure states ([5e11d82](https://github.com/PascaleBeier/hitkeep/commit/5e11d827a73f2e2fe1ff61e4336306401abfd095))
* **frontend:** update web vitals metric fixture ([d1532f3](https://github.com/PascaleBeier/hitkeep/commit/d1532f3c2894aa06c150ff205e0b2707fd701480))
* **i18n:** Move handrolled component translations to OptimusUI ([3f2b5b7](https://github.com/PascaleBeier/hitkeep/commit/3f2b5b7eb0c7912d0a5d9ea73b2c9846f27855ef))
* **tracker:** defer analytics during prerender ([f6e7bc8](https://github.com/PascaleBeier/hitkeep/commit/f6e7bc82355aebd03d082ffdebf2eab9c72743b6)), closes [#271](https://github.com/PascaleBeier/hitkeep/issues/271)


### Performance Improvements

* **ipmeta:** add block-addressable lookup assets ([4f43357](https://github.com/PascaleBeier/hitkeep/commit/4f4335732d2e7bae88c0dbbcd504d7ee9c8a081f))

## [2.12.0](https://github.com/PascaleBeier/hitkeep/compare/v2.11.0...v2.12.0) (2026-07-14)


### Features

* **auth:** add team OIDC SSO ([b48f42e](https://github.com/PascaleBeier/hitkeep/commit/b48f42e5d0cbb253ca484256e0ef4756c9a8b43c))
* **auth:** streamline SSO login and invitation UX ([2a5a981](https://github.com/PascaleBeier/hitkeep/commit/2a5a98150fac6945fb5f51c1979d50a911caa188)), closes [#225](https://github.com/PascaleBeier/hitkeep/issues/225) [#226](https://github.com/PascaleBeier/hitkeep/issues/226)
* **billing:** add annual conversion flow ([#251](https://github.com/PascaleBeier/hitkeep/issues/251)) ([d1c2e3c](https://github.com/PascaleBeier/hitkeep/commit/d1c2e3ce960d51149b089b18ffd624787ebfe849))
* **frontend:** route site settings and invitations ([4c0a0a2](https://github.com/PascaleBeier/hitkeep/commit/4c0a0a257fe5ca72e084c5b021a2c1833fff54cd)), closes [#246](https://github.com/PascaleBeier/hitkeep/issues/246) [#247](https://github.com/PascaleBeier/hitkeep/issues/247)
* **sso:** authorize invite-aware OIDC provisioning ([d5270f6](https://github.com/PascaleBeier/hitkeep/commit/d5270f69054b5330078632ef3bbea755bbbab721)), closes [#225](https://github.com/PascaleBeier/hitkeep/issues/225) [#226](https://github.com/PascaleBeier/hitkeep/issues/226)


### Bug Fixes

* **database:** preserve QR data during site changes ([24a5765](https://github.com/PascaleBeier/hitkeep/commit/24a5765284e8acac947d5d3aa52375d662084def)), closes [#248](https://github.com/PascaleBeier/hitkeep/issues/248)
* **security:** use standard cross-origin protection ([3daa095](https://github.com/PascaleBeier/hitkeep/commit/3daa0955111446f7cfd4b1144904bfec3dd33ada))

## [2.11.0](https://github.com/PascaleBeier/hitkeep/compare/v2.10.3...v2.11.0) (2026-07-11)


### Features

* implement outbound webhooks ([c64175e](https://github.com/PascaleBeier/hitkeep/commit/c64175e41c05f0544e09fedba9f92494c87ec4f7))

## [2.10.3](https://github.com/PascaleBeier/hitkeep/compare/v2.10.2...v2.10.3) (2026-07-10)


### Bug Fixes

* **ci:** set test AWS environment ([9ecaf6f](https://github.com/PascaleBeier/hitkeep/commit/9ecaf6fa721a1cffc0a31622b9fbc3260ac2ac74))
* **frontend:** Update Onboarding Dashboard from Realtime updates, closes [#240](https://github.com/PascaleBeier/hitkeep/issues/240) ([92963b9](https://github.com/PascaleBeier/hitkeep/commit/92963b99ee015c60050597d960f5f4f4148b2b27))
* **s3:** missing AWS extension ([a31c50e](https://github.com/PascaleBeier/hitkeep/commit/a31c50e073979ebea12f41fdd36b66a35595355e))

## [2.10.2](https://github.com/PascaleBeier/hitkeep/compare/v2.10.1...v2.10.2) (2026-07-09)


### Bug Fixes

* **cloud:** resolve plan changes from Stripe price and lapsed statuses ([98881f7](https://github.com/PascaleBeier/hitkeep/commit/98881f70cee82bc1c8c15b5b7d234be346552a79))
* **cloud:** warn free teams by email before retention first trims data ([8cf5915](https://github.com/PascaleBeier/hitkeep/commit/8cf5915a14eac8241e317fcf99d48e80e563ad25))
* **database:** compact fragmented database files on startup and tenant open ([9240341](https://github.com/PascaleBeier/hitkeep/commit/9240341f2726c43d72f0b065dfba3665d7715c72))
* **database:** derive site and tenant cleanup plans from schema constraints ([9e194b8](https://github.com/PascaleBeier/hitkeep/commit/9e194b8aa83aa4ba12710f5c4b087c351e168c5b))
* **database:** disable implicit duckdb extension fetching ([9869698](https://github.com/PascaleBeier/hitkeep/commit/9869698b96a5a0a0f2b832f46b6ffb1dda0c7786))
* **database:** stop preserving insertion order to cut query memory ([2c59dba](https://github.com/PascaleBeier/hitkeep/commit/2c59dba108c3628e6e4d19749428774c62b86b29))
* **docs:** document database scope-column rules for contributors ([03ae0b6](https://github.com/PascaleBeier/hitkeep/commit/03ae0b6b4ec8137d06e3838a7def56e6d9885219))
* **e2e:** pin ecommerce dashboard specs to the 30d range ([09b5232](https://github.com/PascaleBeier/hitkeep/commit/09b5232670be4d45b8ea61c84fa96236230fd491))
* **e2e:** pin remaining seeded analytics specs to the 30d range ([d4449d2](https://github.com/PascaleBeier/hitkeep/commit/d4449d20e41ad1a80cd7163c3d7ad8ebcf4d5ab3))
* **frontend:** show retention hint when a range exceeds the plan window ([edb66fd](https://github.com/PascaleBeier/hitkeep/commit/edb66fd2f63a88d8e9107f22409dd95ed0d26820))
* **sites:** let site admins and owners rename a site domain ([727e5ec](https://github.com/PascaleBeier/hitkeep/commit/727e5ec96ed31807f5f5e1a96e415cc9833f8cf3))
* staticcheck ([6891d6f](https://github.com/PascaleBeier/hitkeep/commit/6891d6fec2d8b7a6f86d24a77eff8e4baa7e7e7a))


### Performance Improvements

* **database:** batch per-hit activity and rollup writes per ingest flush ([ab0b315](https://github.com/PascaleBeier/hitkeep/commit/ab0b3152c72b0a6c62ad00bf2533f4b9115fde07))
* **database:** cache site tenant and domain lookups on the ingest path ([0b9ecdb](https://github.com/PascaleBeier/hitkeep/commit/0b9ecdb1e9af079ddf671aca5efdc94324a99111))
* **frontend:** Preload dashboard routes ([0997660](https://github.com/PascaleBeier/hitkeep/commit/0997660143d7b45c72c194a144a8e2de7d0ca602))
* **ingest:** tune nsq consumer delivery and wait for embedded nsqd readiness ([b361f1a](https://github.com/PascaleBeier/hitkeep/commit/b361f1a5847f16fb46155ef11cb4bab9194999e6))

## [2.10.1](https://github.com/PascaleBeier/hitkeep/compare/v2.10.0...v2.10.1) (2026-07-08)


### Bug Fixes

* **admin:** add DuckDB memory breakdown and copyable system report ([db0588d](https://github.com/PascaleBeier/hitkeep/commit/db0588d9fe9ac8c96ba3d5acc50ae855e50d65ad))
* **admin:** polish the system console ([0c19df7](https://github.com/PascaleBeier/hitkeep/commit/0c19df7e941c6fe07fd2c25b3664c4dbbbe2f300))
* **db:** make deprecated-driver-call suppressions work for both linters ([2396af0](https://github.com/PascaleBeier/hitkeep/commit/2396af00fdb3b6d94fb5fcbf7127c1bcf7fe8ee7))
* **db:** self-heal after a fatal DuckDB invalidation ([b1719ea](https://github.com/PascaleBeier/hitkeep/commit/b1719ea81e45bb44d82b958af749f207abf064e9))


### Performance Improvements

* **db:** bound DuckDB memory and drop analytics ART indexes ([8166a78](https://github.com/PascaleBeier/hitkeep/commit/8166a786e2bf24da26a78eeb6c7b4c60ebd533a8)), closes [#236](https://github.com/PascaleBeier/hitkeep/issues/236)

## [2.10.0](https://github.com/PascaleBeier/hitkeep/compare/v2.9.1...v2.10.0) (2026-07-07)


### Features

* **cloud:** add free-plan upgrade nudges ([d231acd](https://github.com/PascaleBeier/hitkeep/commit/d231acdebe49e3107d37033a7e4b9d69a743b203)), closes [#232](https://github.com/PascaleBeier/hitkeep/issues/232)
* **cloud:** require Pro for managed custom domains ([df119d6](https://github.com/PascaleBeier/hitkeep/commit/df119d6e7dd5dd4cd0a68be4885eff9a63fa40c7)), closes [#232](https://github.com/PascaleBeier/hitkeep/issues/232)
* **frontend:** manage custom domains with DNS setup ([2a51b18](https://github.com/PascaleBeier/hitkeep/commit/2a51b1875597076c1561b1abc81fa9148a5cd13b)), closes [#232](https://github.com/PascaleBeier/hitkeep/issues/232)
* **frontend:** remember chart style and site sort preferences ([b448632](https://github.com/PascaleBeier/hitkeep/commit/b4486323e0c6f8f58a04ac3ed2075ed8615fcc77)), closes [#232](https://github.com/PascaleBeier/hitkeep/issues/232)
* **frontend:** route team admin sections as subpages ([581935e](https://github.com/PascaleBeier/hitkeep/commit/581935edd76964b31734ffe18dfb7004520512f4)), closes [#232](https://github.com/PascaleBeier/hitkeep/issues/232)
* **tracker:** add custom tracking domains ([cfd8be7](https://github.com/PascaleBeier/hitkeep/commit/cfd8be7a5b7651f7e2e471360fe577e48faf91e2)), closes [#232](https://github.com/PascaleBeier/hitkeep/issues/232)


### Bug Fixes

* **frontend:** align search icons with OptimusUI fields ([dd0901f](https://github.com/PascaleBeier/hitkeep/commit/dd0901f58d1bf377c69f9680e2e06ac3d25f8db2)), closes [#232](https://github.com/PascaleBeier/hitkeep/issues/232)

## [2.9.1](https://github.com/PascaleBeier/hitkeep/compare/v2.9.0...v2.9.1) (2026-07-06)


### Bug Fixes

* **frontend:** persist report ranges ([#230](https://github.com/PascaleBeier/hitkeep/issues/230)) ([1ee335d](https://github.com/PascaleBeier/hitkeep/commit/1ee335da5fab6f2d01f1484380356da5b8a5bbea)), closes [#228](https://github.com/PascaleBeier/hitkeep/issues/228)
* use descriptive dashboard browser titles ([0dc1459](https://github.com/PascaleBeier/hitkeep/commit/0dc14594a55f33761d15ef3df834aba310e1f0af)), closes [#229](https://github.com/PascaleBeier/hitkeep/issues/229)

## [2.9.0](https://github.com/PascaleBeier/hitkeep/compare/v2.8.1...v2.9.0) (2026-07-04)


### Features

* **ai:** add ask ai dashboard assistant ([d45b426](https://github.com/PascaleBeier/hitkeep/commit/d45b426202dc3209cecc87275e3fe6c6b540b9be))
* **analytics:** add sites overview API and UI with lightweight metrics and chart visualizations. ([02f9d0e](https://github.com/PascaleBeier/hitkeep/commit/02f9d0e3c41203ae67309d9072783e53515e5e4b)), closes [#215](https://github.com/PascaleBeier/hitkeep/issues/215)
* **analytics:** add today report range presets ([0e4a300](https://github.com/PascaleBeier/hitkeep/commit/0e4a300adf4592f2dadf7d795131737fb6763641))
* **cloud:** expose hosted integration discovery ([4e22779](https://github.com/PascaleBeier/hitkeep/commit/4e227797a016fcb013790030e0e2270c542947e9))
* **cloud:** move region choice into signup ([c4b4a54](https://github.com/PascaleBeier/hitkeep/commit/c4b4a54e0044e7a3b2f2291bfb141f7d4149e8ad))
* **frontend:** link brand to app root ([c5e26c5](https://github.com/PascaleBeier/hitkeep/commit/c5e26c534a9c662f2f8507f507a0a919b625542e)), closes [#214](https://github.com/PascaleBeier/hitkeep/issues/214)
* **frontend:** make range and language selectors searchable ([20bfb38](https://github.com/PascaleBeier/hitkeep/commit/20bfb387b932860895df949af6ffd99f267b7d49))
* **mail:** add legal and social footer links ([b084815](https://github.com/PascaleBeier/hitkeep/commit/b084815d57a27475abbd0a1926d123237a87dfc4))
* **sites:** add owner-only stats reset ([39fabc7](https://github.com/PascaleBeier/hitkeep/commit/39fabc7de3d9efe22ae99e6034d780fa67bfea0e)), closes [#216](https://github.com/PascaleBeier/hitkeep/issues/216)


### Bug Fixes

* **auth:** accept invite links through login flow ([a26f121](https://github.com/PascaleBeier/hitkeep/commit/a26f1219c75a52a765d6ca84f6b5b68274b81a0f)), closes [#221](https://github.com/PascaleBeier/hitkeep/issues/221)
* **frontend:** expose QR navigation in share mode ([25a3d82](https://github.com/PascaleBeier/hitkeep/commit/25a3d82c7ac3bc75fcead3ee7cad6e2e94ff91d2))
* **frontend:** replace favicon and primary logo with optimized svg ([81ce3e9](https://github.com/PascaleBeier/hitkeep/commit/81ce3e9bb90c144a9c098dc4c528fbb412cc4112))
* **frontend:** Sidebar overflow for large sites names (again) ([19feab8](https://github.com/PascaleBeier/hitkeep/commit/19feab8f767b5c3365d994cbe5f5836b15f6274f))
* **helm:** align chart with 2.8.1 ([84f1896](https://github.com/PascaleBeier/hitkeep/commit/84f1896da74e77d5a573edf40e2cf625c18c99f6))
* **seed:** show AI visibility data for today ([3c03270](https://github.com/PascaleBeier/hitkeep/commit/3c032709434aac1db7f2cfa320ed6b304cc79326))
* **seed:** show UTM and QR demo data today ([97e470c](https://github.com/PascaleBeier/hitkeep/commit/97e470cfe8506520395d1725944351f4b1633fdb))
* **ui:** keep language selector overlay above settings card ([3f9a4ff](https://github.com/PascaleBeier/hitkeep/commit/3f9a4ff8788fdbcebcd0fbaa2a922d78f8c73004))

## [2.8.1](https://github.com/PascaleBeier/hitkeep/compare/v2.8.0...v2.8.1) (2026-06-25)


### Bug Fixes

* **frontend:** resolve password settings validation, breadcrumbs scrollbar, and GSC timezone tests ([c09f293](https://github.com/PascaleBeier/hitkeep/commit/c09f2934d3eec75ef68984ea49e92243693ff958))
* **i18n:** update Portuguese translations and add missing QR code labels ([c9af959](https://github.com/PascaleBeier/hitkeep/commit/c9af95946f86163cb8c1caca7cefbfbf8aa1a0a4))

## [2.8.0](https://github.com/PascaleBeier/hitkeep/compare/v2.7.0...v2.8.0) (2026-06-24)


### Features

* **admin:** surface MCP runtime readiness ([41680e6](https://github.com/PascaleBeier/hitkeep/commit/41680e6130aa68bcd144b464bda458484fd2d752))
* **ai:** Add HitKeep Skills ([d391e94](https://github.com/PascaleBeier/hitkeep/commit/d391e94b81d57865f3b1a94e8dd0af60605144fa))
* **frontend:** add QR campaign dashboard ([f176ae0](https://github.com/PascaleBeier/hitkeep/commit/f176ae0913b919f04b293c83191f6041eb67e546))
* **i18n:** add Brazilian Portuguese localization ([c003387](https://github.com/PascaleBeier/hitkeep/commit/c003387344e0cb795162a611bbf32b2352eea198))
* **qr:** add dynamic campaign tracking APIs ([8d54d4a](https://github.com/PascaleBeier/hitkeep/commit/8d54d4ab054c876606961dc9e78788166a52ea07))
* **seed:** add QR campaign demo data ([cc7b533](https://github.com/PascaleBeier/hitkeep/commit/cc7b5332c6957da88bdfc26cf56ef53cd57ede6c))


### Bug Fixes

* **ci:** refresh github action pins ([bc86772](https://github.com/PascaleBeier/hitkeep/commit/bc86772762c9f0aa7eeb1b4b385f85a502716b5f))
* **deps:** Bump duckdb to v2.10504.0 to fix ptr race cond ([00f674f](https://github.com/PascaleBeier/hitkeep/commit/00f674ff856ccb7f093fdf0030a0cbc9318cfe4d))
* **frontend:** clarify admin user role display ([e8790f9](https://github.com/PascaleBeier/hitkeep/commit/e8790f9accb4843469b4398d25ad3bfd96038c77))
* **i18n:** localize default team setup explicitly ([96ea994](https://github.com/PascaleBeier/hitkeep/commit/96ea99467741ae1c7f74351df80cd5a18f53abb8))
* **release:** sync version metadata with release-please ([50690e1](https://github.com/PascaleBeier/hitkeep/commit/50690e1233e669b46dc5fdd3dc5ed42faf841dae))

## [2.7.0](https://github.com/PascaleBeier/hitkeep/compare/v2.6.1...v2.7.0) (2026-06-05)


### Features

* **dashboard:** refresh analytics in realtime ([f05d58d](https://github.com/PascaleBeier/hitkeep/commit/f05d58d3ece5836f616d6b377d3c2e8b86b63f52))


### Bug Fixes

* **api:** allow AI fetch ingest without browser metadata ([59df19d](https://github.com/PascaleBeier/hitkeep/commit/59df19dd646d396598b707f2f445666b057b424d)), closes [#197](https://github.com/PascaleBeier/hitkeep/issues/197)
* **cloud:** let operators manage unlimited own resources ([4b84ee9](https://github.com/PascaleBeier/hitkeep/commit/4b84ee96a3cf162c4599d4eb98f8f02cbaf1eb3a))
* **deps:** update Go toolchain to 1.26.4 ([34271b4](https://github.com/PascaleBeier/hitkeep/commit/34271b4f5de80aa9fa8eb5303eeda6733fbf3598))

## [2.6.1](https://github.com/PascaleBeier/hitkeep/compare/v2.6.0...v2.6.1) (2026-06-01)


### Bug Fixes

* **analytics:** store web vitals metric ids for dedup ([af24874](https://github.com/PascaleBeier/hitkeep/commit/af248743b7597b530eda50b8b4628490257345f4))
* **db:** pin duckdb-go race-safe release ([45f3a6e](https://github.com/PascaleBeier/hitkeep/commit/45f3a6e5505e5eeb3967f450767bfa3a2d4b6f18))
* **deps:** update frontend dependencies ([df6b7b5](https://github.com/PascaleBeier/hitkeep/commit/df6b7b57e44855c415ccfe7f78831877800e4280))
* **deps:** update Go module dependencies ([bf28c30](https://github.com/PascaleBeier/hitkeep/commit/bf28c30fbc8a4c0d62357c2b059061447499493d))
* **frontend:** introduce reusable site selects for API clients and settings ([f0b01e2](https://github.com/PascaleBeier/hitkeep/commit/f0b01e24806cb2a66c9c960c0f3d15530a8c0c2c))
* **ipmeta:** isolate token download test ([34b604e](https://github.com/PascaleBeier/hitkeep/commit/34b604e3ff76af1164bd3f69cc93867f50933744))

## [2.6.0](https://github.com/PascaleBeier/hitkeep/compare/v2.5.1...v2.6.0) (2026-05-18)


### Features

* **analytics:** add geo network evidence to opportunities ([3c8e3fb](https://github.com/PascaleBeier/hitkeep/commit/3c8e3fb826d097e92f9452a44ffee95910ebfa95))
* **analytics:** embed IP2Location city and network metadata ([8c9f458](https://github.com/PascaleBeier/hitkeep/commit/8c9f458770fb425b4007728643da6c2ec38c01e2))
* **analytics:** expose geo network dashboard aggregates ([0e24914](https://github.com/PascaleBeier/hitkeep/commit/0e24914236689467ddf39d859cf5ddbbbec8edbb))
* **analytics:** persist geo network metadata for hits ([87b1b5e](https://github.com/PascaleBeier/hitkeep/commit/87b1b5eddd36dd8b23abe787536f1d5991cb79f5))
* **api:** add country traffic exclusions ([633a74b](https://github.com/PascaleBeier/hitkeep/commit/633a74b9082fbba8414d00d270549e444aec8045))
* **frontend:** add country exclusion controls ([3798a45](https://github.com/PascaleBeier/hitkeep/commit/3798a45d65f2fdeda88dea08eddb711ea1aad14c))
* **frontend:** standardize tabbed analytics breakdown cards ([06fd470](https://github.com/PascaleBeier/hitkeep/commit/06fd470e655681305ca751d497fa7adfe7d9905e))
* **mcp:** expose geo network analytics filters ([5c5d5b9](https://github.com/PascaleBeier/hitkeep/commit/5c5d5b94dd8e1c71b1c1505d27e30d322574e7bc))


### Bug Fixes

* **analytics:** preserve geo network metadata in lifecycle workflows ([cf1cd4d](https://github.com/PascaleBeier/hitkeep/commit/cf1cd4dcfbd03e98eb87e49e8659825136115e03))
* **frontend:** standardize admin and settings tables ([aa8b999](https://github.com/PascaleBeier/hitkeep/commit/aa8b9994952d5c7e59147d8084625e38aaf99cad))
* **teams:** delete empty hosted cloud teams ([f650b9b](https://github.com/PascaleBeier/hitkeep/commit/f650b9b39b7f139a9251d80ac92b53293d7b3a5b))

## [2.5.1](https://github.com/PascaleBeier/hitkeep/compare/v2.5.0...v2.5.1) (2026-05-16)


### Bug Fixes

* **cloud:** age out stale AI provider warnings ([bffa3bd](https://github.com/PascaleBeier/hitkeep/commit/bffa3bd39c960bd7648d2c90b4916855ab2aa5c5))
* **cloud:** harden hosted team membership flows ([ab96a05](https://github.com/PascaleBeier/hitkeep/commit/ab96a0526e0558b730054d968953a1b0eeb74a11))
* **share:** show Web Vitals in shared dashboards ([ea2c783](https://github.com/PascaleBeier/hitkeep/commit/ea2c783de2f67f208ef0fb9a7144c35cba7a2d7c))

## [2.5.0](https://github.com/PascaleBeier/hitkeep/compare/v2.4.2...v2.5.0) (2026-05-15)


### Features

* **analytics:** add web vitals ingest and reporting APIs ([0cecad9](https://github.com/PascaleBeier/hitkeep/commit/0cecad96dcfc40b71bd187bca82e5c850c50e29f))
* **frontend:** add web vitals dashboard ([65a8f65](https://github.com/PascaleBeier/hitkeep/commit/65a8f65f2a33be6ef8877b659694f94a55e1a247))
* **mcp:** expose web vitals aggregates ([a590fc4](https://github.com/PascaleBeier/hitkeep/commit/a590fc4f4eac0a8464adb8b4353f1d47428f5d55))
* **opportunities:** surface web vitals evidence ([a00c029](https://github.com/PascaleBeier/hitkeep/commit/a00c02979f68c9150e5bc677ae52d70989319bb0))
* **seed:** add web vitals demo data ([e26f68c](https://github.com/PascaleBeier/hitkeep/commit/e26f68c69c8a8f7290d5069b97986afff10af9e9))
* **tracker:** add opt-in web vitals bundle ([d86a063](https://github.com/PascaleBeier/hitkeep/commit/d86a0639fe7fca573abb410a74819a9c8976d36f))


### Bug Fixes

* **ai:** defer provider credentials to goAI ([b5d83b4](https://github.com/PascaleBeier/hitkeep/commit/b5d83b4c14d8bea9d19b4108706b7b9c6e54c801))
* **ci:** build snapshots from pull request merge commits ([aff346c](https://github.com/PascaleBeier/hitkeep/commit/aff346c2e6a645e750a28b375bd4991e92919ea0))
* **deployment:** support subdirectory installs ([c43107d](https://github.com/PascaleBeier/hitkeep/commit/c43107de2262c29d8ba2aeeea88b8e2ea82e968f))
* **frontend:** stabilize funnel and goal management UX ([4b8df8b](https://github.com/PascaleBeier/hitkeep/commit/4b8df8ba22a7c6dedb13f94ae97e068d2000c822))
* **mailer:** render RFC-compliant signature delimiter ([#170](https://github.com/PascaleBeier/hitkeep/issues/170)) ([0067658](https://github.com/PascaleBeier/hitkeep/commit/0067658e7b4457e1d9fdf8112b2cb5251f3c6988)), closes [#169](https://github.com/PascaleBeier/hitkeep/issues/169)

## [2.4.2](https://github.com/PascaleBeier/hitkeep/compare/v2.4.1...v2.4.2) (2026-05-08)


### Bug Fixes

* **frontend:** keep site export format menus clickable ([3a0f453](https://github.com/PascaleBeier/hitkeep/commit/3a0f45366b7d123a0786d774069c2818ac6bb241))

## [2.4.1](https://github.com/PascaleBeier/hitkeep/compare/v2.4.0...v2.4.1) (2026-05-08)


### Bug Fixes

* **backend:** Fix mcp server request host validation, fixes [#148](https://github.com/PascaleBeier/hitkeep/issues/148) ([af94bfe](https://github.com/PascaleBeier/hitkeep/commit/af94bfe6b5760b0df8f1f9bbcecc6fff077ecb03))
* **frontend:** Bump Angular packages ([3cbd329](https://github.com/PascaleBeier/hitkeep/commit/3cbd3295ed3d54127d140c3d98861a485c1d9665))
* **security:** update Go to 1.26.3 ([d14446b](https://github.com/PascaleBeier/hitkeep/commit/d14446befccce3ed84f5f92a817d61bcecc9682e)), closes [#150](https://github.com/PascaleBeier/hitkeep/issues/150)

## [2.4.0](https://github.com/PascaleBeier/hitkeep/compare/v2.3.1...v2.4.0) (2026-05-06)


### Features

* Add Google Search Console integration ([5e5c891](https://github.com/PascaleBeier/hitkeep/commit/5e5c8918c336872a826315cbb50bc5f97225bf77)), closes [#139](https://github.com/PascaleBeier/hitkeep/issues/139)
* **backend:** Add dutch language to mails ([e837c95](https://github.com/PascaleBeier/hitkeep/commit/e837c95045dda12bf89bf51a777335dacf7e7648))
* **backend:** Expose search console tools to MCP ([5b84185](https://github.com/PascaleBeier/hitkeep/commit/5b8418525dd8d3d51e53dc5f49c5226d10a6e3e1))
* **frontend:** Add dutch language ([4358497](https://github.com/PascaleBeier/hitkeep/commit/435849722a937f1ba20cd2af1dfbb66c09b97f4e))
* **imports:** add Plausible and Simple Analytics imports ([b714a39](https://github.com/PascaleBeier/hitkeep/commit/b714a394bb20ab04b7c2cde70b7fecac0f861fc4))
* **ingest:** add server-side ingestion ([66cd78d](https://github.com/PascaleBeier/hitkeep/commit/66cd78d8f51f6d0252847618a9be99b54b72c499)), closes [#129](https://github.com/PascaleBeier/hitkeep/issues/129)


### Bug Fixes

* **backend:** Optimize Healthckeck endpoint ([7617ba7](https://github.com/PascaleBeier/hitkeep/commit/7617ba74a134cbc46628db17ecca8dd38b2e446f))
* Consolidate frontend and auth bootstrap, resolving [#137](https://github.com/PascaleBeier/hitkeep/issues/137) ([9155cc1](https://github.com/PascaleBeier/hitkeep/commit/9155cc1bb35c431be34533380172ac9c92bf9c8e))
* **deps:** Bump iploc for may ([6de9693](https://github.com/PascaleBeier/hitkeep/commit/6de9693328e7fd12d291b41a5f1433bbc9836e55))
* **docker:** adjust healthcheck interval to 30s ([d2a45a3](https://github.com/PascaleBeier/hitkeep/commit/d2a45a3db46dcce7a82b573b569820acd4b13392))
* **dx:** Clarify server-side pageview description ([5b2bded](https://github.com/PascaleBeier/hitkeep/commit/5b2bded23caac8862c0c6a440452a58af5de9097))
* **frontend:** Unify copy actions across frontend and expose copyable team, site, and user IDs ([5d49768](https://github.com/PascaleBeier/hitkeep/commit/5d49768a4dab4548deb5a858ccd4d6e8d47a0baf))
* **ux:** Consolidate import and export ([7a83e17](https://github.com/PascaleBeier/hitkeep/commit/7a83e1742e87e90870cff4fd9f146fc8b77d6471))

## [2.3.1](https://github.com/PascaleBeier/hitkeep/compare/v2.3.0...v2.3.1) (2026-04-30)


### Bug Fixes

* **admin:** surface degraded worker health ([5aaab49](https://github.com/PascaleBeier/hitkeep/commit/5aaab492ff60e580b49a9b31c1c5628d46d9bc4a))
* **cloud:** report backup worker status ([77e046d](https://github.com/PascaleBeier/hitkeep/commit/77e046d15fdc3c1e646218b48b16bae13980dcb6)), closes [#124](https://github.com/PascaleBeier/hitkeep/issues/124)
* **frontend:** link managed cloud status page ([ad2dde7](https://github.com/PascaleBeier/hitkeep/commit/ad2dde791c8705167f198f8f8282dbc6cf82b4e1))
* **ingest:** count spam and rejected traffic ([ffb6de3](https://github.com/PascaleBeier/hitkeep/commit/ffb6de38728b42e7d96493c011ff24db08dfa063)), closes [#125](https://github.com/PascaleBeier/hitkeep/issues/125) [#126](https://github.com/PascaleBeier/hitkeep/issues/126)

## [2.3.0](https://github.com/PascaleBeier/hitkeep/compare/v2.2.1...v2.3.0) (2026-04-29)


### Features

* **activation:** add installation and activation center ([9fc6e74](https://github.com/PascaleBeier/hitkeep/commit/9fc6e74bf3420928f3794b29f93705056e6a91a8))
* Add automatic event ingestion ([d58a281](https://github.com/PascaleBeier/hitkeep/commit/d58a2814e4f326d0d66a5285aa77086ce49cd8c6))
* Add readonly MCP Server to be used with Team oder user api token ([84777b4](https://github.com/PascaleBeier/hitkeep/commit/84777b4b4bf20c7983d6de05d7022612fa343735))
* **admin:** add system status and audit console ([f861347](https://github.com/PascaleBeier/hitkeep/commit/f861347f730e21ac9e07512ece7e89454a1647eb))
* **events:** support multiple dimension filters ([51cf9af](https://github.com/PascaleBeier/hitkeep/commit/51cf9af8db33368b208864fc158dbe9b66d8b6b8))


### Bug Fixes

* **admin:** harden system status APIs ([a0dc422](https://github.com/PascaleBeier/hitkeep/commit/a0dc4220acd901b9c48d661feb8da2468246dfe4))
* **auth:** grant admins scoped site and system access ([7762abb](https://github.com/PascaleBeier/hitkeep/commit/7762abbcc2687620fbdb25deb340a586b15a72d4))
* **cloud:** load cloud config in billing builds ([9503033](https://github.com/PascaleBeier/hitkeep/commit/9503033d266e40651bd80ff8d2745a4268394483))
* **frontend:** polish settings layouts and action feedback ([6413695](https://github.com/PascaleBeier/hitkeep/commit/6413695b42db034131de2f0b69ad75247670ac32))
* **frontend:** show admin status action feedback ([85570a4](https://github.com/PascaleBeier/hitkeep/commit/85570a4e73443cf6011d139680f324fde1267d3d))
* **frontend:** show settings action feedback ([d293562](https://github.com/PascaleBeier/hitkeep/commit/d293562ae372864307a4268cc89d7afa6f9709ec))
* **mailer:** use hostname for SMTP HELO, resolves [#112](https://github.com/PascaleBeier/hitkeep/issues/112) ([00ef55c](https://github.com/PascaleBeier/hitkeep/commit/00ef55c6ba1466f869b06a93792ceef5303e6d0e))

## [2.2.1](https://github.com/PascaleBeier/hitkeep/compare/v2.2.0...v2.2.1) (2026-04-11)


### Bug Fixes

* **frontend:** regenerate dashboard lockfile with npm 11.11 ([2c609a3](https://github.com/PascaleBeier/hitkeep/commit/2c609a35ecd69afa78036d2cf2e01149433fbfae))
* **ingestion:** use explicit column order for batch ingestion with legacy schema, resolves [#99](https://github.com/PascaleBeier/hitkeep/issues/99) ([3c4129f](https://github.com/PascaleBeier/hitkeep/commit/3c4129f60717c8915ba80bf7ad76c2da6976ae60))

## [2.2.0](https://github.com/PascaleBeier/hitkeep/compare/v2.1.0...v2.2.0) (2026-04-09)


### Features

* Allow users to sign via mail for 2fa ([f301d9d](https://github.com/PascaleBeier/hitkeep/commit/f301d9d1008ff49de9d13c8c01ccdf4225782607))
* **backend:** Add AI visibility and AI Chatbot backends ([9e9f83a](https://github.com/PascaleBeier/hitkeep/commit/9e9f83a76eafd0f1a218a2fd1966abfc7d92a79a))
* **dashboard:** Add browser tracking ([0d6a124](https://github.com/PascaleBeier/hitkeep/commit/0d6a1241add3e05e099c5e7b58d166efc6b4d54c))
* **dashboard:** Allow deep links for share links ([3792b78](https://github.com/PascaleBeier/hitkeep/commit/3792b78f75d13864defd45be1ea8016bd4e31d3c))
* **l18n:** Localize mails ([6e6cd39](https://github.com/PascaleBeier/hitkeep/commit/6e6cd398ff4bac4a8e036df893266369cde7179e))
* **security:** add spam filtering pipeline and hostname analytics support ([0c1da67](https://github.com/PascaleBeier/hitkeep/commit/0c1da671c0b405113b917412f1ea28752f41069a))


### Bug Fixes

* **backend:** Include AI fetch and in takeout and harden tkeout handler ([794731a](https://github.com/PascaleBeier/hitkeep/commit/794731a56838757c99d9deb387e799182ec0220e))
* **backend:** Move bucket boundary fixes to mailable and improve mailable wording ([ab0a2cd](https://github.com/PascaleBeier/hitkeep/commit/ab0a2cd15b3e7063e83abfe4a416f7d587222e38))
* **ci:** enforce hitkeep binary execute bit in docker image ([ae596c3](https://github.com/PascaleBeier/hitkeep/commit/ae596c33e1f79afb984a5983d669bb7a8c51f61d)), closes [#97](https://github.com/PascaleBeier/hitkeep/issues/97)
* **deps:** bump frontend deps ([861c6d1](https://github.com/PascaleBeier/hitkeep/commit/861c6d173c43605ad1e4185e7c531ecf0834fb64))
* **deps:** update go to 1.26.2 and update dependencies ([f8b395c](https://github.com/PascaleBeier/hitkeep/commit/f8b395cac97f91b1279868a7c2bb2e98e64569d7))
* Ensure all handlers use spam filtering ([4617523](https://github.com/PascaleBeier/hitkeep/commit/461752320ab016ffedf94bcf1e446863c6fc4a36))
* **frontend:** long domain names break sidebar UI (closes [#87](https://github.com/PascaleBeier/hitkeep/issues/87)) ([#88](https://github.com/PascaleBeier/hitkeep/issues/88)) ([98e1283](https://github.com/PascaleBeier/hitkeep/commit/98e1283676eed8d7b4c1384a6ff146df0c837389))
* **frontend:** Update site selector to truncate ([7ac94d0](https://github.com/PascaleBeier/hitkeep/commit/7ac94d09b89843ada00d05c152304e5dfbbd2292))
* harden rollup invalidation and session KPI safety ([5d4682b](https://github.com/PascaleBeier/hitkeep/commit/5d4682b046602675d7633324a6c5a980536f4117))
* **tracker:** Pass on referer ([8287a92](https://github.com/PascaleBeier/hitkeep/commit/8287a929777018c78cce71cf1a822136b5ec31fc))

## [2.1.0](https://github.com/PascaleBeier/hitkeep/compare/v2.0.1...v2.1.0) (2026-03-15)


### Features

* **dashboard:** add landing and exit modes to pages card ([6a09365](https://github.com/PascaleBeier/hitkeep/commit/6a0936520563fc7cd2fe94e32de3acc038b8db51))
* **dashboard:** add language metrics to country selector kpi ([21c563a](https://github.com/PascaleBeier/hitkeep/commit/21c563a790a663e67985693512521596f12e0294))


### Bug Fixes

* **auth:** Support legacy passkeys in migration path and clear wording for leader / follower mfa ([e089eb8](https://github.com/PascaleBeier/hitkeep/commit/e089eb8f573e559f3648be637f2e21f7eee01572))
* **billing:** fix pending registration flow and API Docs for billing ([d127e84](https://github.com/PascaleBeier/hitkeep/commit/d127e8494ae2f181434296b40dc36db7ad9c060d))
* **deps:** Bump angular to v21.2.4 ([a79d1d4](https://github.com/PascaleBeier/hitkeep/commit/a79d1d4776e1fffe11682bac59ce872e5aa72e5f))
* **i18n:** Revise translations for missing special characters ([ca6f627](https://github.com/PascaleBeier/hitkeep/commit/ca6f6270f7cb0405e51485d4a8567e562bceeb82))


### Performance Improvements

* **auth:** Use lru for auth and limiters and remove handgrown totp and webauthn impl ([865ee8a](https://github.com/PascaleBeier/hitkeep/commit/865ee8ac714943b61f269d94989b390274de0e65))
* **frontend:** Cache country flags ([3062804](https://github.com/PascaleBeier/hitkeep/commit/3062804f083b68ceb891e3cecda48c69f6226884))
* **frontend:** Proxy duckduckgo favicons instead of io.copy ([33140a0](https://github.com/PascaleBeier/hitkeep/commit/33140a04ab4b24c4160aea0842df46e626c1dc09))
* **ingest:** batch analytics writes with duckdb appender ([06d82a9](https://github.com/PascaleBeier/hitkeep/commit/06d82a975630a1e7143b77ce9668a559632cfe80))

## [2.1.0](https://github.com/PascaleBeier/hitkeep/compare/v2.0.1...v2.1.0) (2026-03-15)


### Features

* **dashboard:** add landing and exit modes to pages card ([6a09365](https://github.com/PascaleBeier/hitkeep/commit/6a0936520563fc7cd2fe94e32de3acc038b8db51))
* **dashboard:** add language metrics to country selector kpi ([21c563a](https://github.com/PascaleBeier/hitkeep/commit/21c563a790a663e67985693512521596f12e0294))


### Bug Fixes

* **auth:** Support legacy passkeys in migration path and clear wording for leader / follower mfa ([e089eb8](https://github.com/PascaleBeier/hitkeep/commit/e089eb8f573e559f3648be637f2e21f7eee01572))
* **billing:** fix pending registration flow and API Docs for billing ([d127e84](https://github.com/PascaleBeier/hitkeep/commit/d127e8494ae2f181434296b40dc36db7ad9c060d))
* **deps:** Bump angular to v21.2.4 ([a79d1d4](https://github.com/PascaleBeier/hitkeep/commit/a79d1d4776e1fffe11682bac59ce872e5aa72e5f))
* **i18n:** Revise translations for missing special characters ([ca6f627](https://github.com/PascaleBeier/hitkeep/commit/ca6f6270f7cb0405e51485d4a8567e562bceeb82))


### Performance Improvements

* **auth:** Use lru for auth and limiters and remove handgrown totp and webauthn impl ([865ee8a](https://github.com/PascaleBeier/hitkeep/commit/865ee8ac714943b61f269d94989b390274de0e65))
* **frontend:** Cache country flags ([3062804](https://github.com/PascaleBeier/hitkeep/commit/3062804f083b68ceb891e3cecda48c69f6226884))
* **frontend:** Proxy duckduckgo favicons instead of io.copy ([33140a0](https://github.com/PascaleBeier/hitkeep/commit/33140a04ab4b24c4160aea0842df46e626c1dc09))
* **ingest:** batch analytics writes with duckdb appender ([06d82a9](https://github.com/PascaleBeier/hitkeep/commit/06d82a975630a1e7143b77ce9668a559632cfe80))

## [2.0.1](https://github.com/PascaleBeier/hitkeep/compare/v2.0.0...v2.0.1) (2026-03-11)


### Bug Fixes

* **frontend:** simplify password inputs across the app ([7ea010e](https://github.com/PascaleBeier/hitkeep/commit/7ea010efa05de8b454470de01bb313bf8bf6b057))

## [2.0.0](https://github.com/PascaleBeier/hitkeep/compare/v1.8.1...v2.0.0) (2026-03-11)


### ⚠ BREAKING CHANGES

* **teams:** GET /api/user/teams/{id}/audit now returns a paginated object with entries, total, limit, offset, has_more, and optional action instead of a bare array.

### Features

* Add Ecommerce Tracking ([628c882](https://github.com/PascaleBeier/hitkeep/commit/628c882ea67f930c55cc60b541ac8e25a838bd64))
* **api:** add team-owned api clients ([4223a1a](https://github.com/PascaleBeier/hitkeep/commit/4223a1a00399d72b2051aa3ff2e1b528d2e0ec64))
* **cli:** seed multitenant demo data [wip] ([04417fb](https://github.com/PascaleBeier/hitkeep/commit/04417fb142a0cd04fe2dc480366bf564e1ab5931))
* **cloud:** add hosted billing and signup backend ([f795cf6](https://github.com/PascaleBeier/hitkeep/commit/f795cf6d38384caa30e1e50ed208add1bc624b34))
* **cloud:** add hosted signup and billing surfaces ([bdbf94b](https://github.com/PascaleBeier/hitkeep/commit/bdbf94b97ff69402c109886892d68453620cc87f))
* **config:** add data path and backup settings [wip] ([da423c9](https://github.com/PascaleBeier/hitkeep/commit/da423c9526c7174e5b8a7c2a0d98aee153b7ab42))
* **db:** add tenant database migrations [wip] ([f460ab9](https://github.com/PascaleBeier/hitkeep/commit/f460ab91dc12a686dbde8dc95b865a9b750928f5))
* **db:** add tenant store manager and analytics helpers [wip] ([228843a](https://github.com/PascaleBeier/hitkeep/commit/228843a4ef337841d7cc281c4f2d1d74b7cd1481))
* **db:** add tenant team store core [wip] ([589f0fc](https://github.com/PascaleBeier/hitkeep/commit/589f0fcad8ba823f1612a7531f9f6edca8b8380f))
* **ecommerce:** add ga4-inspired analytics endpoints ([47b0227](https://github.com/PascaleBeier/hitkeep/commit/47b022761a35231dc019115f4280fc5694e33e18))
* **frontend:** add confirm popups to admin and site management [wip] ([ec1799e](https://github.com/PascaleBeier/hitkeep/commit/ec1799ea2495f5ff7e2d3649ca6f0b4678e286b9))
* **frontend:** add create team dialog [wip] ([7780626](https://github.com/PascaleBeier/hitkeep/commit/7780626cfb5d060c0f187b06b083c9f2b85da452))
* **frontend:** add ecommerce analytics dashboard ([09d99af](https://github.com/PascaleBeier/hitkeep/commit/09d99af234508f629d075241aca22df0d8ec9a0c))
* **frontend:** add return-url auth redirect support [wip] ([aaa5160](https://github.com/PascaleBeier/hitkeep/commit/aaa5160880b305cff2f22d8965e8e706c3b27276))
* **frontend:** add team admin pages [wip] ([7e1a160](https://github.com/PascaleBeier/hitkeep/commit/7e1a1602c7f08da65c06df7085204dedb8a7c603))
* **frontend:** add team models service and guard [wip] ([4b9484f](https://github.com/PascaleBeier/hitkeep/commit/4b9484fbed4b97e7b7742872dae473bd32df2592))
* **frontend:** add team switcher component [wip] ([98ba1e6](https://github.com/PascaleBeier/hitkeep/commit/98ba1e60bbb5f5fb888454d3024b20e107286a9c))
* **frontend:** move team switcher into account cluster ([7a3c6ab](https://github.com/PascaleBeier/hitkeep/commit/7a3c6ab3b0602c819b82396837a753c50ab46018))
* **frontend:** polish team admin surfaces ([bd48e69](https://github.com/PascaleBeier/hitkeep/commit/bd48e69349a5000d113566261736be3123328998))
* **frontend:** refresh analytics pages for linked ranges [wip] ([af93257](https://github.com/PascaleBeier/hitkeep/commit/af93257adde336c588e51a8f2cb97e30bc5b8615))
* **frontend:** wire team context into main layout [wip] ([512bc70](https://github.com/PascaleBeier/hitkeep/commit/512bc7028b1c187e65f23dba2a18425551179276))
* **mail:** add team invite email support [wip] ([92f5aea](https://github.com/PascaleBeier/hitkeep/commit/92f5aea0243a5a08a9fa9cded02ea9ea35037da5))
* **runtime:** wire tenant stores and entitlements [wip] ([ba97fae](https://github.com/PascaleBeier/hitkeep/commit/ba97fae77b00748820ef23003f0e79c6b0fca2a9))
* **seed:** add ecommerce demo funnel data ([9bbb34c](https://github.com/PascaleBeier/hitkeep/commit/9bbb34c4804fec519c8be89e79e62bbb3757517c))
* **server:** add team management endpoints [wip] ([c1a96e8](https://github.com/PascaleBeier/hitkeep/commit/c1a96e82dff4b787b634272681d16119f358d5c4))
* **server:** resolve analytics through tenant stores [wip] ([f0bff51](https://github.com/PascaleBeier/hitkeep/commit/f0bff51ebe7951705f395ad067b2392f09e4da53))
* **teams:** add archived team purge endpoint ([58b038c](https://github.com/PascaleBeier/hitkeep/commit/58b038c6180b864258faf5453b9475581e80e3b7))
* **teams:** add empty team onboarding cues ([8f79fde](https://github.com/PascaleBeier/hitkeep/commit/8f79fde0d2034639810066a59293e0d7021c818d))
* **teams:** add invite and ownership transfer admin flows ([4641b73](https://github.com/PascaleBeier/hitkeep/commit/4641b73eb3cdbe591c671d7901f124156bed887f))
* **teams:** add ownership transfer safeguards ([91d3442](https://github.com/PascaleBeier/hitkeep/commit/91d344248251c7527e2220aa3838f9da724de59a))
* **teams:** add paginated audit responses ([77ff1e1](https://github.com/PascaleBeier/hitkeep/commit/77ff1e194e8c7ee5828e2e2dfe3cc12566b0ec1e))
* **teams:** add persistent invite lifecycle ([2d74412](https://github.com/PascaleBeier/hitkeep/commit/2d74412f85b15200a922f6263aa43ff1c9dfecad))
* **teams:** add safe team archive lifecycle ([d25dcb7](https://github.com/PascaleBeier/hitkeep/commit/d25dcb7cc8827e2c6dd623792b5c17dc042db210))
* **teams:** add site transfer between teams ([af01e48](https://github.com/PascaleBeier/hitkeep/commit/af01e48ffc1e67a75b17909b7373dc6b3a379b13))
* **teams:** add team archive danger zone ([3a8a963](https://github.com/PascaleBeier/hitkeep/commit/3a8a9632aac0867196816dc67ba4c248f4426781))
* **teams:** document multitenant release notes ([15e31e5](https://github.com/PascaleBeier/hitkeep/commit/15e31e5c2c866a1a160a2dfb9f1f3d973b7c5914))
* **teams:** expose entitlements and usage metrics ([f2eb6a2](https://github.com/PascaleBeier/hitkeep/commit/f2eb6a247c21a06cb9df23aee77b21b9dd23010e))
* **worker:** add tenant backup worker [wip] ([5663dc5](https://github.com/PascaleBeier/hitkeep/commit/5663dc5b41fa7b262c07cc324cde6bd72575c9cb))
* **worker:** add tenant-aware retention and restore [wip] ([1cdc638](https://github.com/PascaleBeier/hitkeep/commit/1cdc638a2f5790c0534739f2a411a2394d119b05))


### Bug Fixes

* **ci:** lower binary glibc target ([1753289](https://github.com/PascaleBeier/hitkeep/commit/17532894796a039648554a55a979a30a3c6a0f26))
* **ci:** lower binary glibc target ([5be3ae3](https://github.com/PascaleBeier/hitkeep/commit/5be3ae3230bcc5c26bc1fcb55b9fcbc945cbd355))
* **ci:** lower binary glibc target ([23262df](https://github.com/PascaleBeier/hitkeep/commit/23262df114d2793fed5bb5a6a7a27b0558e8cac0))
* **cloud:** enforce single billed tenant per account ([9c6ddc5](https://github.com/PascaleBeier/hitkeep/commit/9c6ddc5acc9979f73dfc2e0d23f1d18e077b2b5d))
* **cloud:** hide plan limits in oss team overview ([74ae47d](https://github.com/PascaleBeier/hitkeep/commit/74ae47de3b7192fb170ef804b23938b97db0092b))
* **database:** force duckdb session timezone to utc ([23118d9](https://github.com/PascaleBeier/hitkeep/commit/23118d90696319d60a339eeac238309fe0f4d69f))
* **frontend:** localize range toolbar better ([38c1a3c](https://github.com/PascaleBeier/hitkeep/commit/38c1a3cd809f1dded6e074a12b24025293b59eea))
* **frontend:** Make multi-currency and missing ECommerce data a bit more resilient ([3487323](https://github.com/PascaleBeier/hitkeep/commit/348732384d90a9345859bad485f34fe099b9d8e1))
* **frontend:** restore site settings and embedded builds ([cff982f](https://github.com/PascaleBeier/hitkeep/commit/cff982f477bf8d4d32ff27c05e1c1d915fbb0c07))
* **i18n:** Missing ecommerce translations ([d24c5e2](https://github.com/PascaleBeier/hitkeep/commit/d24c5e2a0a73b103d0ecd7b638fc25839d01a928))
* **lint:** deduplicate tenant analytics handlers ([961733b](https://github.com/PascaleBeier/hitkeep/commit/961733baa2ed2e08e234d643b7b1de674a471631))
* **reports:** harden tenant boundaries ([0b82770](https://github.com/PascaleBeier/hitkeep/commit/0b8277079b64914230eac92599c13e4b3fdfc493))
* **sec:** suppress false-positive gravatar ssrf lint ([8e23fd2](https://github.com/PascaleBeier/hitkeep/commit/8e23fd2560613ba34fae0047af258130bb45729b))
* **security:** Add Recovery Codes and harden queries for gosec ([3d7d962](https://github.com/PascaleBeier/hitkeep/commit/3d7d962c41a12fba1af10cf79a0ac62f33cf0568))
* **teams:** block deleting users who still own teams ([a7db2d2](https://github.com/PascaleBeier/hitkeep/commit/a7db2d216f1e154c06d34f97f56a3cef4d5851e5))
* **teams:** gate audit activity to admins and owners ([cc32ec8](https://github.com/PascaleBeier/hitkeep/commit/cc32ec8a56b976e5496aa83b8aaf93f700f5402b))
* **teams:** normalize json payloads during site transfer ([5c218f6](https://github.com/PascaleBeier/hitkeep/commit/5c218f683609ca18ebcaa381c3a8f366900b8bb7))

## [1.8.1](https://github.com/PascaleBeier/hitkeep/compare/v1.8.0...v1.8.1) (2026-02-26)


### Bug Fixes

* **frontend:** Remove translation fragments from angular core l18n ([8b23ecd](https://github.com/PascaleBeier/hitkeep/commit/8b23ecd53f6345ead03a14144fc1a453c460fdef))
* **frontend:** Update and migrate angular to v21.2.0 ([f25f6b1](https://github.com/PascaleBeier/hitkeep/commit/f25f6b14fb8a153ef008befbcfbb9bf924031999))

## [1.8.0](https://github.com/PascaleBeier/hitkeep/compare/v1.7.0...v1.8.0) (2026-02-24)


### Features

* **analytics:** add period-over-period comparison ([c394b81](https://github.com/PascaleBeier/hitkeep/commit/c394b817a80a8e68f7c4e489a48b15171d5e6dfa))
* **events:** add event timeseries, property breakdown and audience analytics ([513333c](https://github.com/PascaleBeier/hitkeep/commit/513333c610e63121a71b856058eaf8eee3b8adf4))

## [1.7.0](https://github.com/PascaleBeier/hitkeep/compare/v1.6.0...v1.7.0) (2026-02-22)


### Features

* **admin:** include instance roles and site owner emails in listings ([08a30c6](https://github.com/PascaleBeier/hitkeep/commit/08a30c6f7b15df81b6b1908978941426ed1cfce6))
* **analytics:** add filterable UTM dimensions to site stats ([a5c3564](https://github.com/PascaleBeier/hitkeep/commit/a5c3564e9cfd46c78afa506824b8acf045569695))
* **analytics:** add UTM tracking and dashboard KPIs ([d2505f8](https://github.com/PascaleBeier/hitkeep/commit/d2505f814115a6d7b28aab2a5dec44eefa69cab7))
* **api:** publish complete OpenAPI 3.1 spec with Scalar viewer ([6b9ef12](https://github.com/PascaleBeier/hitkeep/commit/6b9ef12b47bc77809add9b0f534ba502a574e88d))
* **auth:** add password recovery pages ([e9613a4](https://github.com/PascaleBeier/hitkeep/commit/e9613a4b6ee10b82a99b0276cbe36891cb124824))
* **auth:** add TOTP and passkey MFA flows ([e599ebf](https://github.com/PascaleBeier/hitkeep/commit/e599ebfa57b6973308d23a67e66c3bc07d196769))
* **auth:** enhance security handlers and permissions ([e693269](https://github.com/PascaleBeier/hitkeep/commit/e693269c261cae88f2fa36e20de78270d7e66ab4))
* **backend:** add user profile management ([60b13f4](https://github.com/PascaleBeier/hitkeep/commit/60b13f4f9aee752c3c013441e1e1399b8de2f8d3))
* **backend:** update API types and server configuration ([c6a3885](https://github.com/PascaleBeier/hitkeep/commit/c6a3885e153823a19b3205c9c9ffb862ef3857c9))
* **dashboard:** add filterable UTM analytics page ([c696af5](https://github.com/PascaleBeier/hitkeep/commit/c696af5c29413248289e58035c02a484f7f76718))
* **dashboard:** add MFA and passkey security UX ([5082b40](https://github.com/PascaleBeier/hitkeep/commit/5082b40537f9f8c8bc866c121355f27a7be95403))
* **dashboard:** add relative-date-time component ([d809487](https://github.com/PascaleBeier/hitkeep/commit/d809487b2bd388e5439748537ef4833477892c2f))
* **dashboard:** add user settings page ([516e785](https://github.com/PascaleBeier/hitkeep/commit/516e785cf00eef4fda2853f202031ec9e75666f7))
* **dashboard:** enhance API reference documentation ([9a4241c](https://github.com/PascaleBeier/hitkeep/commit/9a4241ca85dfaa52895efccf7a273406259fdf83))
* **dashboard:** enhance settings and site management components ([f39a55f](https://github.com/PascaleBeier/hitkeep/commit/f39a55f066df3daca275829a1801dc2313c7fe5b))
* **dashboard:** standardize CRUD datatables and add exclusion settings ([59f3680](https://github.com/PascaleBeier/hitkeep/commit/59f368001f70e6521628fecb67777e63d5a8de58))
* **dashboard:** update admin and sharing features ([2708407](https://github.com/PascaleBeier/hitkeep/commit/27084078fc43149da25d2ca5b5cadbeb601fe58d))
* **dashboard:** update app routing ([8f5f175](https://github.com/PascaleBeier/hitkeep/commit/8f5f175d53dea7de507f98a3faf1f53d9fd7fc0b))
* **dashboard:** update core layout and services ([94b1ab1](https://github.com/PascaleBeier/hitkeep/commit/94b1ab1fc5cee1b5aee5a5959c6d0759fd03bab8))
* **dashboard:** update main pages and navigation ([8374d77](https://github.com/PascaleBeier/hitkeep/commit/8374d77ffa2737a9c3e3c1709bf3b4ade34beaa3))
* **frontend:** Add CRUD for share links ([6d3c2cd](https://github.com/PascaleBeier/hitkeep/commit/6d3c2cdb778f6bdf160c07b0d0219e52ec1900f2))
* **frontend:** Add UTM Builder ([94558e5](https://github.com/PascaleBeier/hitkeep/commit/94558e51707cf4cc1bbd57d02ba4e5f9cc11f851))
* **frontend:** Unify user settings UI ([c84bbb9](https://github.com/PascaleBeier/hitkeep/commit/c84bbb9658fc7d89758ff53aa18e7a846e0aa2c5))
* **i18n:** add es fr it locales and transloco optimize build ([8cea8bf](https://github.com/PascaleBeier/hitkeep/commit/8cea8bf7d7822a9fd5e67c918141eb5401634c75))
* **integration:** add API client CRUD, docs viewer, and auth hardening ([4b3c56e](https://github.com/PascaleBeier/hitkeep/commit/4b3c56e925db54c41b5b5fe38e1672a14e1fff21))
* **reports:** Add configurable E-Mail Reports for sites and user digests ([2b15017](https://github.com/PascaleBeier/hitkeep/commit/2b1501708f2842d87efbf667d55eda52162c7f9f))
* **security:** Add 2fa recovery command ([69559d8](https://github.com/PascaleBeier/hitkeep/commit/69559d85df9a89710f6c10aca75c1dd839073ab9))
* **security:** add IP exclusion filtering and fetch metadata guards ([9c65e45](https://github.com/PascaleBeier/hitkeep/commit/9c65e451ebb2c6ab34e261d850d128cb1f57f24c))
* **takeout:** add json and ndjson exports with centralized format handling ([32a620a](https://github.com/PascaleBeier/hitkeep/commit/32a620a13beda60083082d096c1973ddf6225307))


### Bug Fixes

* **auth:** preserve mfa challenge on invalid totp attempts ([d2b25aa](https://github.com/PascaleBeier/hitkeep/commit/d2b25aa9960d82d471a6077ade745d7ce1f7074d))
* **auth:** require mfa on password login when passkeys are configured ([1c28ee6](https://github.com/PascaleBeier/hitkeep/commit/1c28ee6ea8ac8c47ba400def425472f0b9e72e56))
* **backend:** update ingest and test utilities ([f951e94](https://github.com/PascaleBeier/hitkeep/commit/f951e9488ea06b171c11db7e6001db5ed40e5148))
* **dashboard:** simplify locale labels and stabilize API reference mount ([beb4c42](https://github.com/PascaleBeier/hitkeep/commit/beb4c420ef2343584d86db6326ca2ad76b1a834d))
* **frontend:** make user settings takeout action primary for consistent UX ([ace37b4](https://github.com/PascaleBeier/hitkeep/commit/ace37b4f75a0f91b6d3f1c6b9ceb9e5d2a2046a0))
* **frontend:** stabilize dashboard unit tests with updated providers ([0331af3](https://github.com/PascaleBeier/hitkeep/commit/0331af34f4c91d183f19b6216c9393714af4fb82))
* **frontend:** stop overriding dashboard production output hashing ([d3ced19](https://github.com/PascaleBeier/hitkeep/commit/d3ced1992f019433241dc917a1d96c74c9f23695))
* **mailer:** Correctly provide plain text Mailtemplates ([393647c](https://github.com/PascaleBeier/hitkeep/commit/393647c9245d50380bbdf9db15cef1880aa1b76d))
* **security:** address gosec v2.10 findings with targeted hardening ([b1b7f7c](https://github.com/PascaleBeier/hitkeep/commit/b1b7f7c0717474a55b5c5e6f24b79ed739292fa0))
* **security:** Reduce JWT token duration ([83dc981](https://github.com/PascaleBeier/hitkeep/commit/83dc9815a3db316bffde76eb18c0e4fb06d87852))
* **security:** Update tests for sec-fetch changes ([f6aa80d](https://github.com/PascaleBeier/hitkeep/commit/f6aa80d3b6e9c131c7573e5843860d3f3cf0e4b1))

## [1.6.0](https://github.com/PascaleBeier/hitkeep/compare/v1.5.4...v1.6.0) (2026-02-15)

### Features

- **frontend:** Add trend lines ([e362c85](https://github.com/PascaleBeier/hitkeep/commit/e362c856941a7b72205380a8af9d780797ca20bc))
- **i18n:** add transloco localization and user locale preferences ([bb65797](https://github.com/PascaleBeier/hitkeep/commit/bb65797dc7c9cf298d6e3f8219a6b200969a0754))

### Bug Fixes

- **cluster:** route traffic and readiness to leader ([b14dac0](https://github.com/PascaleBeier/hitkeep/commit/b14dac085140294703daa96edcdac8abebdb0bb3))
- **dev:** set default jwt secret for local backend ([ea6cc20](https://github.com/PascaleBeier/hitkeep/commit/ea6cc20fb133a7804a4457962b82d55d3b38a6a9))

## [1.5.4](https://github.com/PascaleBeier/hitkeep/compare/v1.5.3...v1.5.4) (2026-01-26)

### Miscellaneous Chores

- release 1.5.4 ([6805e3c](https://github.com/PascaleBeier/hitkeep/commit/6805e3c7f5d783b6900ceee72afb0618530bde54))
- release 1.5.4 ([0af3569](https://github.com/PascaleBeier/hitkeep/commit/0af3569443cecdaf012ad2e215fc614305fe5880))

## [1.5.3](https://github.com/PascaleBeier/hitkeep/compare/v1.5.2...v1.5.3) (2026-01-26)

### Miscellaneous Chores

- release 1.5.3 ([4a40f71](https://github.com/PascaleBeier/hitkeep/commit/4a40f71235fdf0314dc6377569abd0f5247d3320))

## [1.5.2](https://github.com/PascaleBeier/hitkeep/compare/v1.5.1...v1.5.2) (2026-01-26)

### Miscellaneous Chores

- release 1.5.2 ([569bc52](https://github.com/PascaleBeier/hitkeep/commit/569bc520d334f038cbb79004766737d2b9d18acf))

## [1.5.1](https://github.com/PascaleBeier/hitkeep/compare/v1.5.0...v1.5.1) (2026-01-25)

### Bug Fixes

- **frontend:** Wrong goal % denominator ([f3e7b6f](https://github.com/PascaleBeier/hitkeep/commit/f3e7b6f23fb6c42b80529a8ab8f6c091f28ffdfb))

## [1.5.0](https://github.com/PascaleBeier/hitkeep/compare/v1.4.0...v1.5.0) (2026-01-24)

### Features

- Add Dashboard filters ([b4b25aa](https://github.com/PascaleBeier/hitkeep/commit/b4b25aa728bb8f44ce0404d7e22d9839cefe77a1))
- Add share service ([b7f6572](https://github.com/PascaleBeier/hitkeep/commit/b7f65720fa43cb50aa96da700ae34a978967de75))

## [1.4.0](https://github.com/PascaleBeier/hitkeep/compare/v1.3.0...v1.4.0) (2025-11-24)

### Features

- **api:** Introduce Change Password Route ([2af6260](https://github.com/PascaleBeier/hitkeep/commit/2af6260eb48fc8c776fa592746f9001741ac77ab))
- **frontend:** Introduce Settings Route with Change Password Components ([702c82e](https://github.com/PascaleBeier/hitkeep/commit/702c82e71a43e32f3f1c97b9f76a6363af8f26ce))

### Bug Fixes

- **frontend:** Add settings component and fix mobile drawer issues ([c40a3c8](https://github.com/PascaleBeier/hitkeep/commit/c40a3c8bd2a4c9abcc0228a62b59d825cfe2daa4))

## [1.3.0](https://github.com/PascaleBeier/hitkeep/compare/v1.2.0...v1.3.0) (2025-11-23)

### Features

- **api:** Add Password Reset ([76a2b09](https://github.com/PascaleBeier/hitkeep/commit/76a2b09e027de508b5c8bd9ea884668bbd774ce4))
- **api:** Introduce Mailer integration ([73339b8](https://github.com/PascaleBeier/hitkeep/commit/73339b87bf19221e0138aa4373b8746165dc9b5b))
- **frontend:** Add Password Reset ([0f1e092](https://github.com/PascaleBeier/hitkeep/commit/0f1e092f78bb5cb7825cac8e79e48cf809265ee1))

## [1.2.0](https://github.com/PascaleBeier/hitkeep/compare/v1.1.1...v1.2.0) (2025-11-23)

### Features

- **api:** Add Live visitors metric ([42268ec](https://github.com/PascaleBeier/hitkeep/commit/42268ec405d33653092b65eb8d0e6d52741078ba))
- **api:** add top pages, referrers, and device breakdown to analytics stats ([50deb0b](https://github.com/PascaleBeier/hitkeep/commit/50deb0bc4b6101d87213e728e144de9254b46b0a))
- **frontend:** Add Live visitors metric ([a3c6003](https://github.com/PascaleBeier/hitkeep/commit/a3c6003709fd2e7bc316f8a897a1ca2eefceeb5f))
- **frontend:** add top pages, referrers, and device breakdown to analytics stats ([1df264c](https://github.com/PascaleBeier/hitkeep/commit/1df264c437cc762ebd10ba52568b307b4153edad))

### Bug Fixes

- **api:** Don't prepend www when resolving favicons so sites not listening on www can resolve ([748960f](https://github.com/PascaleBeier/hitkeep/commit/748960fa03be0dc006cfddd869c1c77a7e72a840))
- **api:** Properly close upstream favicon response ([552b114](https://github.com/PascaleBeier/hitkeep/commit/552b1149401aa95e0b80372ef0900d1f5c4b5fef))
- **dx:** Set default DB path to default docker volume for better DX without requiring a breaking change ([8458ddd](https://github.com/PascaleBeier/hitkeep/commit/8458ddd1ea1d285ea32532dd36687de8fd7abe26))
- **dx:** UPDATE README and docker-compose example for new db default path ([2537d62](https://github.com/PascaleBeier/hitkeep/commit/2537d626fb9dd1ab5646a5249147af7de222b71d))

## [1.1.1](https://github.com/PascaleBeier/hitkeep/compare/v1.1.0...v1.1.1) (2025-11-22)

### Bug Fixes

- **deps:** Bump gomod ([70ae350](https://github.com/PascaleBeier/hitkeep/commit/70ae350128b5614ebec5b2e04b56d752a526c193))

## [1.1.0](https://github.com/PascaleBeier/hitkeep/compare/v1.0.0...v1.1.0) (2025-11-21)

### Features

- **ui:** add date range selector and session metrics to dashboard ([89b61f9](https://github.com/PascaleBeier/hitkeep/commit/89b61f9ce69d1c7aa76b31d0baa284a1e598ad51))
- **ui:** Add more UI components for logout, dark mode, make everything a bit easier on the eyes ([b7814c5](https://github.com/PascaleBeier/hitkeep/commit/b7814c5f2efeef159fb4fb7c0fbe04d127b3f16a))
- **ui:** Allow all kinds of date ranges ([5481272](https://github.com/PascaleBeier/hitkeep/commit/54812728af2adb5daa8bc0e6dbde26505d7edc15))

### Bug Fixes

- **db:** normalize time boundaries for OLAP chart query to fix zero-data bug ([dab4e45](https://github.com/PascaleBeier/hitkeep/commit/dab4e45b6b67b19365997eb8e9bd6ae8dc52acc1))

## 1.0.0 (2025-11-21)

### Features

- **backend:** Add configurable Ratelimiter for endpoints ([925c356](https://github.com/PascaleBeier/hitkeep/commit/925c356bb7dbfaeeebf58c8c0a18619646cec149))
