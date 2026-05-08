# Changelog

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
