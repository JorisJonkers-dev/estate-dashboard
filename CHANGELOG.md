# Changelog

## [0.9.1](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.9.0...v0.9.1) (2026-10-27)


### Bug Fixes

* **deploy:** give the dashboard 256Mi, which 96Mi ran out of ([#94](https://github.com/JorisJonkers-dev/estate-dashboard/issues/94)) ([8ab49e2](https://github.com/JorisJonkers-dev/estate-dashboard/commit/8ab49e24dfb303b4090bea47c885d009b1642e3c))
* read a gated release's state from the Release Gate and Flagger, and follow it live ([#92](https://github.com/JorisJonkers-dev/estate-dashboard/issues/92)) ([ed9cb1f](https://github.com/JorisJonkers-dev/estate-dashboard/commit/ed9cb1f9b1cdb9ed58a412591fa37623da99db09))

## [0.9.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.8.3...v0.9.0) (2026-10-22)


### Features

* **health:** show each Job's runs, and each pod's sidecars and init containers, in the rollout view ([#87](https://github.com/JorisJonkers-dev/estate-dashboard/issues/87)) ([99c6276](https://github.com/JorisJonkers-dev/estate-dashboard/commit/99c62764a00c3de385443d924de52f2a0a9dee65))

## [0.8.3](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.8.2...v0.8.3) (2026-10-22)


### Bug Fixes

* **health:** leave out the pods a Job ran, so a Project with a Job is not marked degraded ([#85](https://github.com/JorisJonkers-dev/estate-dashboard/issues/85)) ([e3207b2](https://github.com/JorisJonkers-dev/estate-dashboard/commit/e3207b2cf33ac56bb54cb895ced1491d625512eb))

## [0.8.2](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.8.1...v0.8.2) (2026-10-21)


### Bug Fixes

* an alert with no class reads by its severity label, so critical alerts no longer show as info ([#79](https://github.com/JorisJonkers-dev/estate-dashboard/issues/79)) ([c84ddfd](https://github.com/JorisJonkers-dev/estate-dashboard/commit/c84ddfd10c3054d220c0799cb06b1a42aafba373))

## [0.8.1](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.8.0...v0.8.1) (2026-10-21)


### Bug Fixes

* a Rollout its unit has already moved past reads as done, not as switching in ([#76](https://github.com/JorisJonkers-dev/estate-dashboard/issues/76)) ([2b4ee00](https://github.com/JorisJonkers-dev/estate-dashboard/commit/2b4ee007e51aa8dc0a5e1d2ef3a66ad9efd2f9a2))

## [0.8.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.7.0...v0.8.0) (2026-10-21)


### Features

* the Composition page reads each delivery's run artifact, so it shows production's runs ([#72](https://github.com/JorisJonkers-dev/estate-dashboard/issues/72)) ([4039805](https://github.com/JorisJonkers-dev/estate-dashboard/commit/40398059a15084e532aec8453eb5bcb707799b8d))
* the images lock records the image's software version ([#73](https://github.com/JorisJonkers-dev/estate-dashboard/issues/73)) ([7b6cee8](https://github.com/JorisJonkers-dev/estate-dashboard/commit/7b6cee853e24330735428487038fcf039d595da2))


### Bug Fixes

* a Rollout leaves out the pods a Job ran, and a release that went ready after a failed check reads as recovered ([#74](https://github.com/JorisJonkers-dev/estate-dashboard/issues/74)) ([6babf5a](https://github.com/JorisJonkers-dev/estate-dashboard/commit/6babf5ad05de317cdcda9f045c2d213807a4d611))
* name a canary pod by its role even when no primary pod is listed ([#65](https://github.com/JorisJonkers-dev/estate-dashboard/issues/65)) ([953708a](https://github.com/JorisJonkers-dev/estate-dashboard/commit/953708a8aa9ee7cbee206302d5805dde0bbd7f22))
* read which Processes Flagger gates from the Release Gate, which production serves ([#69](https://github.com/JorisJonkers-dev/estate-dashboard/issues/69)) ([c20cc1c](https://github.com/JorisJonkers-dev/estate-dashboard/commit/c20cc1ca8ce592d969e46da9a3e3cc9f92355f6d))

## [0.7.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.6.0...v0.7.0) (2026-10-20)


### Features

* the Containers, container, Project and Rollout pages ([#62](https://github.com/JorisJonkers-dev/estate-dashboard/issues/62)) ([ef35498](https://github.com/JorisJonkers-dev/estate-dashboard/commit/ef3549845fad7cf5a27c8b8fc32cf5c0fb271291))


### Bug Fixes

* CI installs vacuum as a release binary, not a build from source ([#64](https://github.com/JorisJonkers-dev/estate-dashboard/issues/64)) ([700a018](https://github.com/JorisJonkers-dev/estate-dashboard/commit/700a018d454263ddcb97a6da02bc3e96747b674f))

## [0.6.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.5.0...v0.6.0) (2026-10-20)


### Features

* show each Project's live pods, its uptime checks and its stack's health ([#52](https://github.com/JorisJonkers-dev/estate-dashboard/issues/52)) ([be3997f](https://github.com/JorisJonkers-dev/estate-dashboard/commit/be3997fb07e54273f6acf1bc360f04aba3804fb7))

## [0.5.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.4.0...v0.5.0) (2026-10-16)


### Features

* connect with the libpq variables where DATABASE_URL is unset ([#48](https://github.com/JorisJonkers-dev/estate-dashboard/issues/48)) ([e9a10ea](https://github.com/JorisJonkers-dev/estate-dashboard/commit/e9a10eabf5400317b9830204b4ee09d55b940497))
* sign in to Postgres with the rotating login the edge derives ([#50](https://github.com/JorisJonkers-dev/estate-dashboard/issues/50)) ([96831ec](https://github.com/JorisJonkers-dev/estate-dashboard/commit/96831ece8585aa14f748641ae14f8dd3cadb7e0a))

## [0.4.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.3.2...v0.4.0) (2026-10-15)


### Features

* a list beside another column scrolls on its own ([#46](https://github.com/JorisJonkers-dev/estate-dashboard/issues/46)) ([140e7b2](https://github.com/JorisJonkers-dev/estate-dashboard/commit/140e7b22c608327b027c9cc4bb4a116a1fd50d7a))

## [0.3.2](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.3.1...v0.3.2) (2026-10-15)


### Bug Fixes

* drop the collector's lines about a gone container, read a pod from when it started, and name pods by role ([#44](https://github.com/JorisJonkers-dev/estate-dashboard/issues/44)) ([86b4dde](https://github.com/JorisJonkers-dev/estate-dashboard/commit/86b4dde0aa58f4eea65605a11435785f4da4481f))

## [0.3.1](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.3.0...v0.3.1) (2026-10-15)


### Bug Fixes

* find containers that just started, date Deployed by release, and read Alertmanager ([#42](https://github.com/JorisJonkers-dev/estate-dashboard/issues/42)) ([33e2e8f](https://github.com/JorisJonkers-dev/estate-dashboard/commit/33e2e8f9f796bc94014938470f6ac5647b38051b))

## [0.3.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.2.0...v0.3.0) (2026-10-15)


### Features

* the estate screens, logs by release, and the house theme ([#39](https://github.com/JorisJonkers-dev/estate-dashboard/issues/39)) ([2164716](https://github.com/JorisJonkers-dev/estate-dashboard/commit/21647163cdf8ff58ab48b39a56cb7a075a6a9dd1))

## [0.2.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.1.0...v0.2.0) (2026-10-15)


### Features

* publish the image and the fragment, and run without Alertmanager ([#27](https://github.com/JorisJonkers-dev/estate-dashboard/issues/27)) ([9b3147c](https://github.com/JorisJonkers-dev/estate-dashboard/commit/9b3147c0051197783376a219289b2f7ef1b01b30))


### Bug Fixes

* read the gate's inputs and records from the Release Gate, and hold no ConfigMap access ([#24](https://github.com/JorisJonkers-dev/estate-dashboard/issues/24)) ([110353d](https://github.com/JorisJonkers-dev/estate-dashboard/commit/110353d1e396f4b711a080d7c0708bd4b36405a7))

## 0.1.0 (2026-10-07)


### Features

* read Alertmanager's alerts, record what fires and resolves, and silence an alert there ([#12](https://github.com/JorisJonkers-dev/estate-dashboard/issues/12)) ([653f1f5](https://github.com/JorisJonkers-dev/estate-dashboard/commit/653f1f5ea3c4f5d063b45baf5ab2028b3cf5fda9))
* read each Project's pin, its deploy log and the open issues from the Estate repository ([#19](https://github.com/JorisJonkers-dev/estate-dashboard/issues/19)) ([5d5e31c](https://github.com/JorisJonkers-dev/estate-dashboard/commit/5d5e31cccb3b7028d850493da360229c6a1c4d2a))
* read Flux's sources and units and each gated release from the cluster, getting nothing it cannot name ([#16](https://github.com/JorisJonkers-dev/estate-dashboard/issues/16)) ([85cdc7a](https://github.com/JorisJonkers-dev/estate-dashboard/commit/85cdc7a0805cbbb1b1962f298762d2b1f60fa812))
* scaffold the dashboard: its contract, its project file and its own state ([#6](https://github.com/JorisJonkers-dev/estate-dashboard/issues/6)) ([ec3b99d](https://github.com/JorisJonkers-dev/estate-dashboard/commit/ec3b99d7c430ec3fc0dd912fcbebbf9854ed580d))
* sign admins in through auth, with a session of the dashboard's own ([#8](https://github.com/JorisJonkers-dev/estate-dashboard/issues/8)) ([83f74cd](https://github.com/JorisJonkers-dev/estate-dashboard/commit/83f74cd21403d320b0e988616246119696d0bd0d))


### Bug Fixes

* keep the dashboard's host behind forward-auth, as well as signing admins in itself ([#10](https://github.com/JorisJonkers-dev/estate-dashboard/issues/10)) ([0996813](https://github.com/JorisJonkers-dev/estate-dashboard/commit/09968132c8c5fb4cec151915180f1a13f2c7fbe7))
